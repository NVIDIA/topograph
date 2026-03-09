/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAuthOption(t *testing.T) {
	tests := []struct {
		name  string
		creds map[string]any
		env   bool
		err   string
	}{
		{
			name:  "Case 1.1: no serviceAccountID in creds",
			creds: map[string]any{"a": "b"},
			err:   "credentials error: missing 'serviceAccountId'",
		},
		{
			name:  "Case 1.2: no publicKeyID in creds",
			creds: map[string]any{"serviceAccountId": "service-account"},
			err:   "credentials error: missing 'publicKeyId'",
		},
		{
			name: "Case 1.3: no privateKey in creds",
			creds: map[string]any{
				"serviceAccountId": "service-account",
				"publicKeyId":      "data",
			},
			err: "credentials error: missing 'privateKey'",
		},
		{
			name: "Case 1.4: valid creds",
			creds: map[string]any{
				"serviceAccountId": "service-account",
				"publicKeyId":      "id",
				"privateKey":       "key",
			},
		},
		{
			name: "Case 2: valid env var",
			env:  true,
		},
		{
			name: "Case 3: no creds",
			err:  "missing authentication credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env {
				os.Setenv(authTokenEnvVar, "data")
				defer os.Unsetenv(authTokenEnvVar)
			}
			_, err := getAuthOption(tt.creds)
			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestGetUserAgentPrefix(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "Case 1: empty version",
			version:  "",
			expected: userAgentProduct,
		},
		{
			name:     "Case 2: whitespace version",
			version:  "   ",
			expected: userAgentProduct,
		},
		{
			name:     "Case 3: non-empty version",
			version:  "main",
			expected: "nvidia-topograph/main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, getUserAgentPrefix(tt.version))
		})
	}
}
