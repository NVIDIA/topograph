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
	testCases := []struct {
		name  string
		creds map[string]string
		env   bool
		err   string
	}{
		{
			name:  "Case 1.1: no serviceAccountID in creds",
			creds: map[string]string{"a": "b"},
			err:   "credentials error: missing service-account-id",
		},
		{
			name:  "Case 1.2: no publicKeyID in creds",
			creds: map[string]string{"service-account-id": "service-account"},
			err:   "credentials error: missing public-key-id",
		},
		{
			name: "Case 1.3: no privateKey in creds",
			creds: map[string]string{
				"service-account-id": "service-account",
				"public-key-id":      "data",
			},
			err: "credentials error: missing private-key",
		},
		{
			name: "Case 1.4: valid creds",
			creds: map[string]string{
				"service-account-id": "service-account",
				"public-key-id":      "id",
				"private-key":        "key",
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env {
				os.Setenv(authTokenEnvVar, "data")
				defer os.Unsetenv(authTokenEnvVar)
			}
			_, err := getAuthOption(tc.creds)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
