/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlockTopology(t *testing.T) {
	testCases := []struct {
		name   string
		nt     *NetworkTopology
		output string
		err    string
	}{
		{
			name: "Case 1: a block without name",
			nt: &NetworkTopology{
				config: &Config{
					BlockSizes: []int{2},
				},
				blocks: []*blockInfo{
					{
						id:    "b1",
						nodes: []string{"n1", "n2"},
					},
				},
			},
			output: strings.Join([]string{
				"BlockName=b1 Nodes=n[1-2]",
				"BlockSizes=2",
				"",
			}, "\n"),
		},
		{
			name: "Case 2: a block with name",
			nt: &NetworkTopology{
				config: &Config{
					BlockSizes: []int{2},
				},
				blocks: []*blockInfo{
					{
						id:    "block001",
						name:  "nvl1",
						nodes: []string{"n1", "n2"},
					},
				},
			},
			output: `# block001=nvl1
BlockName=block001 Nodes=n[1-2]
BlockSizes=2
`,
		},
		{
			name: "Case 3: multiple blocks with mixed settings with blockSizes",
			nt: &NetworkTopology{
				config: &Config{
					BlockSizes: []int{2, 4},
				},
				blocks: []*blockInfo{
					{
						id:    "b1",
						nodes: []string{"n1", "n2"},
					},
					{
						id:    "b2",
						name:  "block2",
						nodes: []string{"n3"},
					},
				},
			},
			output: `BlockName=b1 Nodes=n[1-2]
# b2=block2
BlockName=b2 Nodes=n3
BlockSizes=2,4
`,
		},
		{
			name: "Case 4: multiple blocks with mixed settings without blockSizes",
			nt: &NetworkTopology{
				config: &Config{},
				blocks: []*blockInfo{
					{
						id:    "b1",
						nodes: []string{"n1", "n2"},
					},
					{
						id:    "b2",
						name:  "block2",
						nodes: []string{"n3"},
					},
				},
			},
			output: `BlockName=b1 Nodes=n[1-2]
# b2=block2
BlockName=b2 Nodes=n3
BlockSizes=1,2
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tc.nt.toBlockTopology(&buf, false)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.output, buf.String())
			}
		})
	}
}

func TestGetBlockSizes(t *testing.T) {
	testCases := []struct {
		name           string
		blocks         map[string]int
		blockSize      []int
		expectedOutput []int
	}{
		{
			name: "Case 1: #nodes/block same, #blocks power of 2, blockSizes not requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 2: #nodes/block different, #blocks power of 2, blockSizes not requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 3: #nodes/block same, #blocks !power of 2, blockSizes not requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			expectedOutput: []int{2, 4},
		},
		{
			name: "Case 4: #nodes/block same, #blocks power of 2, blockSizes requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 5: #nodes/block different, #blocks power of 2, blockSizes requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 6: #nodes/block same, #blocks !power of 2, blockSizes requested",
			blocks: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			blockSize:      []int{2},
			expectedOutput: []int{2},
		},
		{
			name: "Case 7: #nodes/block same, #blocks power of 2, blockSizes requested",
			blocks: map[string]int{
				"nvl1": 3,
				"nvl2": 3,
				"nvl3": 3,
				"nvl4": 3,
			},
			blockSize:      []int{3, 6, 12},
			expectedOutput: []int{3, 6, 12},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blockSize := getBlockSizes(populateBlockInfo(tc.blocks), tc.blockSize)
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
