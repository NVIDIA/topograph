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
	"testing"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestConvert(t *testing.T) {
	valid := &topology.InstanceTopology{
		InstanceID:   "id",
		BlockID:      "block",
		SpineID:      "net",
		DatacenterID: "datacenter",
	}

	testCases := []struct {
		name string
		host *core.ComputeHostSummary
		topo *topology.InstanceTopology
		err  string
	}{
		{
			name: "Case 1: missing InstanceId",
			host: &core.ComputeHostSummary{},
			err:  "missing InstanceId in ComputeHostSummary",
		},
		{
			name: "Case 2: missing LocalBlock",
			host: &core.ComputeHostSummary{
				InstanceId: &valid.InstanceID,
			},
			err: `missing LocalBlockId for instance "id"`,
		},
		{
			name: "Case 3: missing NetworkBlockId",
			host: &core.ComputeHostSummary{
				InstanceId:   &valid.InstanceID,
				LocalBlockId: &valid.BlockID,
			},
			err: `missing NetworkBlockId for instance "id"`,
		},
		{
			name: "Case 4: missing HpcIslandId",
			host: &core.ComputeHostSummary{
				InstanceId:     &valid.InstanceID,
				LocalBlockId:   &valid.BlockID,
				NetworkBlockId: &valid.SpineID,
			},
			err: `missing HpcIslandId for instance "id"`,
		},
		{
			name: "Case 5: valid input",
			host: &core.ComputeHostSummary{
				InstanceId:     &valid.InstanceID,
				LocalBlockId:   &valid.BlockID,
				NetworkBlockId: &valid.SpineID,
				HpcIslandId:    &valid.DatacenterID,
			},
			topo: valid,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			topo, err := convert(tc.host)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.topo, topo)
			}
		})
	}
}
