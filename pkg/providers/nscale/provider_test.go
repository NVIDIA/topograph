/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/topograph/internal/httperr"
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

const placementsResponse = `[
  {"metadata": {"id": "placement-1"}},
  {"metadata": {"id": "placement-2"}}
]`

const placementServersResponseTmpl = `[
  {
    "metadata": {
      "id": "%s",
      "name": "%s",
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
      "placementId": "%s",
      "networkId": "61f0ad85-3001-41cb-824a-e6a047668dfe",
      "powerState": "Running",
      "privateIP": "10.0.0.12",
      "macAddress": "fa:16:3e:7c:11:8a"
    }
  }
]`

func newTestProvider(t *testing.T, ctx context.Context, serverURL string) *Provider {
	t.Helper()
	provider, httpErr := Loader(ctx, providers.Config{
		Params: map[string]any{
			"radarApiUrl":    "https://radar.test.com",
			"instanceApiUrl": serverURL,
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
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v2/placements":
			require.Equal(t, "org", r.URL.Query().Get("organizationID"))
			require.Equal(t, "region", r.URL.Query().Get("regionID"))
			_, err := w.Write([]byte(placementsResponse))
			require.NoError(t, err)
		case "/api/v2/placements/placement-1/servers":
			_, err := fmt.Fprintf(w, placementServersResponseTmpl, "psrv-7f3d9d5d2a7c4e32", "training-workers-0", "placement-1")
			require.NoError(t, err)
		case "/api/v2/placements/placement-2/servers":
			_, err := fmt.Fprintf(w, placementServersResponseTmpl, "psrv-950ab3259a1443da", "training-workers-1", "placement-2")
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
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

const placementServersMissingFieldsResponse = `[
  {"metadata": {"id": "psrv-1", "name": "training-workers-0"}},
  {"metadata": {"id": "", "name": "training-workers-1"}},
  {"metadata": {"id": "psrv-2", "name": ""}},
  {"metadata": {"id": "psrv-3", "name": "training-workers-2"}}
]`

func TestPlacementServers(t *testing.T) {
	ctx := context.Background()

	t.Run("excludes servers missing id or name", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(placementServersMissingFieldsResponse))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}
		i2n, err := c.PlacementServers(ctx, "placement-1")
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"psrv-1": "training-workers-0",
			"psrv-3": "training-workers-2",
		}, i2n)
	})

	t.Run("malformed JSON returns an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{not valid json`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}
		i2n, err := c.PlacementServers(ctx, "placement-1")
		require.Error(t, err)
		require.Nil(t, i2n)
	})

	t.Run("canceled context returns promptly without retries", func(t *testing.T) {
		var requests int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(placementsResponse))
			require.NoError(t, err)
		}))
		defer server.Close()

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}

		start := time.Now()
		i2n, err := c.PlacementServers(cancelCtx, "placement-1")
		elapsed := time.Since(start)

		require.Error(t, err)
		require.Nil(t, i2n)
		require.Less(t, elapsed, time.Second, "canceled context should fail immediately, not retry with backoff")
		require.Equal(t, int32(0), atomic.LoadInt32(&requests), "canceled context should not reach the server")
	})
}

const placementsResponseWithEmptyID = `[
  {"metadata": {"id": "placement-1"}},
  {"metadata": {"id": ""}},
  {"metadata": {"id": "placement-2"}}
]`

func TestListPlacements(t *testing.T) {
	ctx := context.Background()

	t.Run("excludes placements missing id", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(placementsResponseWithEmptyID))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}
		ids, err := c.ListPlacements(ctx, "org", "region")
		require.NoError(t, err)
		require.Equal(t, []string{"placement-1", "placement-2"}, ids)
	})

	t.Run("empty list returns no ids and no error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`[]`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}
		ids, err := c.ListPlacements(ctx, "org", "region")
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("malformed JSON returns HTTP 502", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{not valid json`))
			require.NoError(t, err)
		}))
		defer server.Close()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}
		ids, err := c.ListPlacements(ctx, "org", "region")
		require.Error(t, err)
		require.Nil(t, ids)

		var httpErr *httperr.Error
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusBadGateway, httpErr.Code())
	})

	t.Run("canceled context returns without issuing a request", func(t *testing.T) {
		var requests int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(placementsResponse))
			require.NoError(t, err)
		}))
		defer server.Close()

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()

		c := &nscaleClient{instanceAPIURL: server.URL, token: "token"}

		start := time.Now()
		ids, err := c.ListPlacements(cancelCtx, "org", "region")
		elapsed := time.Since(start)

		require.Error(t, err)
		require.Nil(t, ids)
		require.Less(t, elapsed, time.Second, "canceled context should fail immediately, not retry with backoff")
		require.Equal(t, int32(0), atomic.LoadInt32(&requests), "canceled context should not reach the server")
	})
}

func TestInstances2NodeMapErrors(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		failPlacements   bool
		statusCode       int
		body             string
		wantErrSubstring string
	}{
		{
			name:             "400 invalid request listing placements",
			failPlacements:   true,
			statusCode:       http.StatusBadRequest,
			body:             `{"error":"invalid_request","error_description":"request body invalid","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
			wantErrSubstring: "failed to list placements",
		},
		{
			name:             "401 authentication failed listing placements",
			failPlacements:   true,
			statusCode:       http.StatusUnauthorized,
			body:             `{"error":"access_denied","error_description":"authentication failed","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
			wantErrSubstring: "failed to list placements",
		},
		{
			name:             "404 not found listing placements",
			failPlacements:   true,
			statusCode:       http.StatusNotFound,
			body:             `{"error":"not_found","error_description":"the requested resource does not exist","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
			wantErrSubstring: "failed to list placements",
		},
		{
			name:             "403 forbidden fetching placement servers",
			statusCode:       http.StatusForbidden,
			body:             `{"error":"forbidden","error_description":"user credentials do not have the required privileges","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
			wantErrSubstring: "failed to get placement servers",
		},
		{
			name:             "500 server error fetching placement servers",
			statusCode:       http.StatusInternalServerError,
			body:             `{"error":"server_error","error_description":"failed to token claim","trace_id":"57bc14d9bd461f0b5a72db830149b67a"}`,
			wantErrSubstring: "failed to get placement servers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				if r.URL.Path == "/api/v2/placements" {
					if tt.failPlacements {
						w.WriteHeader(tt.statusCode)
						_, err := w.Write([]byte(tt.body))
						require.NoError(t, err)
						return
					}
					_, err := w.Write([]byte(placementsResponse))
					require.NoError(t, err)
					return
				}

				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.body))
				require.NoError(t, err)
			}))
			defer server.Close()

			p := newTestProvider(t, ctx, server.URL)
			_, err := p.Instances2NodeMap(ctx, nil)
			require.ErrorContains(t, err, tt.wantErrSubstring)

			var httpErr *httperr.Error
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, tt.statusCode, httpErr.Code())
		})
	}
}
