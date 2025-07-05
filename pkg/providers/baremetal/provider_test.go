/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package baremetal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/providers"
)

func TestLoader(t *testing.T) {
	ctx := context.TODO()

	testCases := []struct {
		name   string
		config providers.Config
		err    string
	}{
		{
			name:   "Case 1: empty config",
			config: providers.Config{},
		},
		{
			name: "Case 2: missing netqApiUrl",
			config: providers.Config{
				Params: map[string]any{
					"netqLoginUrl": "url",
				},
			},
			err: "netqApiUrl is required when netqLoginUrl is set",
		},
		{
			name: "Case 3: missing netqLoginUrl",
			config: providers.Config{
				Params: map[string]any{
					"netqApiUrl": "url",
				},
			},
			err: "netqLoginUrl is required when netqApiUrl is set",
		},
		{
			name: "Case 4: no creds for netQ",
			config: providers.Config{
				Params: map[string]any{
					"netqLoginUrl": "url",
					"netqApiUrl":   "url",
				},
			},
			err: "username not provided",
		},
		{
			name: "Case 5: missing password",
			config: providers.Config{
				Params: map[string]any{
					"netqLoginUrl": "url",
					"netqApiUrl":   "url",
				},
				Creds: map[string]string{
					"username": "user",
				},
			},
			err: "password not provided",
		},
		{
			name: "Case 6: valid input",
			config: providers.Config{
				Params: map[string]any{
					"netqLoginUrl": "url",
					"netqApiUrl":   "url",
				},
				Creds: map[string]string{
					"username": "user",
					"password": "pwd",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Loader(ctx, tc.config)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
