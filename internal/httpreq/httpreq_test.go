/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package httpreq

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetURL(t *testing.T) {
	testCases := []struct {
		name    string
		baseURL string
		paths   []string
		query   map[string]string
		url     string
		err     string
	}{
		{
			name:    "Case 1: bad base URL",
			baseURL: "123:",
			err:     `parse "123:": first path segment in URL cannot contain colon`,
		},
		{
			name:    "Case 2: single base URL",
			baseURL: "http://localhost",
			url:     "http://localhost",
		},
		{
			name:    "Case 3: base URL with path",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			url:     "http://localhost/a/b/c/d",
		},
		{
			name:    "Case 4: base URL with path and query",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			query:   map[string]string{"key1": "val1", "key2": "val2"},
			url:     "http://localhost/a/b/c/d?key1=val1&key2=val2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := GetURL(tc.baseURL, tc.query, tc.paths...)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {

				require.Nil(t, err)
				require.Equal(t, tc.url, u)
			}
		})
	}
}
