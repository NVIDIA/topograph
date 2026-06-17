/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package lambdai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestLoader(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		config providers.Config
		err    string
	}{
		{
			name: "Case 1: success",
			config: providers.Config{
				Creds: map[string]any{
					"workspaceId": "workspace-123",
					"token":       "token-abc",
				},
				Params: map[string]any{
					"url": "https://api.example.com",
				},
			},
		},
		{
			name: "Case 2: missing workspaceID",
			config: providers.Config{
				Creds: map[string]any{
					"token": "token-abc",
				},
				Params: map[string]any{
					"url": "https://api.example.com",
				},
			},
			err: "credentials error: missing 'workspaceId'",
		},
		{
			name: "Case 3: missing token",
			config: providers.Config{
				Creds: map[string]any{
					"workspaceId": "workspace-123",
				},
				Params: map[string]any{
					"url": "https://api.example.com",
				},
			},
			err: "credentials error: missing 'token'",
		},
		{
			name: "Case 4: missing baseURL",
			config: providers.Config{
				Creds: map[string]any{
					"workspaceId": "workspace-123",
					"token":       "token-abc",
				},
			},
			err: "parameters error: missing 'url'",
		},
		{
			name: "Case 5: invalid trimTiers",
			config: providers.Config{
				Creds: map[string]any{
					"workspaceId": "workspace-123",
					"token":       "token-abc",
				},
				Params: map[string]any{
					"url":                 "https://api.example.com",
					topology.KeyTrimTiers: false,
				},
			},
			err: "parameters error: invalid 'trimTiers' value 'false': unsupported type bool",
		},
		{
			name: "Case 6: empty baseURL",
			config: providers.Config{
				Creds: map[string]any{
					"workspaceId": "workspace-123",
					"token":       "token-abc",
				},
				Params: map[string]any{
					"url": "",
				},
			},
			err: "parameters error: missing 'url'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := Loader(ctx, tt.config)

			if len(tt.err) != 0 {
				require.Nil(t, provider)
				require.NotNil(t, err)
				require.Equal(t, http.StatusBadRequest, err.Code())
				require.Equal(t, tt.err, err.Error())
			} else {
				require.Nil(t, err)
				require.NotNil(t, provider)
			}
		})
	}
}

// TestGenerateTopologyConfig drives the real client against a mock of the Lambda
// topology API, asserting the verified request contract (path, region +
// workspace_id query, Bearer auth), the {data, page_token} response envelope,
// page_token pagination, and the networkPath -> leaf/spine/core mapping.
func TestGenerateTopologyConfig(t *testing.T) {
	ctx := context.Background()

	const page1 = `{"data":[
		{"id":"i-1","networkPath":[{"id":"leaf1"},{"id":"spine1"},{"id":"core1"}],"nvlink":null},
		{"id":"i-2","networkPath":[{"id":"leaf1"},{"id":"spine1"},{"id":"core1"}],"nvlink":null}
	],"page_token":"page2"}`
	const page2 = `{"data":[
		{"id":"i-3","networkPath":[{"id":"leaf2"},{"id":"spine1"},{"id":"core1"}],"nvlink":null}
	],"page_token":null}`

	var paths, regions, workspaces, auths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		paths = append(paths, r.URL.Path)
		regions = append(regions, r.URL.Query().Get("region"))
		workspaces = append(workspaces, r.URL.Query().Get("workspace_id"))
		auths = append(auths, r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page_token") == "" {
			_, _ = w.Write([]byte(page1))
		} else {
			_, _ = w.Write([]byte(page2))
		}
	}))
	defer server.Close()

	provider, httpErr := Loader(ctx, providers.Config{
		Creds: map[string]any{
			authWorkspaceID: "ws-1",
			authToken:       "tok-1",
		},
		Params: map[string]any{
			apiBaseURL: server.URL,
		},
	})
	require.Nil(t, httpErr)

	graph, httpErr := provider.GenerateTopologyConfig(ctx, nil, []topology.ComputeInstances{
		{
			Region:    "stg-sjc01-cl03",
			Instances: map[string]string{"i-1": "node1", "i-2": "node2", "i-3": "node3"},
		},
	})
	require.Nil(t, httpErr)
	require.NotNil(t, graph)

	// Two calls: the second is driven by the page_token returned on page 1.
	require.Equal(t, []string{apiPath, apiPath}, paths)
	require.Equal(t, []string{"stg-sjc01-cl03", "stg-sjc01-cl03"}, regions)
	require.Equal(t, []string{"ws-1", "ws-1"}, workspaces)
	require.Equal(t, []string{"Bearer tok-1", "Bearer tok-1"}, auths)

	// Both pages merged; networkPath objects mapped into the switch hierarchy.
	out, httpErr := slurm.GenerateOutput(ctx, graph, nil)
	require.Nil(t, httpErr)
	require.Equal(t, `SwitchName=core1 Switches=spine1
SwitchName=spine1 Switches=leaf[1-2]
SwitchName=leaf1 Nodes=node[1-2]
SwitchName=leaf2 Nodes=node3
`, string(out))
}
