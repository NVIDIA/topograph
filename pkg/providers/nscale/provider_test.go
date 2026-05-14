/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/stretchr/testify/require"
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
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
				},
				Creds: map[string]any{
					"org":    "org",
					"token":  "token",
					"region": "region",
				},
			},
		},
		{
			name: "Case 2: missing radarApiUrl",
			config: providers.Config{
				Params: map[string]any{
					"instanceApiUrl": "https://instances.test.com",
				},
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
			err: "missing 'radarApiUrl'",
		},
		{
			name: "Case 3: missing instanceApiUrl",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl": "https://radar.test.com",
				},
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
			err: "missing 'instanceApiUrl'",
		},
		{
			name: "Case 4: missing org",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
				},
				Creds: map[string]any{
					"token": "token",
				},
			},
			err: "missing 'org'",
		},
		{
			name: "Case 5: missing token",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
				},
				Creds: map[string]any{
					"org": "org",
				},
			},
			err: "missing 'token'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := Loader(ctx, tt.config)

			if len(tt.err) != 0 {
				require.Nil(t, provider)
				require.NotNil(t, err)
				require.Equal(t, http.StatusBadRequest, err.Code())
				require.Equal(t, err.Error(), tt.err)
			} else {
				require.NotNil(t, provider)
				require.Nil(t, err)
			}
		})
	}
}

func TestInstances2NodeMap(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v2/instances", r.URL.Path)
		require.Equal(t, "org", r.URL.Query().Get("organizationID"))
		require.Equal(t, "region", r.URL.Query().Get("regionID"))
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`[
			{"metadata":{"id":"instance-1","name":"node-1"}},
			{"metadata":{"id":"instance-2","name":"node-2"}},
			{"metadata":{"id":"instance-3","name":"outside-node"}}
		]`))
		require.NoError(t, err)
	}))
	defer server.Close()

	provider, httpErr := Loader(ctx, providers.Config{
		Params: map[string]any{
			"radarApiUrl":    "https://radar.test.com",
			"instanceApiUrl": server.URL,
		},
		Creds: map[string]any{
			"org":    "org",
			"token":  "token",
			"region": "region",
		},
	})
	require.Nil(t, httpErr)

	i2n, err := provider.(*Provider).Instances2NodeMap(ctx, []string{"node-1", "node-2"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"instance-1": "node-1",
		"instance-2": "node-2",
	}, i2n)

	i2n, err = provider.(*Provider).Instances2NodeMap(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"instance-1": "node-1",
		"instance-2": "node-2",
		"instance-3": "outside-node",
	}, i2n)

	provider, httpErr = Loader(ctx, providers.Config{
		Params: map[string]any{
			"radarApiUrl":    "https://radar.test.com",
			"instanceApiUrl": server.URL,
		},
		Creds: map[string]any{
			"org":   "org",
			"token": "token",
		},
	})
	require.Nil(t, httpErr)

	_, err = provider.(*Provider).Instances2NodeMap(ctx, []string{"node-1"})
	require.EqualError(t, err, "missing 'region'")
}
