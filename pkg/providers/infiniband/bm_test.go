/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestPopulateDomainsFromPdshOutput(t *testing.T) {
	nvOutput := `node-10:         CliqueId                          : 4000000004
	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-07:         CliqueId                          : 4000000005
	node-07:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-11:         CliqueId                          : 50000000-0000-0000-0000-000000000003
	node-11:         CliqueId                          : N/A
	node-11:         ClusterUUID                       : 4000000003
	node-11:         ClusterUUID                       : N/A
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

func TestSetID(t *testing.T) {
	clusters := map[string]*Cluster{
		"node1": {node: "node1"},
		"node2": {node: "node2"},
		"node3": {node: "node3"},
	}
	invalid := make(map[string]bool)

	input := []struct {
		nodename string
		idname   string
		val      string
	}{
		{nodename: "node1", idname: "ID", val: "ID1"},
		{nodename: "node1", idname: "UUID", val: "UUID1"},
		{nodename: "node2", idname: "ID", val: "ID2"},
		{nodename: "node2", idname: "UUID", val: "UUID2"},
		{nodename: "node2", idname: "ID", val: "N/A"},
		{nodename: "node2", idname: "UUID", val: "N/A"},
		{nodename: "node3", idname: "ID", val: "ID3"},
		{nodename: "node3", idname: "UUID", val: "UUID3"},
	}

	for _, i := range input {
		cluster := clusters[i.nodename]
		switch i.idname {
		case "ID":
			setID(i.nodename, i.idname, &cluster.cliqueID, i.val, invalid)
		case "UUID":
			setID(i.nodename, i.idname, &cluster.UUID, i.val, invalid)
		}
	}

	resClusters := map[string]*Cluster{
		"node1": {node: "node1", UUID: "UUID1", cliqueID: "ID1"},
		"node2": {node: "node2", UUID: "UUID2", cliqueID: "ID2"},
		"node3": {node: "node3", UUID: "UUID3", cliqueID: "ID3"},
	}
	resInvalid := map[string]bool{"node2": true}

	require.Equal(t, resClusters, clusters)
	require.Equal(t, resInvalid, invalid)
}
