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
//
// Safety invariant: every *Vertex stored in Vertices must be the address of the
// embedded Vertex field of a *BlockVertex — that is, values of the form
// &child.Vertex. Inserting a plain *Vertex that was not obtained this way breaks
// the unsafe cast in asBlockVertex and ChildAt. All writes to Vertices for a
// BlockVertex must go through code that upholds this invariant; the exported
// Vertices field inherited from the embedded Vertex provides no compiler
// enforcement of this constraint.
type BlockVertex struct {
	Vertex
	actualNodeCount  int
	desiredNodeCount int
	hosts            map[string]*HostInfo // non-nil only for leaf vertices
}

// asBlockVertex recovers the *BlockVertex whose embedded Vertex field v points
// to. Because BlockVertex embeds Vertex as its first field, Go guarantees that
// the addresses are identical, making the cast safe — but only for *Vertex
// values stored as &blockVertex.Vertex. Callers must never pass a *Vertex that
// was not obtained that way; see the BlockVertex safety invariant.
func asBlockVertex(v *Vertex) *BlockVertex {
	return (*BlockVertex)(unsafe.Pointer(v))
}

// ChildAt returns the child BlockVertex keyed by name, or nil if absent.
// It is the safe public accessor for child vertices; callers should use this
// rather than reading Vertices directly to avoid bypassing the safety invariant.
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
//     (two levels below root). A host with an empty SubDomain in an otherwise
//     dual-level domain is placed in a fallback sub-domain vertex keyed by the
//     accelerator domain name, so the host is always emitted.
//
// DesiredNodeCount is then set on every vertex via a BFS pass whose behaviour
// depends on the number of blockSizes provided:
//
//   - Multiple blockSizes: all vertices at the same tree depth receive the
//     smallest blockSize >= the maximum actualNodeCount at that depth; the root
//     (actualNodeCount == 0) receives blockSizes[last].
//   - Single blockSize: each vertex is rounded up independently to the nearest
//     multiple of that size; the root receives 0 (ceil(0/bs)*bs = 0) and is
//     therefore not padded by convert().
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
					// A host with no SubDomain in a domain where other hosts carry one
					// indicates a partially-configured provider (e.g. OCI missing rack
					// info). Place it in a fallback sub-domain vertex keyed by the
					// accelerator domain name so the host is always emitted rather than
					// silently dropped.
					klog.Warningf("domain %q: host %q has no SubDomain; placing in fallback sub-domain %q", domain, host.HostName, domain)
					gn = domain
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

// setDesiredCountByLevel assigns desiredNodeCount via a BFS pass.
//
// Multiple blockSizes: all vertices at the same depth receive the smallest
// blockSize >= the maximum actualNodeCount at that depth; the root
// (actualNodeCount == 0) receives blockSizes[last].
//
// Single blockSize: each vertex is rounded up to the nearest multiple of that
// size independently (ceil(actualNodeCount/bs)*bs). Root gets 0 because its
// actualNodeCount is 0.
//
// blockSizes need not be sorted by the caller; a sorted copy is used internally.
func (bv *BlockVertex) setDesiredCountByLevel(blockSizes []int) {
	if bv == nil || len(blockSizes) == 0 {
		return
	}
	bs := slices.Sorted(slices.Values(blockSizes)) // ascending copy, caller's slice is unchanged

	type entry struct {
		node  *BlockVertex
		depth int
	}

	queue := []entry{{bv, 0}}
	maxCountByDepth := []int{}
	var visited []entry

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited = append(visited, curr)
		actual := curr.node.actualNodeCount

		//Calculate the maximum actualNodeCount seen at this depth so far.
		if curr.depth >= len(maxCountByDepth) {
			maxCountByDepth = append(maxCountByDepth, actual)
		} else {
			maxCountByDepth[curr.depth] = max(maxCountByDepth[curr.depth], actual)
		}
		for _, v := range curr.node.Vertices {
			queue = append(queue, entry{asBlockVertex(v), curr.depth + 1})
		}
	}

	//Get the desired node count for each depth level
	// based on the maximum actual node count at that depth and the block sizes.
	desiredByDepth := getDesiredCountByLevel(maxCountByDepth, bs)

	for _, e := range visited {
		if len(bs) == 1 {
			// Single block size:
			// Round up actualNodeCount to the nearest multiple of the single block size.
			cnt := e.node.actualNodeCount
			e.node.desiredNodeCount = ((cnt + bs[0] - 1) / bs[0]) * bs[0]
		} else {
			e.node.desiredNodeCount = desiredByDepth[e.depth]
		}
	}
}

// Returns the desired node count for each depth level
// based on the maximum actual node count at that depth and the block sizes.
func getDesiredCountByLevel(maxCountByDepth, bs []int) []int {
	desiredByDepth := make([]int, len(maxCountByDepth))

	if len(maxCountByDepth) == 0 || len(bs) == 0 {
		return desiredByDepth
	}

	bsIndex := 0
	for i := len(desiredByDepth) - 1; i >= 0; i-- {
		//Derive the desired value either from the provided block size (if available)
		// or from the previously computed desired value at the next depth level.
		var base int
		if bsIndex < len(bs) {
			base = bs[bsIndex]
		} else {
			base = desiredByDepth[i+1]
		}
		desiredByDepth[i] = pow2GroupCapacity(base, maxCountByDepth[i])

		//Increament the block size index to the next block size that is larger than the current desired value.
		for bsIndex+1 < len(bs) && bs[bsIndex+1] <= desiredByDepth[i] {
			bsIndex++
		}
		bsIndex++
	}

	//If there are still block sizes left, set the desired node count for the root to the largest block size.
	if bsIndex < len(bs) {
		desiredByDepth[0] = bs[len(bs)-1]
	}
	return desiredByDepth
}

// pow2GroupCapacity returns the smallest value of the form 2^n × base that is
// >= maxCount, matching the pre-change groupSizeFromDomains power-of-two logic.
func pow2GroupCapacity(base, maxCount int) int {
	capacity := base
	for capacity < maxCount {
		capacity *= 2
	}
	return capacity
}
