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

package k8s

import (
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestGetParameters(t *testing.T) {
	testCases := []struct {
		name   string
		params map[string]any
		ret    *Params
		err    string
	}{
		{
			name: "Case 1: empty map",
			ret:  &Params{Method: topology.MethodLabels},
		},
		{
			name: "Case 2: missing key",
			params: map[string]any{
				topology.KeyMethod:                 topology.MethodSlurm,
				topology.KeyTopoConfigmapName:      "name",
				topology.KeyTopoConfigmapNamespace: "namespace",
			},
			err: `must specify engine parameter "topology_config_path" with slurm method`,
		},
		{
			name: "Case 3: unsupported method key",
			params: map[string]any{
				topology.KeyMethod: "BAD",
			},
			err: `unsupported method "BAD"`,
		},
		{
			name: "Case 4: valid",
			params: map[string]any{
				topology.KeyMethod:                 topology.MethodSlurm,
				topology.KeyTopoConfigPath:         "path",
				topology.KeyTopoConfigmapName:      "name",
				topology.KeyTopoConfigmapNamespace: "namespace",
			},
			ret: &Params{
				Method:             topology.MethodSlurm,
				ConfigPath:         "path",
				ConfigMapName:      "name",
				ConfigMapNamespace: "namespace",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := getParameters(tc.params)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.ret, p)
			}
		})
	}
}
