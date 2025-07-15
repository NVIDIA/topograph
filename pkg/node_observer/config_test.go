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

package node_observer

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewConfigFromFile(t *testing.T) {
	testCases := []struct {
		name   string
		noFile bool
		data   string
		cfg    *Config
		err    string
	}{
		{
			name:   "Case 1: config does not exist",
			noFile: true,
			err:    "open /does/not/exist: no such file or directory",
		},
		{
			name: "Case 2: parse error",
			data: "12345",
			err:  "yaml: unmarshal errors:\n  line 1: cannot unmarshal !!int `12345` into node_observer.Config",
		},
		{
			name: "Case 3: empty config",
			err:  "must specify topology_generator_url",
		},
		{
			name: "Case 4: missing trigger",
			data: `
topology_generator_url: "http://topograph.default.svc.cluster.local:49021/v1/generate"
params:
  topology_config_path: topology.conf
  topology_configmap_name: topology-config
  topology_configmap_namespace: default
`,
			err: "must specify node_labels and/or pod_labels in trigger",
		},
		{
			name: "Case 5: valid",
			data: `
topology_generator_url: "http://topograph.default.svc.cluster.local:49021/v1/generate"
params:
  topology_config_path: topology.conf
  topology_configmap_name: topology-config
  topology_configmap_namespace: default
trigger:
  node_labels:
    a: b
    c: d
`,
			cfg: &Config{
				TopologyGeneratorURL: "http://topograph.default.svc.cluster.local:49021/v1/generate",
				Params: map[string]any{
					"topology_config_path":         "topology.conf",
					"topology_configmap_name":      "topology-config",
					"topology_configmap_namespace": "default",
				},
				Trigger: Trigger{
					NodeLabels: map[string]string{"a": "b", "c": "d"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fname string
			if tc.noFile {
				fname = "/does/not/exist"
			} else {
				f, err := os.CreateTemp("", "test-*")
				require.NoError(t, err)
				fname = f.Name()
				defer func() { _ = os.Remove(fname) }()
				defer func() { _ = f.Close() }()

				n, err := f.WriteString(tc.data)
				require.NoError(t, err)
				require.Equal(t, len(tc.data), n)
				err = f.Sync()
				require.NoError(t, err)
			}
			cfg, err := NewConfigFromFile(fname)
			if len(tc.err) != 0 {
				require.Error(t, err, "expected error starts with %q", tc.err)
				require.True(t, strings.HasSuffix(err.Error(), tc.err), "unexpected error %q", err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.cfg, cfg)
			}
		})
	}
}
