/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestSortHostsByName(t *testing.T) {
	hosts := []*topology.HostInfo{
		{HostName: "z"},
		{HostName: "a"},
		{HostName: "m"},
	}
	sortHostsByName(hosts)
	require.Equal(t, []string{"a", "m", "z"}, []string{hosts[0].HostName, hosts[1].HostName, hosts[2].HostName})
}

// TestBaseBlockFillsSlotLeftToRight verifies that hosts fill base block slots left to
// right and that slots beyond the provided hosts remain empty placeholders.
func TestBaseBlockFillsSlotLeftToRight(t *testing.T) {
	bb := newBaseBlock("B1", []*topology.HostInfo{
		{HostName: "n0", Domain: "B1"},
		{HostName: "n1", Domain: "B1"},
	}, 4)

	require.Len(t, bb.leaves, 4)
	require.NotNil(t, bb.leaves[0].host)
	require.Equal(t, "n0", bb.leaves[0].host.HostName)
	require.NotNil(t, bb.leaves[1].host)
	require.Equal(t, "n1", bb.leaves[1].host.HostName)
	require.Nil(t, bb.leaves[2].host)
	require.Nil(t, bb.leaves[3].host)
}

// TestBaseBlocksFromDomainsMultiplePerdomain verifies that multiple hosts under
// the same domainID are packed into a single base block.
func TestBaseBlocksFromDomainsMultiplePerdomain(t *testing.T) {
	domains := topology.NewDomainMap()
	domains.AddHostInfo(&topology.HostInfo{
		Domain:   "B1",
		HostName: "n0",
	})
	domains.AddHostInfo(&topology.HostInfo{
		Domain:   "B1",
		HostName: "n1",
	})

	packed := packDomainsIntoBaseBlocks(domains, 2, 0)
	require.Len(t, packed, 1)
	require.Equal(t, "B1", packed[0].id)
	require.Len(t, hostNamesFromLeaves(packed[0].leaves), 2)
}

// TestPackKeepsdomainsIndependent verifies that domains are never merged
// together even when each has fewer hosts than baseBlockSize. Each domain is
// packed independently into its own base block(s), with no cross-domain combining.
func TestPackKeepsdomainsIndependent(t *testing.T) {
	domains := topology.NewDomainMap()
	for _, accel := range []string{"B1", "B2", "B3"} {
		for j := range 3 {
			domains.AddHostInfo(&topology.HostInfo{
				Domain:   accel,
				HostName: fmt.Sprintf("%s-n%d", accel, j),
			})
		}
	}

	packed := packDomainsIntoBaseBlocks(domains, 8, 0)
	require.Len(t, packed, 3)
	require.Equal(t, "B1", packed[0].id)
	require.Len(t, hostNamesFromLeaves(packed[0].leaves), 3)
	require.Equal(t, "B2", packed[1].id)
	require.Len(t, hostNamesFromLeaves(packed[1].leaves), 3)
	require.Equal(t, "B3", packed[2].id)
	require.Len(t, hostNamesFromLeaves(packed[2].leaves), 3)
}

// TestPackSplitsWhenHostsExceedBlockSize verifies that a single domain with more
// hosts than baseBlockSize is split into multiple base blocks with "#N" ID suffixes.
func TestPackSplitsWhenHostsExceedBlockSize(t *testing.T) {
	domains := topology.NewDomainMap()
	for i := range 10 {
		domains.AddHostInfo(&topology.HostInfo{
			Domain:   "B1",
			HostName: fmt.Sprintf("n%d", i),
		})
	}
	packed := packDomainsIntoBaseBlocks(domains, 4, 0)
	require.Len(t, packed, 3)
	require.Len(t, hostNamesFromLeaves(packed[0].leaves), 4)
	require.Len(t, hostNamesFromLeaves(packed[2].leaves), 2)
}

// TestShapedBlockTreeSlots verifies that two domains fill the two available tree
// slots in sorted order when the tree has exactly the needed capacity.
func TestShapedBlockTreeSlots(t *testing.T) {
	domains := topology.NewDomainMap()
	domains.AddHostInfo(&topology.HostInfo{
		Domain:     "B1",
		HostName:   "n1",
		InstanceID: "i1",
	})
	domains.AddHostInfo(&topology.HostInfo{
		Domain:     "B3",
		HostName:   "n3",
		InstanceID: "i3",
	})

	fanouts, ok := fanoutsPerLevel([]int{4, 8})
	require.True(t, ok)
	const baseBlockSize = 4
	packed := packDomainsIntoBaseBlocks(domains, baseBlockSize, 0)
	expandedFanouts := expandFanoutsForCapacity(fanouts, len(packed))
	tree := buildAggregateShape(expandedFanouts, baseBlockSize)
	mergeBaseBlocksIntoTree(tree, packed)
	slots := collectBaseBlockSlots(tree)
	require.Len(t, slots, 2)
	require.Equal(t, "B1", slots[0].domainIdentifier())
	require.Equal(t, "B3", slots[1].domainIdentifier())
}

// TestBlocksFromShapedTreeFillsSequentially verifies that blocks fill left-to-right
// in sorted domain order regardless of domain ID format. Each domain
// has baseBlockSize hosts so they pack independently (no merging).
func TestBlocksFromShapedTreeFillsSequentially(t *testing.T) {
	fanouts, ok := fanoutsPerLevel([]int{4, 8, 16})
	require.True(t, ok)
	domains := topology.NewDomainMap()
	accels := []string{"gpu-clique-a", "gpu-clique-b", "gpu-clique-c"}
	for _, accel := range accels {
		for i := range 4 {
			domains.AddHostInfo(&topology.HostInfo{
				Domain:   accel,
				HostName: fmt.Sprintf("%s-n%d", accel, i),
			})
		}
	}

	const baseBlockSize = 4
	packed := packDomainsIntoBaseBlocks(domains, baseBlockSize, 0)
	expandedFanouts := expandFanoutsForCapacity(fanouts, len(packed))
	tree := buildAggregateShape(expandedFanouts, baseBlockSize)
	mergeBaseBlocksIntoTree(tree, packed)
	byName := map[string]*blockInfo{
		"gpu-clique-a": {name: "gpu-clique-a", nodes: []string{"gpu-clique-a-n0"}},
		"gpu-clique-b": {name: "gpu-clique-b", nodes: []string{"gpu-clique-b-n0"}},
		"gpu-clique-c": {name: "gpu-clique-c", nodes: []string{"gpu-clique-c-n0"}},
	}
	out := blocksFromShapedTree(tree, byName, 3)
	require.Len(t, out, 3)
	require.Equal(t, "gpu-clique-a", out[0].name)
	require.Equal(t, "gpu-clique-b", out[1].name)
	require.Equal(t, "gpu-clique-c", out[2].name)
}


// TestSplitIntoBaseBlocksChunksExcessHosts verifies that 12 hosts with a blockSize of 4
// produce exactly 3 blocks, each fully populated, filling slots left-to-right.
func TestSplitIntoBaseBlocksChunksExcessHosts(t *testing.T) {
	hosts := make([]*topology.HostInfo, 12)
	for i := range 12 {
		hosts[i] = &topology.HostInfo{
			HostName: fmt.Sprintf("n%d", i),
			Domain:   "B1",
		}
	}
	sortHostsByName(hosts)
	blocks := splitIntoBaseBlocks("B1", hosts, 4)
	require.Len(t, blocks, 3)
	require.Len(t, blocks[0].leaves, 4)
	require.Len(t, hostNamesFromLeaves(blocks[0].leaves), 4)
	require.Len(t, hostNamesFromLeaves(blocks[1].leaves), 4)
	require.Len(t, hostNamesFromLeaves(blocks[2].leaves), 4)
}

func TestExpandFanoutsForCapacity(t *testing.T) {
	require.Equal(t, 4, totalBaseBlockSlots([]int{2, 2}))
	require.Equal(t, []int{2, 8}, expandFanoutsForCapacity([]int{2, 2}, 12))
	require.Equal(t, 16, totalBaseBlockSlots(expandFanoutsForCapacity([]int{2, 2}, 12)))
	// Empty fanout slice must not panic.
	require.Equal(t, []int(nil), expandFanoutsForCapacity(nil, 4))
	require.Equal(t, []int{}, expandFanoutsForCapacity([]int{}, 4))
	// Non-positive required must return fanouts unchanged.
	require.Equal(t, []int{2, 2}, expandFanoutsForCapacity([]int{2, 2}, 0))
	require.Equal(t, []int{2, 2}, expandFanoutsForCapacity([]int{2, 2}, -1))
}

// TestShapedTreeExpandsForExcessHosts verifies that when required base blocks exceed
// the initial tree capacity, the last fanout tier is doubled until all hosts fit.
func TestShapedTreeExpandsForExcessHosts(t *testing.T) {
	domains := topology.NewDomainMap()
	for i := range 12 {
		domains.AddHostInfo(&topology.HostInfo{
			Domain:   "B1",
			HostName: fmt.Sprintf("n%d", i),
		})
	}
	fanouts, ok := fanoutsPerLevel([]int{4, 8, 16})
	require.True(t, ok)
	const baseBlockSize = 4
	packed := packDomainsIntoBaseBlocks(domains, baseBlockSize, 0)
	expandedFanouts := expandFanoutsForCapacity(fanouts, len(packed))
	tree := buildAggregateShape(expandedFanouts, baseBlockSize)
	mergeBaseBlocksIntoTree(tree, packed)
	slots := collectBaseBlockSlots(tree)
	require.GreaterOrEqual(t, len(slots), 3)
	var allNodes []string
	for _, s := range slots {
		allNodes = append(allNodes, hostNamesFromLeaves(s.leaves)...)
	}
	require.Len(t, allNodes, 12)
}
