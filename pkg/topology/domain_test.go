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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetDomainTreeUnrackedHostFallback verifies that a host with an empty SubDomain
// inside a two-level domain (where other hosts carry SubDomain values) is placed in a
// fallback sub-domain vertex keyed by the accelerator domain name rather than dropped.
// TestBlockVertexNilSafety verifies that DesiredNodeCount and Hosts do not panic on a
// nil receiver and return their documented zero values.
func TestBlockVertexNilSafety(t *testing.T) {
	var bv *BlockVertex
	require.Equal(t, 0, bv.DesiredNodeCount())
	require.Nil(t, bv.Hosts())
}

// TestGetDomainTreeSingleLevel verifies the one-level path: when no host in a domain
// carries a SubDomain, the domain vertex is a leaf (Hosts() != nil) holding all hosts
// directly. DesiredNodeCount follows pow2GroupCapacity(baseBS, actualCount) at the domain
// level, and the root always receives blockSizes[last].
func TestGetDomainTreeSingleLevel(t *testing.T) {
	dm := NewDomainMap()
	for i := 1; i <= 3; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i)})
	}

	root := dm.GetDomainTree([]int{4, 8})
	require.NotNil(t, root)
	require.Equal(t, 8, root.DesiredNodeCount(), "root must receive blockSizes[last]")

	d1 := root.ChildAt("d1")
	require.NotNil(t, d1)
	require.NotNil(t, d1.Hosts(), "single-level domain vertex must be a leaf (Hosts != nil)")
	require.Len(t, d1.Hosts(), 3)
	require.Equal(t, 3, d1.actualNodeCount)
	require.Equal(t, 4, d1.DesiredNodeCount(), "pow2GroupCapacity(4,3)=4")
}

// TestGetDomainTreeTwoLevel verifies the two-level path: when hosts carry SubDomain, the
// domain vertex is an interior node (Hosts() == nil) and each distinct SubDomain becomes
// a leaf child. Desired counts are max-driven across all sub-domain siblings.
func TestGetDomainTreeTwoLevel(t *testing.T) {
	dm := NewDomainMap()
	for i := 1; i <= 4; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i), SubDomain: "d1.rack-1"})
	}
	for i := 5; i <= 9; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i), SubDomain: "d1.rack-2"})
	}

	root := dm.GetDomainTree([]int{9, 144})
	require.Equal(t, 144, root.DesiredNodeCount())

	d1 := root.ChildAt("d1")
	require.NotNil(t, d1)
	require.Nil(t, d1.Hosts(), "two-level domain vertex must be interior (Hosts == nil)")
	require.Equal(t, 9, d1.actualNodeCount)

	r1 := d1.ChildAt("d1.rack-1")
	require.NotNil(t, r1)
	require.Len(t, r1.Hosts(), 4)
	require.Equal(t, 9, r1.DesiredNodeCount(), "all leaves get max-driven desired: pow2GroupCapacity(9,5)=9")

	r2 := d1.ChildAt("d1.rack-2")
	require.NotNil(t, r2)
	require.Len(t, r2.Hosts(), 5)
	require.Equal(t, 9, r2.DesiredNodeCount())
}

// TestGetDomainTreeBFSLevelSizing verifies that setDesiredCountByLevel uses the maximum
// actualNodeCount across all siblings at each depth so that every domain at the same level
// receives the same DesiredNodeCount, not an individually-computed one.
func TestGetDomainTreeBFSLevelSizing(t *testing.T) {
	dm := NewDomainMap()
	for i := 1; i <= 3; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i)})
	}
	for i := 1; i <= 4; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d2", HostName: fmt.Sprintf("g%d", i)})
	}

	root := dm.GetDomainTree([]int{4, 16})
	require.Equal(t, 16, root.DesiredNodeCount(), "root must receive blockSizes[last]")

	d1 := root.ChildAt("d1")
	d2 := root.ChildAt("d2")
	require.NotNil(t, d1)
	require.NotNil(t, d2)
	// max actual at depth 1 is 4 (d2); pow2GroupCapacity(4,4)=4; both domains get 4.
	require.Equal(t, 4, d1.DesiredNodeCount(), "smaller domain must match the max-driven desired count")
	require.Equal(t, 4, d2.DesiredNodeCount())
}

// TestGetDomainTreeSingleBlockSizeRounding verifies the single-blockSize per-node
// rounding path: each vertex rounds its own actualNodeCount up to the nearest multiple
// of the single blockSize (root's actualNodeCount=0 produces desiredNodeCount=0).
func TestGetDomainTreeSingleBlockSizeRounding(t *testing.T) {
	dm := NewDomainMap()
	for i := 1; i <= 3; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i)})
	}
	for i := 1; i <= 5; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d2", HostName: fmt.Sprintf("g%d", i)})
	}

	root := dm.GetDomainTree([]int{4})

	require.Equal(t, 0, root.DesiredNodeCount(), "root actualNodeCount=0 → ((0+4-1)/4)*4=0")
	require.Equal(t, 4, root.ChildAt("d1").DesiredNodeCount(), "ceil(3/4)*4=4")
	require.Equal(t, 8, root.ChildAt("d2").DesiredNodeCount(), "ceil(5/4)*4=8")
}

// TestGetDomainTreeBlockSizesUnsortedContract verifies the documented contract that
// blockSizes need not be sorted by the caller — reversed and sorted inputs must produce
// identical DesiredNodeCount values at every level.
func TestGetDomainTreeBlockSizesUnsortedContract(t *testing.T) {
	dm := NewDomainMap()
	for i := 1; i <= 3; i++ {
		dm.AddHostInfo(&HostInfo{Domain: "d1", HostName: fmt.Sprintf("h%d", i)})
	}

	sorted := dm.GetDomainTree([]int{4, 16})
	reversed := dm.GetDomainTree([]int{16, 4})

	require.Equal(t, sorted.DesiredNodeCount(), reversed.DesiredNodeCount(),
		"root desired count must be identical regardless of blockSizes order")
	require.Equal(t, sorted.ChildAt("d1").DesiredNodeCount(), reversed.ChildAt("d1").DesiredNodeCount(),
		"domain desired count must be identical regardless of blockSizes order")
}

func TestGetDomainTreeUnrackedHostFallback(t *testing.T) {
	const domain = "nvl-a"
	const rackedSubDomain = "nvl-a.rack-1"

	dm := NewDomainMap()
	// 8 racked hosts
	for i := 1; i <= 8; i++ {
		dm.AddHostInfo(&HostInfo{
			Domain:    domain,
			HostName:  fmt.Sprintf("node%04d", i),
			SubDomain: rackedSubDomain,
		})
	}
	// 1 rackless host — SubDomain intentionally empty, as OCI sets it when rack is absent
	dm.AddHostInfo(&HostInfo{
		Domain:   domain,
		HostName: "node0009",
	})

	root := dm.GetDomainTree([]int{9, 144})
	require.NotNil(t, root)

	domainBV := root.ChildAt(domain)
	require.NotNil(t, domainBV, "domain vertex must exist")
	require.Equal(t, 9, domainBV.actualNodeCount, "all 9 hosts must be placed (none dropped)")

	// Fallback vertex is keyed by the domain name itself.
	fallback := domainBV.ChildAt(domain)
	require.NotNil(t, fallback, "fallback sub-domain vertex keyed by domain name must exist")
	require.Len(t, fallback.Hosts(), 1, "fallback vertex must hold the one rackless host")
	require.NotNil(t, fallback.Hosts()["node0009"])

	// Racked vertex still holds all 8 racked hosts.
	racked := domainBV.ChildAt(rackedSubDomain)
	require.NotNil(t, racked, "racked sub-domain vertex must exist")
	require.Len(t, racked.Hosts(), 8)
}

func TestDomainMapAddHost(t *testing.T) {
	domainMap := NewDomainMap()

	domainMap.AddHost("domain1", "instance1", "host1")
	domainMap.AddHost("domain1", "instance2", "host2")
	domainMap.AddHost("domain2", "instance3", "host3")
	domainMap.AddHost("", "instance4", "host4")

	require.Equal(t, DomainMap{
		"domain1": map[string]*HostInfo{
			"host1": {Domain: "domain1", InstanceID: "instance1", HostName: "host1"},
			"host2": {Domain: "domain1", InstanceID: "instance2", HostName: "host2"},
		},
		"domain2": map[string]*HostInfo{
			"host3": {Domain: "domain2", InstanceID: "instance3", HostName: "host3"},
		},
	}, domainMap)
}
