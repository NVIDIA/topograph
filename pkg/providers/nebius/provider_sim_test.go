/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

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

	nodeModel = `
switches:
- name: core
  switches: [spine]
- name: spine
  switches: [tor]
- name: tor
  capacity_blocks: [cb]
capacity_blocks:
- name: cb
  type: GB200
  nvlink: nvl1
  nodes: [11]
`

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
  nodes: [11,12]
- name: cb2
  type: GB200
  nvlink: nvl2
  nodes: [21,22]
`
)

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name      string
		model     string
		pageSize  *int
		instances []topology.ComputeInstances
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
			name:  "Case 2: ClientFactory API error",
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
			name:  "Case 3: GetComputeInstance error",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11"},
				},
			},
			apiErr: errInstances,
			err:    "failed to get instance list: API error",
		},
		{
			name:  "Case 4: topology path error",
			model: nodeModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11"},
				},
			},
			apiErr: errTopologyPath,
			topology: `SwitchName=no-topology Nodes=node11
`,
		},
		{
			name:  "Case 5: missing network interface",
			model: nodeModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11"},
				},
			},
			apiErr: errNetworkIntf,
			topology: `SwitchName=no-topology Nodes=node11
`,
		},
		{
			name:   "Case 6: valid cluster in tree format without pagination",
			model:  clusterModel,
			params: map[string]any{"plugin": "topology/tree"},
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11", "12": "node12", "21": "node21", "22": "node22", "31": "node31"},
				},
			},
			topology: `SwitchName=core Switches=spine
SwitchName=no-topology Nodes=node31
SwitchName=spine Switches=tor[1-2]
SwitchName=tor1 Nodes=node[11-12]
SwitchName=tor2 Nodes=node[21-22]
`,
		},
		{
			name:     "Case 7: valid cluster in tree format with pagination",
			model:    clusterModel,
			pageSize: ptr.Int(2),
			params:   map[string]any{"plugin": "topology/tree"},
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11", "12": "node12", "21": "node21", "22": "node22", "31": "node31"},
				},
			},
			topology: `SwitchName=core Switches=spine
SwitchName=no-topology Nodes=node31
SwitchName=spine Switches=tor[1-2]
SwitchName=tor1 Nodes=node[11-12]
SwitchName=tor2 Nodes=node[21-22]
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
			provider, httpErr := LoaderSim(ctx, cfg)
			if httpErr != nil {
				if len(tc.err) == 0 {
					require.Nil(t, httpErr)
				} else if tc.err != ignoreErrMsg {
					require.EqualError(t, httpErr, tc.err)
				}
				return
			}

			topo, httpErr := provider.GenerateTopologyConfig(ctx, tc.pageSize, tc.instances)
			if len(tc.err) != 0 {
				require.EqualError(t, httpErr, tc.err)
			} else {
				require.Nil(t, httpErr)
				data, httpErr := slurm.GenerateOutput(ctx, topo, tc.params)
				require.Nil(t, httpErr)
				require.Equal(t, tc.topology, string(data))
			}
		})
	}
}
