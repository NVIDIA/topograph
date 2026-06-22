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
