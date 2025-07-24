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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	testTreeConfig = `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=Node[201-202,205]
SwitchName=S3 Nodes=Node[304-306]
`

	testBlockConfig1_1 = `BlockName=B1 Nodes=Node[104-106]
BlockName=B2 Nodes=Node[201-202,205]
BlockSizes=3
`

	testBlockConfig1_2 = `BlockName=B2 Nodes=Node[201-202,205]
BlockName=B1 Nodes=Node[104-106]
BlockSizes=3
`

	testBlockConfigDiffNumNodes = `BlockName=B2 Nodes=Node[201-202,205-206]
BlockName=B1 Nodes=Node[104-106]
BlockSizes=3,6
`

	testBlockConfig2 = `BlockName=B2 Nodes=Node[201-202,205]
BlockName=B1 Nodes=Node[104-106]
BlockName=B4 Nodes=Node[401-403]
BlockName=B3 Nodes=Node[301-303]
BlockSizes=3
`

	testBlockConfigDFS = `BlockName=B3 Nodes=Node205
BlockName=B2 Nodes=Node[104-105]
BlockName=B1 Nodes=Node202
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

	testBlockConfigFakeNodes = `BlockName=B3 Nodes=Node205,fake[100-101]
BlockName=B2 Nodes=Node[104-105],fake102
BlockName=B1 Nodes=Node202,fake[103-104]
BlockSizes=3
`
)

func TestValidateConfig(t *testing.T) {
	emptyRoot := &topology.Vertex{Vertices: make(map[string]*topology.Vertex)}
	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: nil},
	}

	testCases := []struct {
		name string
		root *topology.Vertex
		cfg  *Config
		err  string
	}{
		{
			name: "Case 1: empty config",
			root: emptyRoot,
			cfg:  &Config{},
			err:  `unsupported topology plugin ""`,
		},
		{
			name: "Case 2: missing tree root",
			root: emptyRoot,
			cfg: &Config{
				Plugin: topology.TopologyTree,
			},
			err: "missing tree topology",
		},
		{
			name: "Case 3: missing block root",
			root: emptyRoot,
			cfg: &Config{
				Plugin: topology.TopologyBlock,
			},
			err: "missing block topology",
		},
		{
			name: "Case 4: mutually exclusive parameters",
			root: emptyRoot,
			cfg: &Config{
				Plugin:     topology.TopologyTree,
				Topologies: map[string]*TopologySpec{"topo": nil},
			},
			err: "plugin and topologies parameters are mutually exclusive",
		},
		{
			name: "Case 5: missing plugin in topology spec",
			root: emptyRoot,
			cfg: &Config{
				Topologies: map[string]*TopologySpec{
					"topo": {}},
			},
			err: `unsupported topology plugin "" for topology "topo"`,
		},
		{
			name: "Case 6: missing tree root in topology spec",
			root: emptyRoot,
			cfg: &Config{
				Topologies: map[string]*TopologySpec{
					"topo": {Plugin: topology.TopologyTree}},
			},
			err: `missing tree topology for topology "topo"`,
		},
		{
			name: "Case 7: missing block root in topology spec",
			root: emptyRoot,
			cfg: &Config{
				Topologies: map[string]*TopologySpec{
					"topo": {Plugin: topology.TopologyBlock}},
			},
			err: `missing block topology for topology "topo"`,
		},
		{
			name: "Case 8: missing nodes in topology spec",
			root: blockRoot,
			cfg: &Config{
				Topologies: map[string]*TopologySpec{
					"topo": {Plugin: topology.TopologyBlock}},
			},
			err: `topology "topo" specifies no nodes`,
		},
		{
			name: "Case 8: missing nodes in topology spec",
			root: emptyRoot,
			cfg: &Config{
				Topologies: map[string]*TopologySpec{
					"topo": {Plugin: topology.TopologyFlat}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewNetworkTopology(tc.root, tc.cfg)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}

}

func TestToTreeTopology(t *testing.T) {
	v, _ := GetTreeTestSet(false)
	cfg := &Config{
		Plugin: topology.TopologyTree,
	}
	nt, _ := NewNetworkTopology(v, cfg)
	expected := map[string][]string{
		"":    {"S1"},
		"S1":  {"S2", "S3"},
		"S2":  {"I21", "I22", "I25"},
		"S3":  {"I34", "I35", "I36"},
		"I21": {},
		"I22": {},
		"I25": {},
		"I34": {},
		"I35": {},
		"I36": {},
	}
	require.Equal(t, expected, nt.tree)

	part := nt.getPartitionTree([]string{"I34", "I35"})
	expected = map[string][]string{
		"":   {"S1"},
		"S1": {"S3"},
		"S3": {"I34", "I35"},
	}

	require.Equal(t, expected, part)

	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testTreeConfig, buf.String())
}

func TestToBlockTopology(t *testing.T) {
	v, _ := getBlockTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{3},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfig1_1, buf.String())
}

func TestToBlockMultiIBTopology(t *testing.T) {
	v, _ := GetBlockWithMultiIBTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{3},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfig2, buf.String())
}

func TestToBlockIBTopology(t *testing.T) {
	v, _ := getBlockWithIBTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{3},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfig1_2, buf.String())
}

func TestToBlockDiffNumNode(t *testing.T) {
	v, _ := getBlockWithDiffNumNodeTestSet()
	cfg := &Config{
		Plugin: topology.TopologyBlock,
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfigDiffNumNodes, buf.String())
}

func TestToBlockDFSIBTopology(t *testing.T) {
	v, _ := getBlockWithDFSIBTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{1},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfigDFS, buf.String())
}

func TestBlockFakeNodes(t *testing.T) {
	// Test Fake node config
	fakeNodeData := "fake[100-998]"
	fnc := getFakeNodeConfig(fakeNodeData)

	expectedFnc := &fakeNodeConfig{
		nodes: []string{},
		index: 0,
	}
	for i := 100; i <= 998; i++ {
		expectedFnc.nodes = append(expectedFnc.nodes, fmt.Sprintf("fake%d", i))
	}
	require.Equal(t, expectedFnc, fnc)

	// Test Fake node output
	v, _ := getBlockWithDFSIBTestSet()
	cfg := &Config{
		Plugin:       topology.TopologyBlock,
		FakeNodePool: fakeNodeData,
		BlockSizes:   []int{3},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testBlockConfigFakeNodes, buf.String())
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
									"node-1-id": {
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
									"node-2-id": {
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

	cfg := &Config{
		Plugin: topology.TopologyTree,
	}
	nt, _ := NewNetworkTopology(root, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, shortNameExpectedResult, buf.String())
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
		Vertices: map[string]*topology.Vertex{
			topology.TopologyBlock: blockRoot,
			topology.TopologyTree:  treeRoot,
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
		Vertices: map[string]*topology.Vertex{
			topology.TopologyBlock: blockRoot,
			topology.TopologyTree:  treeRoot,
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
		Vertices: map[string]*topology.Vertex{
			topology.TopologyBlock: blockRoot,
			topology.TopologyTree:  treeRoot,
		},
	}
	return root, instance2node
}
