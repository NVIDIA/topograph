/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"net/http"
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
					"baseUrl": "https://api.test.com",
				},
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
		},
		{
			name: "Case 2: missing baseUrl",
			config: providers.Config{
				Creds: map[string]any{
					"org":   "org",
					"token": "token",
				},
			},
			err: "missing 'baseUrl'",
		},
		{
			name: "Case 3: missing org",
			config: providers.Config{
				Params: map[string]any{
					"baseUrl": "https://api.test.com",
				},
				Creds: map[string]any{
					"token": "token",
				},
			},
			err: "missing 'org'",
		},
		{
			name: "Case 4: missing token",
			config: providers.Config{
				Params: map[string]any{
					"baseUrl": "https://api.test.com",
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
