/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewController(t *testing.T) {
	ctx := context.TODO()

	cfg := &Config{
		Provider: "test",
		Engine:   "test",
	}

	testCases := []struct {
		name    string
		trigger Trigger
		err     string
	}{
		{
			name: "Case 1: bad trigger",
			trigger: Trigger{
				PodSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Operator: "BAD"},
					},
				},
			},
			err: `"BAD" is not a valid label selector operator`,
		},
		{
			name: "Case 2: valid input",
			trigger: Trigger{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "val"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "key", Operator: "In", Values: []string{"val"}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Trigger = tc.trigger
			_, err := NewController(ctx, nil, cfg)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
