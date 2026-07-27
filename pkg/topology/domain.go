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
	"slices"
	"strings"
	"unsafe"

	"k8s.io/klog/v2"
)

type HostInfo struct {
	Domain     string
	InstanceID string
	HostName   string
	SubDomain  string // optional: sub-domain name this host belongs to within its accelerator domain
}

// BlockVertex augments a Vertex with domain-tree-specific metadata. The
// general-purpose Vertex type is kept unmodified; BlockVertex embeds it and
// reuses Vertex.Vertices to store child BlockVertices (cast as *Vertex) so
// there is no separate Children map.
type BlockVertex struct {
	Vertex
	actualNodeCount  int
	desiredNodeCount int
	hosts            map[string]*HostInfo // non-nil only for leaf vertices
}

// asBlockVertex recovers the *BlockVertex whose embedded Vertex field v points
// to. Because BlockVertex embeds Vertex as its first field, Go guarantees that
// the addresses are identical, making the unsafe cast safe for any *Vertex that
// was stored as &blockVertex.Vertex.
func asBlockVertex(v *Vertex) *BlockVertex {
	return (*BlockVertex)(unsafe.Pointer(v))
}

// ChildAt returns the child BlockVertex keyed by name, or nil if absent.
func (bv *BlockVertex) ChildAt(name string) *BlockVertex {
	if v := bv.Vertices[name]; v != nil {
		return asBlockVertex(v)
	}
	return nil
}

// Hosts returns the host map for this vertex; nil for interior vertices.
func (bv *BlockVertex) Hosts() map[string]*HostInfo {
	if bv == nil {
		return nil
	}
	return bv.hosts
}

// DesiredNodeCount returns the slot capacity assigned to this vertex.
func (bv *BlockVertex) DesiredNodeCount() int {
	if bv == nil {
		return 0
	}
	return bv.desiredNodeCount
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

// GetDomainTree builds a flat BlockVertex tree from the DomainMap:
//
//   - When no host in a domain has a SubDomain, the domain vertex is a leaf
//     that holds its hosts directly (one level below root).
//   - When hosts carry a SubDomain, the domain vertex has one child per distinct
//     SubDomain value, and each sub-domain vertex holds the hosts belonging to it
//     (two levels below root).
//
// DesiredNodeCount is then set on every vertex via a BFS pass: all vertices at
// the same tree depth receive the smallest blockSize >= the maximum
// actualNodeCount at that depth. Root (actualNodeCount == 0) always receives
// blockSizes[last].
func (m DomainMap) GetDomainTree(blockSizes []int) *BlockVertex {
	root := &BlockVertex{Vertex: Vertex{ID: "root", Vertices: make(map[string]*Vertex)}}

	for domain, hosts := range m {
		domainBV := &BlockVertex{Vertex: Vertex{ID: domain}}
		root.Vertices[domain] = &domainBV.Vertex

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
			domainBV.hosts = hostMap
			domainBV.actualNodeCount = len(hosts)
		} else {
			// Two-level: one sub-domain vertex per distinct SubDomain under the domain.
			// Count only hosts that are successfully placed so that partially-configured
			// deployments (some hosts missing SubDomain) do not inflate actualNodeCount.
			domainBV.Vertices = make(map[string]*Vertex)
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
				sub := domainBV.ChildAt(gn)
				if sub == nil {
					sub = &BlockVertex{Vertex: Vertex{ID: gn}, hosts: make(map[string]*HostInfo)}
					domainBV.Vertices[gn] = &sub.Vertex
				}
				sub.hosts[host.HostName] = host
				sub.actualNodeCount++
				placed++
			}
			domainBV.actualNodeCount = placed
		}
	}

	root.setDesiredCountByLevel(blockSizes)
	return root
}

// setDesiredCountByLevel assigns desiredNodeCount via a BFS pass: all vertices
// at the same depth receive the smallest blockSize >= the maximum actualNodeCount
// at that depth. Root (actualNodeCount == 0) always receives blockSizes[last].
// blockSizes need not be sorted by the caller; a sorted copy is used internally.
func (bv *BlockVertex) setDesiredCountByLevel(blockSizes []int) {
	if bv == nil || len(blockSizes) == 0 {
		return
	}
	bs := slices.Sorted(slices.Values(blockSizes)) // ascending copy, caller's slice is unchanged
	last := bs[len(bs)-1]

	type entry struct {
		node  *BlockVertex
		depth int
	}

	queue := []entry{{bv, 0}}
	depthMax := map[int]int{}
	var visited []entry

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited = append(visited, curr)
		// Always insert the depth key so depthMax covers every level, including
		// root (actualNodeCount == 0). Without this, depth 0 is absent from the
		// map and root's desiredNodeCount would be left at 0.
		actual := curr.node.actualNodeCount
		if actual > depthMax[curr.depth] {
			depthMax[curr.depth] = actual
		} else if _, seen := depthMax[curr.depth]; !seen {
			depthMax[curr.depth] = 0
		}
		for _, v := range curr.node.Vertices {
			queue = append(queue, entry{asBlockVertex(v), curr.depth + 1})
		}
	}

	desiredByDepth := make(map[int]int, len(depthMax))
	for depth, maxCount := range depthMax {
		if maxCount == 0 {
			desiredByDepth[depth] = last
			continue
		}
		desired := last
		for _, v := range bs {
			if v >= maxCount {
				desired = v
				break
			}
		}
		desiredByDepth[depth] = desired
	}

	for _, e := range visited {
		e.node.desiredNodeCount = desiredByDepth[e.depth]
	}
}
