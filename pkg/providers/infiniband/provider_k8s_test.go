/*
 * Copyright 2025-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetParameters(t *testing.T) {
	tests := []struct {
		name          string
		params        map[string]any
		labelSelector string
		err           string
	}{
		{name: "no parameters"},
		{
			name:   "bad node selector",
			params: map[string]any{"nodeSelector": .1},
			err:    "could not decode configuration",
		},
		{
			name:          "node selector",
			params:        map[string]any{"nodeSelector": map[string]string{"key": "val"}},
			labelSelector: "key=val",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := getParameters(test.params)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			if test.labelSelector == "" {
				require.Nil(t, params.nodeListOpt)
			} else {
				require.Equal(t, &metav1.ListOptions{LabelSelector: test.labelSelector}, params.nodeListOpt)
			}
		})
	}
}
