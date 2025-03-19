/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package gcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessNodeRegionOutput(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "Case 1: empty string",
		},
		{
			name: "Case 2: without delimeters",
			in: `region
`,
			out: "region",
		},
		{
			name: "Case 3: with delimeters",
			in: `projects/12345/zones/region
`,
			out: "region",
		},
	}
	var provider Provider
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, provider.ProcessNodeRegionOutput(tc.in))
		})
	}
}

func TestProcessNodeInstanceOutput(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "Case 1: empty string",
		},
		{
			name: "Case 2: without whitespaces",
			in:   "12345",
			out:  "12345",
		},
		{
			name: "Case 3: with whitespaces",
			in: ` 12345
`,
			out: "12345",
		},
	}
	var provider Provider
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, provider.ProcessNodeInstanceOutput(tc.in))
		})
	}
}
