/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetParameters(t *testing.T) {
	testCases := []struct {
		name   string
		params map[string]any
		ret    *Params
		err    string
	}{
		{
			name:   "Case 1: no params",
			params: nil,
			ret:    &Params{},
		},
		{
			name:   "Case 2: bad params",
			params: map[string]any{"nodeSelector": .1},
			err:    "could not decode configuration: 1 error(s) decoding:\n\n* 'nodeSelector' expected a map, got 'float64'",
		},
		{
			name:   "Case 3: valid input",
			params: map[string]any{"nodeSelector": map[string]string{"key": "val"}},
			ret: &Params{
				NodeSelector: map[string]string{"key": "val"},
				nodeListOpt: &metav1.ListOptions{
					LabelSelector: "key=val",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := getParameters(tc.params)
			if len(tc.err) != 0 {
				require.ErrorContains(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.ret, p)
			}
		})
	}
}
