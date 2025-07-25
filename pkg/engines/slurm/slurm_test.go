/*
 * Copyright (c) 2024-2025, NVIDIA CORPORATION.  All rights reserved.
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

package slurm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

func TestParseFakeNodes(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		out  string
		err  string
	}{
		{
			name: "Case 1: no nodes",
			err:  "fake partition has no nodes",
		},
		{
			name: "Case 2: valid input",
			in: `PartitionName=fake
   AllowQos=ALL
   DefaultTime=NONE DisableRootJobs=NO ExclusiveUser=NO GraceTime=0 Hidden=NO
   MaxNodes=UNLIMITED MaxTime=08:00:00 MinNodes=1 LLN=NO MaxCPUsPerNode=UNLIMITED MaxCPUsPerSocket=UNLIMITED
   Nodes=fake-[01-16]
   OverTimeLimit=NONE PreemptMode=OFF
   JobDefaults=(null)
   DefMemPerNode=UNLIMITED MaxMemPerNode=UNLIMITED
`,
			out: "fake-[01-16]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := parseFakeNodes(tc.in)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.out, out)
			}
		})
	}
}

func TestParseTopologyNodes(t *testing.T) {
	input := `BlockName=nvlblk01-lyris BlockIndex=0 Nodes=lyris[0001-0018] BlockSize=18
BlockName=nvlblk02-lyris BlockIndex=1 Nodes=lyris[0019-0036] BlockSize=18
BlockName=nvlblk03-lyris BlockIndex=2 Nodes=lyris[0037-0054] BlockSize=18
BlockName=nvlblk04-lyris BlockIndex=3 Nodes=lyris[0055-0072] BlockSize=18
BlockName=nvlblk05-lyris BlockIndex=4 Nodes=lyris[0073-0090] BlockSize=18
BlockName=nvlblk06-lyris BlockIndex=5 Nodes=lyris[0091-0108] BlockSize=18
BlockName=nvlblk07-lyris BlockIndex=6 Nodes=lyris[0109-0126] BlockSize=18
BlockName=nvlblk08-lyris BlockIndex=7 Nodes=lyris[0127-0144] BlockSize=18
AggregatedBlock=nvlblk01-lyris,nvlblk02-lyris BlockIndex=8 Nodes=lyris[0001-0036] BlockSize=36
AggregatedBlock=nvlblk03-lyris,nvlblk04-lyris BlockIndex=9 Nodes=lyris[0037-0072] BlockSize=36
AggregatedBlock=nvlblk05-lyris,nvlblk06-lyris BlockIndex=10 Nodes=lyris[0073-0108] BlockSize=36
AggregatedBlock=nvlblk07-lyris,nvlblk08-lyris BlockIndex=11 Nodes=lyris[0109-0144] BlockSize=36
`
	expected := []string{"lyris[0001-0144]"}
	actual, _ := parseTopologyNodes(input)
	require.Equal(t, expected, actual)
}

func TestGetParams(t *testing.T) {
	testCases := []struct {
		name   string
		in     string
		params *Params
		err    string
	}{
		{
			name: "Case1: bad input",
			in:   `{"topologies": "bad"}`,
			err:  "could not decode configuration: 1 error(s) decoding:\n\n* 'topologies' expected a map, got 'string'",
		},
		{
			name: "Case2: valid input",
			in: `
{
  "plugin": "123",
  "fakeNodesEnabled": true,
  "topologies": {
	"topo1": {
	  "plugin": "topology/block",
	  "block_sizes": "2,4"
	},
	"topo2": {
	  "plugin": "topology/block",
	  "block_sizes": "8,16",
	  "nodes": ["n1", "n2", "n3"]
	},
	"topo3": {
	  "plugin": "topology/flat",
	  "cluster_default": true
	}
  }
}
`,
			params: &Params{
				BaseParams: BaseParams{
					Plugin:           "123",
					FakeNodesEnabled: true,
					Topologies: map[string]*Topology{
						"topo1": {
							Plugin:     "topology/block",
							BlockSizes: "2,4",
						},
						"topo2": {
							Plugin:     "topology/block",
							BlockSizes: "8,16",
							Nodes: []string{
								"n1",
								"n2",
								"n3",
							},
						},
						"topo3": {
							Plugin:  "topology/flat",
							Default: true,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result map[string]any

			err := json.Unmarshal([]byte(tc.in), &result)
			require.NoError(t, err, "failed to unmarshal")

			params, err := getParams(result)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.params, params)
			}
		})
	}
}

func TestGetTranslateConfig(t *testing.T) {
	ctx := context.TODO()
	testCases := []struct {
		name   string
		params *BaseParams
		cfg    *translate.Config
	}{
		{
			name: "Case 1: minimal input",
			params: &BaseParams{
				Plugin: topology.TopologyTree,
			},
			cfg: &translate.Config{
				Plugin: topology.TopologyTree,
			},
		},
		{
			name: "Case 2: invalid blocksize",
			params: &BaseParams{
				Plugin:     topology.TopologyBlock,
				BlockSizes: "bad",
			},
			cfg: &translate.Config{
				Plugin: topology.TopologyBlock,
			},
		},
		{
			name: "Case 3: valid blocksize",
			params: &BaseParams{
				Plugin:     topology.TopologyBlock,
				BlockSizes: "2,4,8",
			},
			cfg: &translate.Config{
				Plugin:     topology.TopologyBlock,
				BlockSizes: []int{2, 4, 8},
			},
		},
		{
			name: "Case 4: with fake nodes",
			params: &BaseParams{
				Plugin:           topology.TopologyBlock,
				BlockSizes:       "2,4,8",
				FakeNodesEnabled: true,
				FakeNodePool:     "fake[001-100]",
			},
			cfg: &translate.Config{
				Plugin:       topology.TopologyBlock,
				BlockSizes:   []int{2, 4, 8},
				FakeNodePool: "fake[001-100]",
			},
		},
		{
			name: "Case 4: with fake nodes",
			params: &BaseParams{
				Topologies: map[string]*Topology{
					"topo": {
						Plugin: topology.TopologyBlock,
						Nodes:  []string{"node[001-100]"},
					},
				},
			},
			cfg: &translate.Config{
				Topologies: map[string]*translate.TopologySpec{
					"topo": {
						Plugin: topology.TopologyBlock,
						Nodes:  []string{"node[001-100]"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := GetTranslateConfig(ctx, tc.params)
			require.Equal(t, tc.cfg, cfg)
		})
	}
}
