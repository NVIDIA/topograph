/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDevicePluginInfo(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		ns        string
		ds        string
	}{
		{
			name: "Case 1: no overrides uses defaults",
			ns:   defaultGpuOperatorNamespace,
			ds:   defaultDevicePluginDaemonSet,
		},
		{
			name: "Case 2: override namespace only",
			overrides: map[string]string{
				gpuOperatorNamespaceArg: "custom-ns",
			},
			ns: "custom-ns",
			ds: defaultDevicePluginDaemonSet,
		},
		{
			name: "Case 3: override daemonset only",
			overrides: map[string]string{
				devicePluginDaemonSetArg: "custom-ds",
			},
			ns: defaultGpuOperatorNamespace,
			ds: "custom-ds",
		},
		{
			name: "Case 4: override both",
			overrides: map[string]string{
				gpuOperatorNamespaceArg:  "custom-ns",
				devicePluginDaemonSetArg: "custom-ds",
			},
			ns: "custom-ns",
			ds: "custom-ds",
		},
		{
			name: "Case 5: irrelevant keys ignored",
			overrides: map[string]string{
				"other": "value",
			},
			ns: defaultGpuOperatorNamespace,
			ds: defaultDevicePluginDaemonSet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds, ns := getDevicePluginInfo(tt.overrides)
			require.Equal(t, tt.ds, ds)
			require.Equal(t, tt.ns, ns)
		})
	}
}

func TestParseClusterID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		clusterID string
		err       string
	}{
		{
			name: "Case 1: missing ClusterUUID",
			err:  "missing ClusterUUID",
		},
		{
			name:  "Case 2: missing CliqueId",
			input: "  ClusterUUID     : 0000-0000-0000-0000-000000000000",
			err:   "missing CliqueId",
		},
		{
			name: "Case 3: valid input",
			input: `
        CliqueId                          : 0
        ClusterUUID                       : 00000000-0000-0000-0000-000000000000
`,
			clusterID: "00000000-0000-0000-0000-000000000000.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterID, err := parseClusterID(tt.input)
			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.clusterID, clusterID)
			}
		})
	}
}
