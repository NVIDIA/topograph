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

package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/NVIDIA/topograph/pkg/protos"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestConvert(t *testing.T) {
	testCases := []struct {
		name string
		in   *pb.Instance
		out  *topology.InstanceTopology
	}{
		{
			name: "Case 1: all params",
			in: &pb.Instance{
				Id:            "1",
				NetworkLayers: []string{"block1", "spine1", "dc1"},
				NvlinkDomain:  "nvl1",
			},
			out: &topology.InstanceTopology{
				InstanceID:    "1",
				BlockID:       "block1",
				SpineID:       "spine1",
				DatacenterID:  "dc1",
				AcceleratorID: "nvl1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, convert(tc.in))
		})
	}
}
