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

package oci

import (
	"context"
	"os"
	"testing"

	"github.com/agrea/ptr"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	ignoreErrMsg = "_IGNORE_"

	clusterModel = `
switches:
- name: core
  switches: [spine]
- name: spine
  switches: [tor1,tor2]
- name: tor1
  capacity_blocks: [cb1]
- name: tor2
  capacity_blocks: [cb2]
capacity_blocks:
- name: cb1
  type: GB200
  nvlink: nvl1
  nodes: [n11,n12]
- name: cb2
  type: GB200
  nvlink: nvl2
  nodes: [n21,n22]
`
)

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name      string
		model     string
		region    string
		instances []topology.ComputeInstances
		pageSize  *int
		params    map[string]any
		apiErr    int
		topology  string
		err       string
	}{
		{
			name:  "Case 1: bad model",
			model: `bad: model: error:`,
			err:   ignoreErrMsg,
		},
		{
			name:  "Case 2: no ComputeInstances",
			model: clusterModel,
		},
		{
			name:  "Case 3: missing region",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Instances: map[string]string{"n11": "node11"},
				},
			},
			err: `must specify region`,
		},
		{
			name:  "Case 4.1: ListAvailabilityDomains API error",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11"},
				},
			},
			apiErr: errListAvailabilityDomains,
			err:    `failed to get availability domains: API error`,
		},
		{
			name:  "Case 4.2: ListComputeHosts API error",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11"},
				},
			},
			apiErr: errListComputeHosts,
			err:    `failed to get hosts info: API error`,
		},
		{
			name:  "Case 4.3: ClientFactory API error",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11"},
				},
			},
			apiErr: errClientFactory,
			err:    `failed to create API client: API error`,
		},
		{
			name:   "Case 5: valid cluster in tree format without paging",
			model:  clusterModel,
			region: "region",
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11", "n12": "node12", "n21": "node21", "n22": "node22", "n31": "node31"},
				},
			},
			topology: `# switch.3.1=core
SwitchName=switch.3.1 Switches=switch.2.1
SwitchName=no-topology Nodes=node31
# switch.2.1=spine
SwitchName=switch.2.1 Switches=switch.1.[1-2]
# switch.1.1=tor1
SwitchName=switch.1.1 Nodes=node[11-12]
# switch.1.2=tor2
SwitchName=switch.1.2 Nodes=node[21-22]
`,
		},
		{
			name:   "Case 6: valid cluster in block format with paging",
			model:  clusterModel,
			region: "region",
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11", "n12": "node12", "n21": "node21", "n22": "node22", "n31": "node31"},
				},
			},
			pageSize: ptr.Int(2),
			params:   map[string]any{"plugin": "topology/block"},
			topology: `# block002=nvl2
BlockName=block002 Nodes=node[21-22]
# block001=nvl1
BlockName=block001 Nodes=node[11-12]
BlockSizes=2,4
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "test-*")
			require.NoError(t, err)
			defer func() { _ = os.Remove(f.Name()) }()
			defer func() { _ = f.Close() }()
			n, err := f.WriteString(tc.model)
			require.NoError(t, err)
			require.Equal(t, len(tc.model), n)
			err = f.Sync()
			require.NoError(t, err)

			cfg := providers.Config{
				Params: map[string]any{
					"model_path": f.Name(),
					"api_error":  tc.apiErr,
				},
			}
			provider, err := LoaderSim(ctx, cfg)
			if err != nil {
				if len(tc.err) == 0 {
					require.NoError(t, err)
				} else if tc.err != ignoreErrMsg {
					require.EqualError(t, err, tc.err)
				}
				return
			}

			topo, err := provider.GenerateTopologyConfig(ctx, tc.pageSize, tc.instances)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				data, err := slurm.GenerateOutput(ctx, topo, tc.params)
				require.NoError(t, err)
				require.Equal(t, tc.topology, string(data))
			}
		})
	}
}
