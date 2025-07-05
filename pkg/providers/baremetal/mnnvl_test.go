/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package baremetal

import (
	"bytes"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestPopulateDomainsFromPdshOutput(t *testing.T) {
	nvOutput := `node-10:         CliqueId                          : 4000000004
	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-07:         CliqueId                          : 4000000005
	node-07:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-08:         CliqueId                          : 4000000005
	node-08:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-09:         CliqueId                          : 4000000005
	node-09:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
`
	domainMap := topology.DomainMap{
		"50000000-0000-0000-0000-000000000004.4000000005": map[string]string{"node-07": "node-07", "node-08": "node-08"},
		"50000000-0000-0000-0000-000000000005.4000000004": map[string]string{"node-10": "node-10"},
		"50000000-0000-0000-0000-000000000005.4000000005": map[string]string{"node-09": "node-09"},
	}

	testCases := []struct {
		name     string
		nvOutput string
		domains  topology.DomainMap
		err      string
	}{
		{
			name:     "Case 1: missing CliqueId",
			nvOutput: `	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005`,
			err:      `missing CliqueId for node "node-10"`,
		},
		{
			name: "Case 2: missing ClusterUUID",
			nvOutput: `node-10:         CliqueId                          : 4000000004
	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-07:         CliqueId                          : 4000000005
`,
			err: `missing ClusterUUID for node "node-07"`,
		},
		{
			name:     "Case 3: valid input",
			nvOutput: nvOutput,
			domains:  domainMap,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			domains, err := populateDomainsFromPdshOutput(bytes.NewBufferString(tc.nvOutput))
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.domains, domains)
			}
		})
	}
}
