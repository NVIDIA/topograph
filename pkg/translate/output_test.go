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

package translate

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	testTreeConfig = `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=Node[201-202,205]
SwitchName=S3 Nodes=Node[304-306]
`

	testBlockConfig = `BlockName=B1 Nodes=Node[104-106]
BlockName=B2 Nodes=Node[201-202,205]
BlockSizes=3
`

	testBlockConfigDiffNumNodes = `BlockName=B1 Nodes=Node[104-106]
BlockName=B2 Nodes=Node[201-202,205-206]
BlockSizes=3,6
`

	testBlockConfig2 = `BlockName=B3 Nodes=Node[301-303]
BlockName=B4 Nodes=Node[401-403]
BlockName=B1 Nodes=Node[104-106]
BlockName=B2 Nodes=Node[201-202,205]
BlockSizes=3
`

	testBlockConfigDFS = `BlockName=B1 Nodes=Node202
BlockName=B2 Nodes=Node[104-105]
BlockName=B3 Nodes=Node205
BlockSizes=1
`

	shortNameExpectedResult = `# switch.3.1=hpcislandid-1
SwitchName=switch.3.1 Switches=switch.2.[1-2]
# switch.2.1=network-block-1
SwitchName=switch.2.1 Switches=switch.1.1
# switch.2.2=network-block-2
SwitchName=switch.2.2 Switches=switch.1.2
# switch.1.1=local-block-1
SwitchName=switch.1.1 Nodes=node-1
# switch.1.2=local-block-2
SwitchName=switch.1.2 Nodes=node-2
`

	slurmFileData = `NodeName=fake[100-998] RealMemory=65238 Boards=1 SocketsPerBoard=1 CoresPerSocket=16 ThreadsPerCore=1 Features=location=us-central1-b,CPU State=FUTURE
PartitionName="cpu-small" MinNodes=1 DefaultTime=8:00:00 MaxTime=8:00:00 AllowGroups=ALL PriorityJobFactor=1 PriorityTier=1 OverSubscribe=NO PreemptMode=OFF QOS=20_cpus_per_user AllowAccounts=ALL AllowQos=ALL TRESBillingWeights=CPU=0.0000001,Mem=0.000000015G Nodes=cpu-small-[001,002],fake[001-998]
`

	testBlockConfigFakeNodes = `BlockName=B1 Nodes=Node202, fake[100-101]
BlockName=B2 Nodes=Node[104-105], fake102
BlockName=B3 Nodes=Node205, fake[103-104]
BlockSizes=3
`
)

func TestToTreeTopology(t *testing.T) {
	v, _ := GetTreeTestSet(false)
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	require.Equal(t, testTreeConfig, buf.String())
}

func TestToBlockTopology(t *testing.T) {
	v, _ := getBlockTestSet()
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	require.Equal(t, testBlockConfig, buf.String())
}

func TestToBlockMultiIBTopology(t *testing.T) {
	v, _ := GetBlockWithMultiIBTestSet()
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testBlockConfig2:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func TestToBlockIBTopology(t *testing.T) {
	v, _ := getBlockWithIBTestSet()
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testBlockConfig:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func TestToBlockDiffNumNode(t *testing.T) {
	v, _ := getBlockWithDiffNumNodeTestSet()
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testBlockConfigDiffNumNodes:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func TestToBlockDFSIBTopology(t *testing.T) {
	v, _ := getBlockWithDFSIBTestSet()
	buf := &bytes.Buffer{}
	err := Write(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testBlockConfigDFS:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func TestToSlurmNameShortener(t *testing.T) {
	v := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{
			"hpcislandid-1": {
				ID:   "hpcislandid-1",
				Name: "switch.3.1",
				Vertices: map[string]*topology.Vertex{
					"network-block-1": {
						ID:   "network-block-1",
						Name: "switch.2.1",
						Vertices: map[string]*topology.Vertex{
							"local-block-1": {
								ID:   "local-block-1",
								Name: "switch.1.1",
								Vertices: map[string]*topology.Vertex{
									"node-1": {
										ID:   "node-1-id",
										Name: "node-1",
									},
								},
							},
						},
					},
					"network-block-2": {
						ID:   "network-block-2",
						Name: "switch.2.2",
						Vertices: map[string]*topology.Vertex{
							"local-block-2": {
								ID:   "local-block-2",
								Name: "switch.1.2",
								Vertices: map[string]*topology.Vertex{
									"node-2": {
										ID:   "node-2-id",
										Name: "node-2",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyTree: v},
	}

	buf := &bytes.Buffer{}
	err := Write(buf, root)
	require.NoError(t, err)
	require.Equal(t, shortNameExpectedResult, buf.String())
}

func TestGetBlockSize(t *testing.T) {
	testCases := []struct {
		name           string
		domainVisited  map[string]int
		adminBlockSize string
		expectedOutput string
	}{
		{
			name: "Case 1: #nodes/block same, #blocks power of 2, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			adminBlockSize: "",
			expectedOutput: "2,4",
		},
		{
			name: "Case 2: #nodes/block different, #blocks power of 2, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			adminBlockSize: "",
			expectedOutput: "2,4",
		},
		{
			name: "Case 3: #nodes/block same, #blocks !power of 2, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			adminBlockSize: "",
			expectedOutput: "2,4",
		},
		{
			name: "Case 4: #nodes/block same, #blocks power of 2, admin provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
			},
			adminBlockSize: "2",
			expectedOutput: "2",
		},
		{
			name: "Case 5: #nodes/block different, #blocks power of 2, admin provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 3,
			},
			adminBlockSize: "2",
			expectedOutput: "2",
		},
		{
			name: "Case 6: #nodes/block same, #blocks !power of 2, admin provided base block size",
			domainVisited: map[string]int{
				"nvl1": 2,
				"nvl2": 2,
				"nvl3": 2,
			},
			adminBlockSize: "2",
			expectedOutput: "2",
		},
		{
			name: "Case 7: #nodes/block same, #blocks power of 2, admin provided blocksizes",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 3,
				"nvl3": 3,
				"nvl4": 3,
			},
			adminBlockSize: "3,6,12",
			expectedOutput: "3,6,12",
		},
		{
			name: "Case 8: #nodes/block different, #blocks power of 2, admin provided wrong base blocksize",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
				"nvl4": 4,
			},
			adminBlockSize: "4",
			expectedOutput: "3,6,12",
		},
		{
			name: "Case 9: #nodes/block different, #blocks !power of 2, admin provided wrong blocksizes",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
			},
			adminBlockSize: "3,4",
			expectedOutput: "3,6",
		},
		{
			name: "Case 10: #nodes/block different, #blocks !power of 2, admin blocksizes parse error",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
			},
			adminBlockSize: "a,4",
			expectedOutput: "3,6",
		},
		{
			name: "Case 11: #nodes/block different, #blocks !power of 2, admin blocksizes parse error",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
			},
			adminBlockSize: "3,a",
			expectedOutput: "3,6",
		},
		{
			name: "Case 12: #nodes/block same, #blocks power of 2, admin provided larger base blocksize",
			domainVisited: map[string]int{
				"nvl1": 4,
				"nvl2": 4,
				"nvl3": 4,
				"nvl4": 4,
			},
			adminBlockSize: "10",
			expectedOutput: "4,8,16",
		},
		{
			name: "Case 13: #nodes/block different, #blocks power of 2, admin provided smaller base blocksize",
			domainVisited: map[string]int{
				"nvl1": 3,
				"nvl2": 4,
				"nvl3": 3,
				"nvl4": 4,
			},
			adminBlockSize: "2",
			expectedOutput: "2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := getBlockSize(getBlockRoot(tc.domainVisited), tc.adminBlockSize, nil)
			require.Equal(t, tc.expectedOutput, got)
		})
	}
}

func getBlockWithIBTestSet() (*topology.Vertex, map[string]string) {
	//
	//     ibRoot1
	//        |
	//        S1
	//      /    \
	//    S2      S3
	//    |       |
	//   ---     ---
	//   I14\    I21\
	//   I15-B1  I22-B2
	//   I16/    I25/
	//   ---     ---
	//
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S2": sw2, "S3": sw3},
	}
	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S1": sw1},
	}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}

func getBlockWithDFSIBTestSet() (*topology.Vertex, map[string]string) {
	//
	//     		 ibRoot1
	//       /      |        \
	//   S1         S2         S3
	//   |          |          |
	//   S4        ---         S5
	//   |         I14\        |
	//  ---			  B2      ---
	//  I22-B1     I15/       I25-B3
	//  ---        ---        ---
	//
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105",
		"I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}

	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}

	sw4 := &topology.Vertex{
		ID:       "S4",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	sw5 := &topology.Vertex{
		ID:       "S5",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"S5": sw5},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S4": sw4},
	}

	sw0 := &topology.Vertex{
		ID:       "S0",
		Vertices: map[string]*topology.Vertex{"S1": sw1, "S2": sw2, "S3": sw3},
	}

	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S0": sw0},
	}

	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}
	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	block3 := &topology.Vertex{
		ID:       "B3",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2, "B3": block3},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "1",
		},
	}
	return root, instance2node
}

func getBlockTestSet() (*topology.Vertex, map[string]string) {
	//
	//	---        ---
	//   I14\      I21\
	//   I15-B1    I22-B2
	//   I16/      I25/
	//   ---       ---
	//
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot},
		Metadata: map[string]string{
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}

func getBlockWithDiffNumNodeTestSet() (*topology.Vertex, map[string]string) {
	//
	//     ibRoot1
	//        |
	//        S1
	//      /    \
	//    S2      S3
	//    |       |
	//   ---     ---
	//   I14\    I21\
	//   I15-B1  I22-B2
	//   I16/    I25  /
	//           I26 /
	//   ---     ---
	//
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205", "I26": "Node206",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}
	n26 := &topology.Vertex{ID: "I26", Name: "Node206"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	sw3 := &topology.Vertex{

		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25, "I26": n26},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S2": sw2, "S3": sw3},
	}
	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S1": sw1},
	}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25, "I26": n26},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyPlugin: topology.TopologyBlock,
		},
	}
	return root, instance2node
}

func getBlockRoot(domainVisited map[string]int) *topology.Vertex {
	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{},
	}
	for id, numNodes := range domainVisited {
		vertex := &topology.Vertex{
			ID:       fmt.Sprintf("block-%s", id),
			Name:     id,
			Vertices: make(map[string]*topology.Vertex),
		}

		for node := range numNodes {
			nodeid := strconv.Itoa(node)
			vertex.Vertices[nodeid] = &topology.Vertex{
				Name: nodeid,
				ID:   nodeid,
			}
		}

		blockRoot.Vertices[id] = vertex
	}
	return blockRoot
}
func TestGetBlockSizeWithFakeNodes(t *testing.T) {
	testCases := []struct {
		name           string
		domainVisited  map[string]int
		adminBlockSize string
		expectedOutput string
	}{
		{
			name: "Case 1: #nodes/block same, #nodes/block = 18, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 18,
				"nvl2": 18,
			},
			adminBlockSize: "",
			expectedOutput: "18,36",
		},
		{
			name: "Case 2: #nodes/block different, #nodes/block <= 18, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 12,
				"nvl2": 18,
			},
			adminBlockSize: "",
			expectedOutput: "18,36",
		},
		{
			name: "Case 3: #nodes/block same, #nodes/block < 18, admin !provided base block size",
			domainVisited: map[string]int{
				"nvl1": 12,
				"nvl2": 12,
			},
			adminBlockSize: "",
			expectedOutput: "18,36",
		},
		{
			name: "Case 4: #nodes/block same, #nodes/block < 18, admin provided base block size=18",
			domainVisited: map[string]int{
				"nvl1": 12,
				"nvl2": 12,
			},
			adminBlockSize: "18",
			expectedOutput: "18",
		},
		{
			name: "Case 5: #nodes/block same, #nodes/block = 18, admin provided base block size<18",
			domainVisited: map[string]int{
				"nvl1": 18,
				"nvl2": 18,
			},
			adminBlockSize: "15",
			expectedOutput: "15",
		},
	}

	fnc := &fakeNodeConfig{baseBlockSize: 0}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := getBlockSize(getBlockRoot(tc.domainVisited), tc.adminBlockSize, fnc)
			require.Equal(t, tc.expectedOutput, got)
		})
	}
}

func TestBlockFakeNodes(t *testing.T) {
	// Test Fake node config
	file, err := os.CreateTemp("", "test-fakecfg")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()

	_, err = file.WriteString(fmt.Sprintf(slurmFileData))
	require.NoError(t, err)

	fnc, err := getFakeNodeConfig(file.Name())
	require.NoError(t, err)

	expectedFnc := &fakeNodeConfig{
		startRange: 100,
		endRange:   998,
		lastUsed:   99,
	}
	require.Equal(t, expectedFnc, fnc)

	// Test Fake node output
	v, _ := getBlockWithFakeNodes(file.Name())
	buf := &bytes.Buffer{}
	err = Write(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testBlockConfigFakeNodes:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func getBlockWithFakeNodes(fileName string) (*topology.Vertex, map[string]string) {
	//
	//     		 ibRoot1
	//       /      |        \
	//   S1         S2         S3
	//   |          |          |
	//   S4        ---         S5
	//   |         I14\        |
	//  ---			  B2      ---
	//  I22-B1     I15/       I25-B3
	//  ---        ---        ---
	//
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105",
		"I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}

	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}

	sw4 := &topology.Vertex{
		ID:       "S4",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	sw5 := &topology.Vertex{
		ID:       "S5",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"S5": sw5},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S4": sw4},
	}

	sw0 := &topology.Vertex{
		ID:       "S0",
		Vertices: map[string]*topology.Vertex{"S1": sw1, "S2": sw2, "S3": sw3},
	}

	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S0": sw0},
	}

	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}
	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	block3 := &topology.Vertex{
		ID:       "B3",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2, "B3": block3},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyPlugin:           topology.TopologyBlock,
			topology.KeyBlockSizes:       "3",
			topology.KeyFakeNodesEnabled: "true",
			topology.KeySlurmFile:        fileName,
		},
	}
	return root, instance2node
}
