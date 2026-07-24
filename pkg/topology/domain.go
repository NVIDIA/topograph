/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package topology

import (
	"fmt"
	"maps"
	"strings"

	"k8s.io/klog/v2"
)

type HostInfo struct {
	Domain     string
	InstanceID string
	HostName   string
	SubDomain  string // optional: sub-domain name this host belongs to within its accelerator domain
}

// vertexMeta holds domain-tree-specific metadata alongside a Vertex. The
// general-purpose Vertex type is kept unmodified; this type carries the per-node
// counts and host map that do not belong on Vertex.
type vertexMeta struct {
	actualNodeCount  int
	desiredNodeCount int
	hosts            map[string]*HostInfo // non-nil only for leaf vertices
}

// DomainTree is returned by GetDomainTree. It pairs a Vertex tree (the node
// hierarchy) with a per-vertex metadata map (host counts and slot capacities).
type DomainTree struct {
	Root *Vertex
	meta map[*Vertex]*vertexMeta
}

// Hosts returns the host map for v, or nil if v is not a leaf in this tree.
func (dt *DomainTree) Hosts(v *Vertex) map[string]*HostInfo {
	if m, ok := dt.meta[v]; ok {
		return m.hosts
	}
	return nil
}

// DesiredNodeCount returns the slot capacity assigned to v during tree construction.
func (dt *DomainTree) DesiredNodeCount(v *Vertex) int {
	if m, ok := dt.meta[v]; ok {
		return m.desiredNodeCount
	}
	return 0
}

// DomainMap maps accelerator domain name to host metadata.
type DomainMap map[string]map[string]*HostInfo

func NewDomainMap() DomainMap {
	return make(DomainMap)
}

func (m DomainMap) AddHost(domain, instance, host string) {
	m.AddHostInfo(&HostInfo{Domain: domain, InstanceID: instance, HostName: host})
}

func (m DomainMap) String() string {
	var str strings.Builder
	str.WriteString("DomainMap:\n")
	for name, nodes := range m {
		fmt.Fprintf(&str, " %s : %v\n", name, nodes)
	}
	return str.String()
}

func (m DomainMap) AddHostInfo(hostInfo *HostInfo) {
	if hostInfo == nil {
		return
	}
	if hostInfo.Domain == "" {
		klog.Warningf("skipping topology domain with empty name for host %q (instance %q)", hostInfo.HostName, hostInfo.InstanceID)
		return
	}

	if hosts, ok := m[hostInfo.Domain]; ok {
		hosts[hostInfo.HostName] = hostInfo
	} else {
		m[hostInfo.Domain] = map[string]*HostInfo{hostInfo.HostName: hostInfo}
	}
}

// GetDomainTree builds a flat Vertex tree from the DomainMap and returns a
// DomainTree that pairs the tree with per-vertex metadata:
//
//   - When no host in a domain has a SubDomain, the domain vertex is a leaf
//     that holds its hosts directly (one level below root).
//   - When hosts carry a SubDomain, the domain vertex has one child per distinct
//     SubDomain value, and each sub-domain vertex holds the hosts belonging to it
//     (two levels below root).
//
// DesiredNodeCount is then set on every vertex via a BFS pass: all vertices at
// the same tree depth receive the smallest blockSize >= the maximum
// ActualNodeCount at that depth. Root (ActualNodeCount == 0) always receives
// blockSizes[last].
func (m DomainMap) GetDomainTree(blockSizes []int) *DomainTree {
	dt := &DomainTree{
		Root: &Vertex{ID: "root", Vertices: make(map[string]*Vertex)},
		meta: make(map[*Vertex]*vertexMeta),
	}
	dt.meta[dt.Root] = &vertexMeta{}

	for domain, hosts := range m {
		domainVertex := &Vertex{
			ID:       domain,
			Vertices: make(map[string]*Vertex),
		}
		dt.Root.Vertices[domain] = domainVertex
		dt.meta[domainVertex] = &vertexMeta{}

		hasSubDomain := false
		for _, host := range hosts {
			if host.SubDomain != "" {
				hasSubDomain = true
				break
			}
		}

		if !hasSubDomain {
			// One-level: domain vertex is the leaf; hosts live here directly.
			hostMap := make(map[string]*HostInfo, len(hosts))
			maps.Copy(hostMap, hosts)
			dt.meta[domainVertex].hosts = hostMap
			dt.meta[domainVertex].actualNodeCount = len(hosts)
		} else {
			// Two-level: one sub-domain vertex per distinct SubDomain under the domain.
			// Count only hosts that are successfully placed so that partially-configured
			// deployments (some hosts missing SubDomain) do not inflate actualNodeCount.
			placed := 0
			for _, host := range hosts {
				gn := host.SubDomain
				if gn == "" {
					// A host with no SubDomain in a domain where other hosts carry
					// SubDomains indicates a partially-configured provider. Bucketing
					// it under key "" would create a vertex that sorts before all real
					// sub-domains, shifting block numbers and emitting a nameless block.
					klog.Warningf("domain %q: host %q has no SubDomain while other hosts in the domain do; skipping", domain, host.HostName)
					continue
				}
				if _, ok := domainVertex.Vertices[gn]; !ok {
					subVertex := &Vertex{ID: gn, Vertices: make(map[string]*Vertex)}
					domainVertex.Vertices[gn] = subVertex
					dt.meta[subVertex] = &vertexMeta{hosts: make(map[string]*HostInfo)}
				}
				subMeta := dt.meta[domainVertex.Vertices[gn]]
				subMeta.hosts[host.HostName] = host
				subMeta.actualNodeCount++
				placed++
			}
			dt.meta[domainVertex].actualNodeCount = placed
		}
	}

	dt.setDesiredCountByLevel(blockSizes)
	return dt
}

// setDesiredCountByLevel assigns DesiredNodeCount via a BFS pass: all vertices
// at the same depth receive the smallest blockSize >= the maximum ActualNodeCount
// at that depth. Root (ActualNodeCount == 0) always receives blockSizes[last].
func (dt *DomainTree) setDesiredCountByLevel(blockSizes []int) {
	if dt == nil || dt.Root == nil || len(blockSizes) == 0 {
		return
	}
	last := blockSizes[len(blockSizes)-1]

	type entry struct {
		node  *Vertex
		depth int
	}

	queue := []entry{{dt.Root, 0}}
	depthMax := map[int]int{}
	var visited []entry

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited = append(visited, curr)
		// Always insert the depth key so depthMax covers every level, including
		// root (ActualNodeCount == 0). Without this, depth 0 is absent from the
		// map and root's DesiredNodeCount would be left at 0.
		actual := dt.meta[curr.node].actualNodeCount
		if actual > depthMax[curr.depth] {
			depthMax[curr.depth] = actual
		} else if _, seen := depthMax[curr.depth]; !seen {
			depthMax[curr.depth] = 0
		}
		for _, child := range curr.node.Vertices {
			queue = append(queue, entry{child, curr.depth + 1})
		}
	}

	desiredByDepth := make(map[int]int, len(depthMax))
	for depth, maxCount := range depthMax {
		if maxCount == 0 {
			desiredByDepth[depth] = last
			continue
		}
		desired := last
		for _, v := range blockSizes {
			if v >= maxCount {
				desired = v
				break
			}
		}
		desiredByDepth[depth] = desired
	}

	for _, e := range visited {
		dt.meta[e.node].desiredNodeCount = desiredByDepth[e.depth]
	}
}
