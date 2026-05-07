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

func TestNewModelFromFileMedium(t *testing.T) {
	cfg, err := NewModelFromFile("../../tests/models/medium.yaml")
	require.NoError(t, err)

	require.Len(t, cfg.Switches, 7)
	require.Len(t, cfg.CapacityBlocks, 5)
	require.Len(t, cfg.Nodes, 8)

	require.Equal(t, []string{"1101", "1102"}, cfg.Switches["sw11"].Nodes)
	require.Equal(t, map[string]CapacityBlock{
		"cb11": {
			Nodes:      []string{"1101", "1102"},
			Attributes: BasicNodeAttributes{NVLink: "nvl1"},
		},
		"cb12": {
			Nodes:      []string{"1201", "1202"},
			Attributes: BasicNodeAttributes{NVLink: "nvl2"},
		},
		"cb13": {
			Nodes:      []string{"1301", "1302"},
			Attributes: BasicNodeAttributes{NVLink: "nvl3"},
		},
		"cb14": {
			Nodes:      []string{"1401", "1402"},
			Attributes: BasicNodeAttributes{NVLink: "nvl4"},
		},
		"cb15": {},
	}, cfg.CapacityBlocks)

	require.Equal(t, &Node{
		Name:       "1101",
		Attributes: NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl1"}},
		Metadata: map[string]string{
			"region":            "us-west",
			"availability_zone": "zone1",
			"group":             "cb11",
		},
		NetLayers:     []string{"sw11", "sw21", "sw3"},
		CapacityBlock: "cb11",
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
	require.Len(t, cfg.CapacityBlocks, 5)
	require.Len(t, cfg.Nodes, 72)

	require.Equal(t, &Node{
		Name: "node2215",
		Attributes: NodeAttributes{
			BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl-2-2"},
			Status:              "Completed",
			Timestamp:           "2026/01/01 13:59:00.000",
			GPUs: []GPU{
				{
					Index:     0,
					PCIBusID:  "00000000:61:1D.5",
					UUID:      "GPU-36d8f310-d6e2-4937-aaea-47778977d89a",
					Model:     "NVIDIA GB300",
					MemoryMiB: 284208,
				},
				{
					Index:     1,
					PCIBusID:  "00000000:96:1E.7",
					UUID:      "GPU-6a485c2a-c0bd-44f8-abac-cbd3e7b9b352",
					Model:     "NVIDIA GB300",
					MemoryMiB: 284208,
				},
				{
					Index:     2,
					PCIBusID:  "00000000:69:0C.4",
					UUID:      "GPU-41be6e1a-9964-489b-be71-7653d3bdd655",
					Model:     "NVIDIA GB300",
					MemoryMiB: 284208,
				},
				{
					Index:     3,
					PCIBusID:  "00000000:7D:1D.3",
					UUID:      "GPU-3f0ac5a6-0d2a-4c5e-be59-bbbc8e8b3e28",
					Model:     "NVIDIA GB300",
					MemoryMiB: 284208,
				},
			},
		},
		Metadata: map[string]string{
			"region":            "us-east",
			"availability_zone": "zone1",
			"group":             "none",
		},
		NetLayers:     []string{"leaf-2-2", "spine-2", "core"},
		CapacityBlock: "cb-2-2",
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
			name: "Case 1: derive nodes from CapacityBlocks",
			cfg: `
switches:
  core:
    switches: [leaf]
  leaf:
    nodes: ["n[1-2]", n3]
capacity_blocks:
  cb1:
    nodes: ["n[1-2]"]
    attributes:
      nvlink: nvl1
  cb2:
    nodes: [n3]
    attributes:
      nvlink: nvl2
`,
			model: &Model{
				Switches: map[string]*Switch{
					"core": {
						Name:     "core",
						Switches: []string{"leaf"},
					},
					"leaf": {Name: "leaf", Nodes: []string{"n1", "n2", "n3"}},
				},
				Nodes: map[string]*Node{
					"n1": {
						Name:          "n1",
						Attributes:    NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl1"}},
						CapacityBlock: "cb1",
						NetLayers:     []string{"leaf", "core"},
						Metadata:      map[string]string{},
					},
					"n2": {
						Name:          "n2",
						Attributes:    NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl1"}},
						CapacityBlock: "cb1",
						NetLayers:     []string{"leaf", "core"},
						Metadata:      map[string]string{},
					},
					"n3": {
						Name:          "n3",
						Attributes:    NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl2"}},
						CapacityBlock: "cb2",
						NetLayers:     []string{"leaf", "core"},
						Metadata:      map[string]string{},
					},
				},
				CapacityBlocks: map[string]CapacityBlock{
					"cb1": {
						Nodes:      []string{"n1", "n2"},
						Attributes: BasicNodeAttributes{NVLink: "nvl1"},
					},
					"cb2": {
						Nodes:      []string{"n3"},
						Attributes: BasicNodeAttributes{NVLink: "nvl2"},
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
			name: "Case 2: declared CapacityBlocks with top-level nodes",
			cfg: `
nodes:
  n1:
    capacity_block_id: cb1
capacity_blocks:
  cb1: {}
`,
			model: &Model{
				Nodes: map[string]*Node{
					"n1": {
						Name:          "n1",
						CapacityBlock: "cb1",
					},
				},
				CapacityBlocks: map[string]CapacityBlock{"cb1": {Nodes: []string{"n1"}}},
				Instances: []topology.ComputeInstances{
					{
						Region:    "none",
						Instances: map[string]string{"n1": "n-n1"},
					},
				},
			},
		},
		{
			name: "Case 3: derive CapacityBlocks from nodes",
			cfg: `
nodes:
  n1:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n2:
    attributes:
      nvlink: nvl2
`,
			model: &Model{
				Nodes: map[string]*Node{
					"n1": {
						Name:          "n1",
						Attributes:    NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl1"}},
						CapacityBlock: "cb1",
					},
					"n2": {
						Name:       "n2",
						Attributes: NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl2"}},
					},
				},
				CapacityBlocks: map[string]CapacityBlock{
					"cb1": {
						Nodes:      []string{"n1"},
						Attributes: BasicNodeAttributes{NVLink: "nvl1"},
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
			name: "Case 4: CapacityBlocks assigns missing node capacity block",
			cfg: `
nodes:
  n1: {}
capacity_blocks:
  cb1:
    nodes: [n1]
    attributes:
      nvlink: nvl1
`,
			model: &Model{
				Nodes: map[string]*Node{
					"n1": {
						Name:          "n1",
						Attributes:    NodeAttributes{BasicNodeAttributes: BasicNodeAttributes{NVLink: "nvl1"}},
						CapacityBlock: "cb1",
					},
				},
				CapacityBlocks: map[string]CapacityBlock{
					"cb1": {
						Nodes:      []string{"n1"},
						Attributes: BasicNodeAttributes{NVLink: "nvl1"},
					},
				},
				Instances: []topology.ComputeInstances{
					{
						Region:    "none",
						Instances: map[string]string{"n1": "n-n1"},
					},
				},
			},
		},
		{
			name: "Case 5: orphan CapacityBlock is allowed",
			cfg: `
nodes:
  n1:
    capacity_block_id: cb1
capacity_blocks:
  cb1: {}
  cb2: {}
`,
			model: &Model{
				Nodes: map[string]*Node{
					"n1": {
						Name:          "n1",
						CapacityBlock: "cb1",
					},
				},
				CapacityBlocks: map[string]CapacityBlock{
					"cb1": {Nodes: []string{"n1"}},
					"cb2": {},
				},
				Instances: []topology.ComputeInstances{
					{
						Region:    "none",
						Instances: map[string]string{"n1": "n-n1"},
					},
				},
			},
		},
		{
			name: "Case 6: conflicting capacity block assignment",
			cfg: `
nodes:
  n1:
    capacity_block_id: cb1
capacity_blocks:
  cb2:
    nodes: [n1]
`,
			err: `node "n1" belongs to capacity blocks "cb1" and "cb2"`,
		},
		{
			name: "Case 7: parsing error",
			cfg: `
capacity_blocks:
- cb1
`,
			err: "cannot unmarshal !!seq into map[string]models.CapacityBlock",
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
