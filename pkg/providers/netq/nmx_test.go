/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"os"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestGetComputeUrl(t *testing.T) {
	p := &Provider{
		params: &ProviderParams{},
		cred:   &Credentials{user: "user", passwd: "passwd"},
	}

	testCases := []struct {
		name       string
		serverURL  string
		headers    map[string]string
		computeUrl string
		err        string
	}{
		{
			name:      "Case 1: invalid URL",
			serverURL: `:///server`,
			err:       `parse ":///server": missing protocol scheme`,
		},
		{
			name:       "Case 2: valid input",
			serverURL:  `https://server.com`,
			headers:    map[string]string{"Authorization": "Basic dXNlcjpwYXNzd2Q="},
			computeUrl: `https://server.com/nmx/v1/compute-nodes`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p.params.ApiURL = tc.serverURL
			url, headers, err := p.getComputeUrl()
			if len(tc.err) != 0 {
				require.NotNil(t, err)
				require.EqualError(t, err, tc.err)
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.computeUrl, url)
				require.Equal(t, tc.headers, headers)
			}
		})
	}
}

func TestParseComputeNodes(t *testing.T) {
	data, err := os.ReadFile("../../../tests/output/netq/computeNodes.json")
	require.NoError(t, err)

	domains, err := parseComputeNodes(data)
	require.Nil(t, err)

	expected := topology.NewDomainMap()
	expected.AddHost("3fbfc98b-7f95-4749-ab11-bd351a2aab3e", "node1", "node1")
	expected.AddHost("3fbfc98b-7f95-4749-ab11-bd351a2aab3e", "node2", "node2")

	require.Equal(t, expected, domains)
}
