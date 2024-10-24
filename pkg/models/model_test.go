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
)

func TestNewModelFromFile(t *testing.T) {
	cfg, err := NewModelFromFile("tests/models/medium-h100.yaml")
	require.NoError(t, err)

	expected := &Model{
		Switches: []Switch{
			{
				Name:     "sw3",
				Switches: []string{"sw21", "sw22"},
			},
			{
				Name:     "sw21",
				Switches: []string{"sw11", "sw12"},
			},
			{
				Name:     "sw22",
				Switches: []string{"sw13", "sw14"},
			},
			{
				Name:           "sw11",
				CapacityBlocks: []string{"cb11"},
			},
			{
				Name:           "sw12",
				CapacityBlocks: []string{"cb12"},
			},
			{
				Name:           "sw13",
				CapacityBlocks: []string{"cb13"},
			},
			{
				Name:           "sw14",
				CapacityBlocks: []string{"cb14"},
			},
		},
		CapacityBlocks: []CapacityBlock{
			{
				Name:   "cb10",
				Type:   "H100",
				NVLink: "nv1",
				Nodes:  []string{"n10-1", "n10-2"},
			},
			{
				Name:   "cb11",
				Type:   "H100",
				NVLink: "nv1",
				Nodes:  []string{"n11-1", "n11-2"},
			},
			{
				Name:  "cb12",
				Type:  "H100",
				Nodes: []string{"n12-1", "n12-2"},
			},
			{
				Name:  "cb13",
				Type:  "H100",
				Nodes: []string{"n13-1", "n13-2"},
			},
			{
				Name:  "cb14",
				Type:  "H100",
				Nodes: []string{"n14-1", "n14-2"},
			},
		},

		Nodes: map[string]*Node{
			"n10-1": {
				Name:   "n10-1",
				Type:   "H100",
				NVLink: "nv1",
			},
			"n10-2": {
				Name:   "n10-2",
				Type:   "H100",
				NVLink: "nv1",
			},
			"n11-1": {
				Name:      "n11-1",
				Type:      "H100",
				NVLink:    "nv1",
				NetLayers: []string{"sw11", "sw21", "sw3"},
			},
			"n11-2": {
				Name:      "n11-2",
				Type:      "H100",
				NVLink:    "nv1",
				NetLayers: []string{"sw11", "sw21", "sw3"},
			},
			"n12-1": {
				Name:      "n12-1",
				Type:      "H100",
				NetLayers: []string{"sw12", "sw21", "sw3"},
			},
			"n12-2": {
				Name:      "n12-2",
				Type:      "H100",
				NetLayers: []string{"sw12", "sw21", "sw3"},
			},
			"n13-1": {
				Name:      "n13-1",
				Type:      "H100",
				NetLayers: []string{"sw13", "sw22", "sw3"},
			},
			"n13-2": {
				Name:      "n13-2",
				Type:      "H100",
				NetLayers: []string{"sw13", "sw22", "sw3"},
			},
			"n14-1": {
				Name:      "n14-1",
				Type:      "H100",
				NetLayers: []string{"sw14", "sw22", "sw3"},
			},
			"n14-2": {
				Name:      "n14-2",
				Type:      "H100",
				NetLayers: []string{"sw14", "sw22", "sw3"},
			},
		},
	}

	require.Equal(t, expected, cfg)
}
