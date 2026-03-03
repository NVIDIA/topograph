/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package lambdai

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/providers"
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
					authWorkspaceID: "workspace-123",
					authToken:       "token-abc",
				},
				Params: map[string]any{
					apiBaseURL: "https://api.example.com",
				},
			},
		},
		{
			name: "Case 2: missing workspaceID",
			config: providers.Config{
				Creds: map[string]any{
					authToken: "token-abc",
				},
				Params: map[string]any{
					apiBaseURL: "https://api.example.com",
				},
			},
			err: "credentials error: missing 'workspaceId'",
		},
		{
			name: "Case 3: missing token",
			config: providers.Config{
				Creds: map[string]any{
					authWorkspaceID: "workspace-123",
				},
				Params: map[string]any{
					apiBaseURL: "https://api.example.com",
				},
			},
			err: "credentials error: missing 'token'",
		},
		{
			name: "Case 4: missing baseURL",
			config: providers.Config{
				Creds: map[string]any{
					authWorkspaceID: "workspace-123",
					authToken:       "token-abc",
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
