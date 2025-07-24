/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetBlockSize(t *testing.T) {
	testCases := []struct {
		name           string
		blocks         map[string]int
		blockSize      []int
		useFake        bool
		expectedOutput []int
	}{
		{
			name: "Case 1: #nodes/block same, #blocks power of 2, admin !provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 2: #nodes/block different, #blocks power of 2, admin !provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 3: #nodes/block same, #blocks !power of 2, admin !provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 4: #nodes/block same, #blocks power of 2, admin provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 5: #nodes/block different, #blocks power of 2, admin provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 6: #nodes/block same, #blocks !power of 2, admin provided base block size",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 7: #nodes/block same, #blocks power of 2, admin provided blocksizes",
			blocks: map[string]int{
				"nvl1": 3,
				"nvl2": 3,
				"nvl3": 3,
				"nvl4": 3,
			},
			blockSize:      []int{3, 6, 12},
			expectedOutput: []int{3, 6, 12},
		},
		{
			name: "Case 8: #nodes/block different, #blocks power of 2, admin provided wrong base blocksize",
			blocks: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
				"nvl4": 4,
			},
			blockSize:      []int{4},
			expectedOutput: []int{3, 6, 12},
		},
		{
			name: "Case 9: #nodes/block different, #blocks !power of 2, admin provided wrong blocksizes",
			blocks: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
			},
			blockSize:      []int{3, 4},
			expectedOutput: []int{3, 6},
		},
		{
			name: "Case 10: #nodes/block same, #blocks power of 2, admin provided larger base blocksize",
			blocks: map[string]int{
				"nvl1": 4,
				"nvl2": 4,
				"nvl3": 4,
				"nvl4": 4,
			},
			blockSize:      []int{10},
			expectedOutput: []int{4, 8, 16},
		},
		{
			name: "Case 11: #nodes/block different, #blocks power of 2, admin provided smaller base blocksize",
			blocks: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
				"nvl4": 4,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name:    "Case 12: with fake nodes, #nodes = base block size, no requested block sizes",
			useFake: true,
			blocks: map[string]int{
				"nvl1": 18,
				"nvl2": 18,
			},
			expectedOutput: []int{18, 36},
		},
		{
			name:    "Case 13: with fake nodes, mixed #nodes, no requested block sizes",
			useFake: true,
			blocks: map[string]int{
				"nvl1": 12,
				"nvl2": 18,
			},
			expectedOutput: []int{12, 24},
		},
		{
			name:    "Case 14: with fake nodes, requested base block size > #nodes",
			useFake: true,
			blocks: map[string]int{
				"nvl1": 12,
				"nvl2": 12,
			},
			blockSize:      []int{18, 36},
			expectedOutput: []int{18, 36},
		},
		{
			name:    "Case 15: with fake nodes, requested base block size > #nodes",
			useFake: true,
			blocks: map[string]int{
				"nvl1": 12,
				"nvl2": 12,
			},
			blockSize:      []int{18},
			expectedOutput: []int{18},
		},
		{
			name:    "Case 16: with fake nodes, requested base block size < #nodes",
			useFake: true,
			blocks: map[string]int{
				"nvl1": 18,
				"nvl2": 18,
			},
			blockSize:      []int{15},
			expectedOutput: []int{15},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blockSize := getBlockSize(populateBlockInfo(tc.blocks), tc.blockSize, tc.useFake)
			require.Equal(t, tc.expectedOutput, blockSize)
		})
	}
}

func populateBlockInfo(blocks map[string]int) []*blockInfo {
	result := make([]*blockInfo, 0, len(blocks))

	for name, numNodes := range blocks {
		info := &blockInfo{
			id:    fmt.Sprintf("block-%s", name),
			name:  name,
			nodes: make([]string, 0, numNodes),
		}

		for node := range numNodes {
			info.nodes = append(info.nodes, fmt.Sprintf("%d", node))
		}

		result = append(result, info)
	}
	return result
}
