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

package models

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func acceleratorLabels(accelerator string) map[string]string {
	if accelerator == "" {
		return nil
	}
	return map[string]string{LabelAccelerator: accelerator}
}

func TestNewModelFromFileMedium(t *testing.T) {
	cfg, err := NewModelFromFile("../../tests/models/medium.yaml")
	require.NoError(t, err)

	require.Len(t, cfg.Switches, 7)
	require.Len(t, cfg.CapacityBlocks, 4)
	require.Len(t, cfg.Nodes, 8)

	require.Equal(t, []string{"1101", "1102"}, cfg.Switches["sw11"].Nodes)
	require.Equal(t, []CapacityBlock{
		{
			Switch: "sw11",
			Nodes:  []string{"1101", "1102"},
			Labels: acceleratorLabels("nvl1"),
		},
		{
			Switch: "sw12",
			Nodes:  []string{"1201", "1202"},
			Labels: acceleratorLabels("nvl2"),
		},
		{
			Switch: "sw13",
			Nodes:  []string{"1301", "1302"},
			Labels: acceleratorLabels("nvl3"),
		},
		{
			Switch: "sw14",
			Nodes:  []string{"1401", "1402"},
			Labels: acceleratorLabels("nvl4"),
		},
	}, cfg.CapacityBlocks)

	require.Equal(t, &topology.Instance{
		ID: "1101",
		Labels: map[string]string{
			LabelTopologyRegion: "us-west",
			LabelTopologyZone:   "zone1",
			LabelAccelerator:    "nvl1",
		},
		NetLayers: []string{"sw11", "sw21", "sw3"},
	}, cfg.Nodes["1101"])

	require.Equal(t, []topology.ComputeInstances{
		{
			Region: "us-west",
			Instances: map[string]string{
				"1101": "n-1101",
				"1102": "n-1102",
				"1201": "n-1201",
				"1202": "n-1202",
				"1301": "n-1301",
				"1302": "n-1302",
				"1401": "n-1401",
				"1402": "n-1402",
			},
		},
	}, cfg.Instances)
}

func TestNewModelFromFileNVL72(t *testing.T) {
	cfg, err := NewModelFromFile("../../tests/models/nvl72.yaml")
	require.NoError(t, err)

	require.Len(t, cfg.Switches, 7)
	require.Len(t, cfg.CapacityBlocks, 4)
	require.Len(t, cfg.Nodes, 72)

	require.Equal(t, &topology.Instance{
		ID: "node2215",
		Labels: map[string]string{
			LabelTopologyRegion: "us-east",
			LabelTopologyZone:   "zone1",
			LabelAccelerator:    "nvl-2-2",
		},
		NetLayers: []string{"leaf-2-2", "spine-2", "core"},
	}, cfg.Nodes["node2215"])
}

func TestModelCompletion(t *testing.T) {
	tests := []struct {
		name  string
		cfg   string
		model *Model
		err   string
	}{
		{
			name: "Case 1: derive nodes from blocks",
			cfg: `
switches:
  core:
    switches: [leaf]
  leaf: {}
blocks:
- switch: leaf
  nodes: ["n[1-2]"]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
- switch: leaf
  nodes: [n3]
  labels:
    network.topology.nvidia.com/accelerator: nvl2
`,
			model: &Model{
				Switches: map[string]*Switch{
					"core": {
						Name:     "core",
						Switches: []string{"leaf"},
					},
					"leaf": {Name: "leaf", Nodes: []string{"n1", "n2", "n3"}},
				},
				Nodes: map[string]*topology.Instance{
					"n1": {
						ID:        "n1",
						NetLayers: []string{"leaf", "core"},
						Labels:    acceleratorLabels("nvl1"),
					},
					"n2": {
						ID:        "n2",
						NetLayers: []string{"leaf", "core"},
						Labels:    acceleratorLabels("nvl1"),
					},
					"n3": {
						ID:        "n3",
						NetLayers: []string{"leaf", "core"},
						Labels:    acceleratorLabels("nvl2"),
					},
				},
				CapacityBlocks: []CapacityBlock{
					{
						Switch: "leaf",
						Nodes:  []string{"n1", "n2"},
						Labels: acceleratorLabels("nvl1"),
					},
					{
						Switch: "leaf",
						Nodes:  []string{"n3"},
						Labels: acceleratorLabels("nvl2"),
					},
				},
				Instances: []topology.ComputeInstances{
					{
						Region:    "none",
						Instances: map[string]string{"n1": "n-n1", "n2": "n-n2", "n3": "n-n3"},
					},
				},
			},
		},
		{
			name: "Case 2: switches are optional",
			cfg: `
blocks:
- nodes: ["n[1-2]"]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
`,
			model: &Model{
				Nodes: map[string]*topology.Instance{
					"n1": {
						ID:     "n1",
						Labels: acceleratorLabels("nvl1"),
					},
					"n2": {
						ID:     "n2",
						Labels: acceleratorLabels("nvl1"),
					},
				},
				CapacityBlocks: []CapacityBlock{
					{
						Nodes:  []string{"n1", "n2"},
						Labels: acceleratorLabels("nvl1"),
					},
				},
				Instances: []topology.ComputeInstances{
					{
						Region:    "none",
						Instances: map[string]string{"n1": "n-n1", "n2": "n-n2"},
					},
				},
			},
		},
		{
			name: "Case 3: block must declare nodes",
			cfg: `
blocks:
- nodes: [n1]
  labels:
    network.topology.nvidia.com/accelerator: nvl1
- labels: {}
`,
			err: `capacity block at index 1 must declare at least one node`,
		},
		{
			name: "Case 4: conflicting block assignment",
			cfg: `
blocks:
- nodes: [n1]
- nodes: [n1]
`,
			err: `node "n1" belongs to more than one capacity block`,
		},
		{
			name: "Case 5: block switch must exist",
			cfg: `
blocks:
- switch: missing
  nodes: [n1]
`,
			err: `capacity block at index 0 references unknown switch "missing"`,
		},
		{
			name: "Case 6: parsing error",
			cfg: `
blocks:
- cb1
`,
			err: "cannot unmarshal !!str `cb1`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, err := NewModelFromData([]byte(tt.cfg), "inline")
			if len(tt.err) != 0 {
				require.NotNil(t, err)
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.model, model)
			}
		})
	}
}

func TestSwitchAndBlockLabelsMerge(t *testing.T) {
	cfg := `
switches:
  core:
    labels:
      topology.kubernetes.io/region: region1
      inherited: core
    switches: [leaf]
  leaf:
    labels:
      topology.kubernetes.io/zone: zone1
      inherited: leaf
blocks:
- switch: leaf
  nodes: [n1]
  labels:
    block-label: block-value
    inherited: block
`

	model, err := NewModelFromData([]byte(cfg), "inline")
	require.NoError(t, err)

	require.Equal(t, map[string]string{
		LabelTopologyRegion: "region1",
		LabelTopologyZone:   "zone1",
		"inherited":         "block",
		"block-label":       "block-value",
	}, model.Nodes["n1"].Labels)
	require.Equal(t, []topology.ComputeInstances{
		{
			Region:    "region1",
			Instances: map[string]string{"n1": "n-n1"},
		},
	}, model.Instances)
}
