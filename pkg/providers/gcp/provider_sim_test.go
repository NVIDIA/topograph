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

package gcp

import (
	"context"
	"os"
	"testing"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

const (
	ignoreErrMsg = "_IGNORE_"

	singleCluster = `
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

	mediumCluster = `
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
		instances []topology.ComputeInstances
		apiErr    bool
		topology  string
		err       string
	}{
		{
			name:  "Case 1: bad model",
			model: `bad: model: error:`,
			err:   ignoreErrMsg,
		},
		{
			name: "Case 2: unsupported instance ID",
			model: `
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
  nodes: [n11]
`,
			err: `failed to create simulation client: invalid instance ID "n11"; must be numerical`,
		},
		{
			name:  "Case 3: no ComputeInstances",
			model: mediumCluster,
		},
		{
			name:  "Case 4: single node",
			model: singleCluster,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11", "12": "nodeCPU"},
				},
			},
			topology: `SwitchName=spine Switches=tor
SwitchName=no-topology Nodes=nodeCPU
SwitchName=tor Nodes=node11
`,
		},
		{
			name:  "Case 5: page iterator error",
			model: singleCluster,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11"},
				},
			},
			apiErr: true,
			err:    "failed to get instance topology: iterator error",
		},
		{
			name:  "Case 6: valid input",
			model: mediumCluster,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"11": "node11", "12": "node12", "21": "node21", "22": "node22"},
				},
			},
			topology: `SwitchName=spine Switches=tor[1-2]
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
				Params: map[string]any{"model_path": f.Name()},
			}
			sim, err := LoaderSim(ctx, cfg)
			if err != nil {
				if len(tc.err) == 0 {
					require.NoError(t, err)
				} else if tc.err != ignoreErrMsg {
					require.EqualError(t, err, tc.err)
				}
				return
			}
			provider := sim.(*simProvider)

			if tc.apiErr {
				cl, _ := provider.clientFactory()
				cl.(*simClient).pages[0].err = true
			}

			topo, err := provider.GenerateTopologyConfig(ctx, nil, tc.instances)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				data, err := slurm.GenerateOutput(ctx, topo, nil)
				require.NoError(t, err)
				require.Equal(t, tc.topology, string(data))
			}
		})
	}
}
