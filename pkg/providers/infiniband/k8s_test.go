/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClusterID(t *testing.T) {
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clusterID, err := parseClusterID(tc.input)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.clusterID, clusterID)
			}
		})
	}
}
