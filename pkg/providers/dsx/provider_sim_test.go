/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package dsx

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/agrea/ptr"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/providersim"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestMain(m *testing.M) {
	stop := providersim.StartDefaultForTests(nil)
	code := m.Run()
	stop()
	os.Exit(code)
}

const (
	smallTreeSlurmTree = `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=n-I[21-22,25]
SwitchName=S3 Nodes=n-I[34-36]
`

	smallTreeSlurmFiltered = `SwitchName=S1 Switches=S2
SwitchName=S2 Nodes=n-I[21-22]
`

	smallTreeSlurmTrim2 = `SwitchName=S2 Nodes=n-I[21-22,25]
SwitchName=S3 Nodes=n-I[34-36]
`

	nvl72BlockSlurm = `# block001=nvl-1-1
BlockName=block001 Nodes=n-[1101-1115]
# block002=nvl-1-2
BlockName=block002 Nodes=n-[1201-1215]
# block003=nvl-2-1
BlockName=block003 Nodes=n-[2101-2115]
# block004=nvl-2-2
BlockName=block004 Nodes=n-[2201-2218]
BlockSizes=15,30,60
`
)

func TestGenerateTopologyConfig_simMissingFile_notFound(t *testing.T) {
	p, herr := LoaderSim(context.Background(), providers.Config{
		Params: map[string]any{
			"modelFileName": "does-not-exist-embedded.json",
		},
	})
	require.Nil(t, herr)

	_, err := p.GenerateTopologyConfig(context.Background(), nil, nil)
	require.NotNil(t, err)
	require.Equal(t, http.StatusNotFound, err.Code())
	require.Contains(t, err.Error(), "file not found")
}

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name       string
		params     map[string]any
		pageSize   *int
		instances  []topology.ComputeInstances
		loadErr    string
		genErr     string
		genCode    int
		topology   string // Slurm output when gen succeeds
		slurmExtra map[string]any
		checkTree  bool // when true, assert small-tree switch hierarchy (requires small-tree.json model)
	}{
		{
			name:    "Case 1: loader missing modelFileName",
			params:  map[string]any{},
			loadErr: "no model file name for simulation",
		},
		{
			name: "Case 2: loader invalid trimTiers",
			params: map[string]any{
				"modelFileName": "small-tree.json",
				"trimTiers":     6,
			},
			loadErr: "parameters error: invalid 'trimTiers' value '6': must be an integer between 0 and 2",
		},
		{
			name: "Case 3: loader invalid trimTiers type",
			params: map[string]any{
				"modelFileName": "small-tree.json",
				"trimTiers":     "x",
			},
			// Fails in [providers.GetSimulationParams] mapstructure decode (int) before [GetTrimTiers].
			loadErr: "error decoding params: could not decode configuration: 1 error(s) decoding:\n\n* error decoding 'trimTiers': invalid int \"x\"",
		},
		{
			name: "Case 4.1: ClientFactory API error",
			params: map[string]any{
				"modelFileName": "small-tree.json",
				"api_error":     simAPIErrClientFactory,
			},
			genErr:  "failed to get client: API error",
			genCode: http.StatusBadGateway,
		},
		{
			name: "Case 4.2: GetTopology API error",
			params: map[string]any{
				"modelFileName": "small-tree.json",
				"api_error":     simAPIErrGetTopology,
			},
			genErr:  "API error",
			genCode: http.StatusBadGateway,
		},
		{
			name: "Case 5: valid topology (tree, no instances)",
			params: map[string]any{
				"modelFileName": "small-tree.json",
			},
			topology:  smallTreeSlurmTree,
			checkTree: true,
		},
		{
			name: "Case 6: valid topology (tree, filtered instances)",
			params: map[string]any{
				"modelFileName": "small-tree.json",
			},
			instances: []topology.ComputeInstances{
				{
					Region: "region",
					Instances: map[string]string{
						"I21": "n-I21",
						"I22": "n-I22",
					},
				},
			},
			topology: smallTreeSlurmFiltered,
		},
		{
			name: "Case 7: valid topology (tree, trimTiers=2)",
			params: map[string]any{
				"modelFileName": "small-tree.json",
				"trimTiers":     2,
			},
			topology: smallTreeSlurmTrim2,
		},
		{
			name: "Case 8: valid topology (block, paging)",
			params: map[string]any{
				"modelFileName": "nvl72.json",
			},
			pageSize:   ptr.Int(2),
			topology:   nvl72BlockSlurm,
			slurmExtra: map[string]any{"plugin": "topology/block"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, httpErr := LoaderSim(ctx, providers.Config{Params: tc.params})
			if len(tc.loadErr) != 0 {
				require.NotNil(t, httpErr)
				require.EqualError(t, httpErr, tc.loadErr)
				return
			}
			require.Nil(t, httpErr)

			topo, genErr := provider.GenerateTopologyConfig(ctx, tc.pageSize, tc.instances)
			if len(tc.genErr) != 0 {
				require.NotNil(t, genErr)
				require.Equal(t, tc.genCode, genErr.Code())
				require.EqualError(t, genErr, tc.genErr)
				return
			}
			require.Nil(t, genErr)

			if tc.checkTree {
				tr := topo.Vertices[topology.TopologyTree]
				require.Contains(t, tr.Vertices, "S1")
				s1 := tr.Vertices["S1"]
				require.Contains(t, s1.Vertices, "S2")
				require.Contains(t, s1.Vertices, "S3")
			}

			if len(tc.topology) != 0 {
				data, slurmErr := slurm.GenerateOutput(ctx, topo, tc.slurmExtra)
				require.Nil(t, slurmErr)
				require.Equal(t, tc.topology, string(data))
			}
		})
	}
}
