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

func TestParsePartitionNodes(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		out  []string
		err  string
	}{
		{
			name: "Case 1: no nodes",
			in: `PartitionName=my_partition
   AllowGroups=ALL AllowAccounts=ALL AllowQos=ALL
   AllocNodes=ALL Default=NO QoS=N/A
   DefaultTime=NONE DisableRootJobs=NO ExclusiveUser=NO ExclusiveTopo=NO GraceTime=0 Hidden=NO
   MaxNodes=UNLIMITED MaxTime=UNLIMITED MinNodes=0 LLN=NO MaxCPUsPerNode=UNLIMITED MaxCPUsPerSocket=UNLIMITED
   NodeSets=my_partition
   PriorityJobFactor=1 PriorityTier=1 RootOnly=NO ReqResv=NO OverSubscribe=NO
   OverTimeLimit=NONE PreemptMode=OFF
   State=UP TotalCPUs=384 TotalNodes=2 SelectTypeParameters=NONE
   JobDefaults=(null)
   DefMemPerNode=UNLIMITED MaxMemPerNode=UNLIMITED
   TRES=cpu=384,mem=4095888M,node=2,billing=384,gres/gpu=16
`,
			err: `partition "test" has no nodes`,
		},
		{
			name: "Case 2: valid input",
			in: `PartitionName=my_partition
   AllowGroups=ALL AllowAccounts=ALL AllowQos=ALL
   AllocNodes=ALL Default=NO QoS=N/A
   DefaultTime=NONE DisableRootJobs=NO ExclusiveUser=NO ExclusiveTopo=NO GraceTime=0 Hidden=NO
   MaxNodes=UNLIMITED MaxTime=UNLIMITED MinNodes=0 LLN=NO MaxCPUsPerNode=UNLIMITED MaxCPUsPerSocket=UNLIMITED
   NodeSets=my_partition
   Nodes=dgx[0001-0010],dgx[0021-0030]
   PriorityJobFactor=1 PriorityTier=1 RootOnly=NO ReqResv=NO OverSubscribe=NO
   OverTimeLimit=NONE PreemptMode=OFF
   Topology=topo_my_partition
   State=UP TotalCPUs=384 TotalNodes=2 SelectTypeParameters=NONE
   JobDefaults=(null)
   DefMemPerNode=UNLIMITED MaxMemPerNode=UNLIMITED
   TRES=cpu=384,mem=4095888M,node=2,billing=384,gres/gpu=16
`,
			out: []string{"dgx[0001-0010,0021-0030]"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := parsePartitionNodes("test", tc.in)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.out, out)
			}
		})
	}
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
	  "blockSizes": [2,4]
	},
	"topo2": {
	  "plugin": "topology/block",
	  "blockSizes": [8,16],
	  "nodes": ["n1", "n2", "n3"]
	},
	"topo3": {
	  "plugin": "topology/flat",
	  "clusterDefault": true
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
							BlockSizes: []int{2, 4},
						},
						"topo2": {
							Plugin:     "topology/block",
							BlockSizes: []int{8, 16},
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
		err    string
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
			name: "Case 5: with invalid partition topology",
			params: &BaseParams{
				Topologies: map[string]*Topology{
					"topo1": {
						Plugin: topology.TopologyBlock,
						Nodes:  []string{"node[001-100]"},
					},
					"topo2": {
						Plugin: topology.TopologyTree,
					},
				},
			},
			err: "missing partition name",
		},
		{
			name: "Case 6: with valid partition topology",
			params: &BaseParams{
				Topologies: map[string]*Topology{
					"default": {
						Plugin:  topology.TopologyFlat,
						Default: true,
					},
					"topo": {
						Plugin: topology.TopologyBlock,
						Nodes:  []string{"node[001-100]"},
					},
				},
			},
			cfg: &translate.Config{
				Topologies: map[string]*translate.TopologySpec{
					"default": {
						Plugin:         topology.TopologyFlat,
						ClusterDefault: true,
					},
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
			cfg, err := GetTranslateConfig(ctx, tc.params, nil)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.cfg, cfg)
			}
		})
	}
}
