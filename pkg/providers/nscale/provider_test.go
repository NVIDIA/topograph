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
					"placementId":    "placement-1",
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
					"placementId":    "placement-1",
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
					"placementId": "placement-1",
				},
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
			err: "missing 'instanceApiUrl'",
		},
		{
			name: "Case 4: missing placementId",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
				},
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
			err: "missing 'placementId'",
		},
		{
			name: "Case 5: missing org",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
					"placementId":    "placement-1",
				},
				Creds: map[string]any{
					"token": "token",
				},
			},
			err: "missing 'org'",
		},
		{
			name: "Case 6: missing token",
			config: providers.Config{
				Params: map[string]any{
					"radarApiUrl":    "https://radar.test.com",
					"instanceApiUrl": "https://instances.test.com",
					"placementId":    "placement-1",
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

const placementServersResponse = `[
  {
    "metadata": {
      "id": "psrv-7f3d9d5d2a7c4e32",
      "name": "training-workers-0",
      "organizationId": "9a8c6370-4065-4d4a-9da0-7678df40cd9d",
      "projectId": "e36c058a-8eba-4f5b-91f4-f6ffb983795c",
      "creationTime": "2026-04-28T11:04:00Z",
      "createdBy": "john.doe@example.com",
      "provisioningStatus": "provisioned",
      "healthStatus": "healthy"
    },
    "status": {
      "regionId": "c7568e2d-f9ab-453d-9a3a-51375f78426b",
      "reservationId": "a64f9269-36e0-4312-b8d1-52d93d569b7b",
      "placementId": "b8ce034e-fccb-4d6c-a0e0-af3e3f346715",
      "networkId": "61f0ad85-3001-41cb-824a-e6a047668dfe",
      "powerState": "Running",
      "privateIP": "10.0.0.12",
      "publicIP": "203.0.113.12",
      "macAddress": "fa:16:3e:7c:11:8a"
    }
  },
  {
    "metadata": {
      "id": "psrv-950ab3259a1443da",
      "name": "training-workers-1",
      "organizationId": "9a8c6370-4065-4d4a-9da0-7678df40cd9d",
      "projectId": "e36c058a-8eba-4f5b-91f4-f6ffb983795c",
      "creationTime": "2026-04-28T11:04:02Z",
      "createdBy": "john.doe@example.com",
      "provisioningStatus": "provisioned",
      "healthStatus": "healthy"
    },
    "status": {
      "regionId": "c7568e2d-f9ab-453d-9a3a-51375f78426b",
      "reservationId": "a64f9269-36e0-4312-b8d1-52d93d569b7b",
      "placementId": "b8ce034e-fccb-4d6c-a0e0-af3e3f346715",
      "networkId": "61f0ad85-3001-41cb-824a-e6a047668dfe",
      "powerState": "Running",
      "privateIP": "10.0.0.13",
      "macAddress": "fa:16:3e:11:70:2f"
    }
  }
]`

func newTestProvider(t *testing.T, ctx context.Context, serverURL string) *Provider {
	t.Helper()
	provider, httpErr := Loader(ctx, providers.Config{
		Params: map[string]any{
			"radarApiUrl":    "https://radar.test.com",
			"instanceApiUrl": serverURL,
			"placementId":    "placement-1",
		},
		Creds: map[string]any{
			"org":    "org",
			"token":  "token",
			"region": "region",
		},
	})
	require.Nil(t, httpErr)
	return provider.(*Provider)
}

func TestInstances2NodeMap(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v2/placements/placement-1/servers", r.URL.Path)
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(placementServersResponse))
		require.NoError(t, err)
	}))
	defer server.Close()

	p := newTestProvider(t, ctx, server.URL)

	i2n, err := p.Instances2NodeMap(ctx, []string{"training-workers-0", "training-workers-1"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"psrv-7f3d9d5d2a7c4e32": "training-workers-0",
		"psrv-950ab3259a1443da": "training-workers-1",
	}, i2n)

	i2n, err = p.Instances2NodeMap(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"psrv-7f3d9d5d2a7c4e32": "training-workers-0",
		"psrv-950ab3259a1443da": "training-workers-1",
	}, i2n)
}

func TestPlacementServersErrors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "400 invalid request",
			statusCode: http.StatusBadRequest,
			body:       `{"error":"invalid_request","error_description":"request body invalid","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
		},
		{
			name:       "401 authentication failed",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":"access_denied","error_description":"authentication failed","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"error":"forbidden","error_description":"user credentials do not have the required privileges","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
		},
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			body:       `{"error":"not_found","error_description":"the requested resource does not exist","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
		},
		{
			name:       "500 server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":"server_error","error_description":"failed to token claim","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.body))
				require.NoError(t, err)
			}))
			defer server.Close()

			p := newTestProvider(t, ctx, server.URL)
			_, err := p.Instances2NodeMap(ctx, nil)
			require.ErrorContains(t, err, "failed to get placement servers")
		})
	}
}
