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

	require.Len(t, cfg.Switches, 7)
	require.Len(t, cfg.CapacityBlocks, 4)
	require.Len(t, cfg.Nodes, 8)

	require.Equal(t, []string{"1101", "1102"}, cfg.Switches["sw11"].Nodes)
	require.Equal(t, []string{"cb11", "cb12", "cb13", "cb14"}, cfg.CapacityBlocks)

	require.Equal(t, &Node{
		Name: "1101",
		Attributes: NodeAttributes{
			NVLink: "nvl1",
		},
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
