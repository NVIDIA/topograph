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

func TestNewModelFromFile(t *testing.T) {
	cfg, err := NewModelFromFile("../../tests/models/medium.yaml")
	require.NoError(t, err)

	expected := &Model{
		Switches: []*Switch{
			{
				Name:     "sw3",
				Metadata: map[string]string{"region": "us-west"},
				Switches: []string{"sw21", "sw22"},
			},
			{
				Name:     "sw21",
				Metadata: map[string]string{"availability_zone": "zone1"},
				Switches: []string{"sw11", "sw12"},
			},
			{
				Name:     "sw22",
				Metadata: map[string]string{"availability_zone": "zone2"},
				Switches: []string{"sw13", "sw14"},
			},
			{
				Name:           "sw11",
				Metadata:       map[string]string{"group": "cb11"},
				CapacityBlocks: []string{"cb11"},
			},
			{
				Name:           "sw12",
				Metadata:       map[string]string{"group": "cb12"},
				CapacityBlocks: []string{"cb12"},
			},
			{
				Name:           "sw13",
				Metadata:       map[string]string{"group": "cb13"},
				CapacityBlocks: []string{"cb13"},
			},
			{
				Name:           "sw14",
				Metadata:       map[string]string{"group": "cb14"},
				CapacityBlocks: []string{"cb14"},
			},
		},
		CapacityBlocks: []*CapacityBlock{
			{
				Name:   "cb11",
				Type:   "GB200",
				NVLink: "nvl1",
				Nodes:  []string{"n11-1", "n11-2"},
			},
			{
				Name:   "cb12",
				Type:   "GB200",
				NVLink: "nvl2",
				Nodes:  []string{"n12-1", "n12-2"},
			},
			{
				Name:   "cb13",
				Type:   "GB200",
				NVLink: "nvl3",
				Nodes:  []string{"n13-1", "n13-2"},
			},
			{
				Name:   "cb14",
				Type:   "GB200",
				NVLink: "nvl4",
				Nodes:  []string{"n14-1", "n14-2"},
			},
		},
		Nodes: map[string]*Node{
			"n11-1": {
				Name: "n11-1",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone1",
					"group":             "cb11",
				},
				Type:          "GB200",
				NVLink:        "nvl1",
				NetLayers:     []string{"sw11", "sw21", "sw3"},
				CapacityBlock: "cb11",
			},
			"n11-2": {
				Name: "n11-2",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone1",
					"group":             "cb11",
				},
				Type:          "GB200",
				NVLink:        "nvl1",
				NetLayers:     []string{"sw11", "sw21", "sw3"},
				CapacityBlock: "cb11",
			},
			"n12-1": {
				Name: "n12-1",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone1",
					"group":             "cb12",
				},
				Type:          "GB200",
				NVLink:        "nvl2",
				NetLayers:     []string{"sw12", "sw21", "sw3"},
				CapacityBlock: "cb12",
			},
			"n12-2": {
				Name: "n12-2",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone1",
					"group":             "cb12",
				},
				Type:          "GB200",
				NVLink:        "nvl2",
				NetLayers:     []string{"sw12", "sw21", "sw3"},
				CapacityBlock: "cb12",
			},
			"n13-1": {
				Name: "n13-1",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone2",
					"group":             "cb13",
				},
				Type:          "GB200",
				NVLink:        "nvl3",
				NetLayers:     []string{"sw13", "sw22", "sw3"},
				CapacityBlock: "cb13",
			},
			"n13-2": {
				Name: "n13-2",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone2",
					"group":             "cb13",
				},
				Type:          "GB200",
				NVLink:        "nvl3",
				NetLayers:     []string{"sw13", "sw22", "sw3"},
				CapacityBlock: "cb13",
			},
			"n14-1": {
				Name: "n14-1",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone2",
					"group":             "cb14",
				},
				Type:          "GB200",
				NVLink:        "nvl4",
				NetLayers:     []string{"sw14", "sw22", "sw3"},
				CapacityBlock: "cb14",
			},
			"n14-2": {
				Name: "n14-2",
				Metadata: map[string]string{
					"region":            "us-west",
					"availability_zone": "zone2",
					"group":             "cb14",
				},
				Type:          "GB200",
				NVLink:        "nvl4",
				NetLayers:     []string{"sw14", "sw22", "sw3"},
				CapacityBlock: "cb14",
			},
		},
		Instances: []topology.ComputeInstances{
			{
				Region: "us-west",
				Instances: map[string]string{
					"n11-1": "n11-1",
					"n11-2": "n11-2",
					"n12-1": "n12-1",
					"n12-2": "n12-2",
					"n13-1": "n13-1",
					"n13-2": "n13-2",
					"n14-1": "n14-1",
					"n14-2": "n14-2",
				},
			},
		},
	}

	require.Equal(t, expected, cfg)
}
