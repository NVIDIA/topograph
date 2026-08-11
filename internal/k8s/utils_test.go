/*
 * Copyright 2025-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestIsPodReady(t *testing.T) {
	testCases := []struct {
		name  string
		pod   *corev1.Pod
		ready bool
	}{
		{
			name: "Case 1: ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.DisruptionTarget,
							Status: corev1.ConditionUnknown,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			ready: true,
		},
		{
			name: "Case 2: implicit not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.DisruptionTarget,
							Status: corev1.ConditionUnknown,
						},
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			ready: false,
		},
		{
			name: "Case 3: explicit not ready",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.DisruptionTarget,
							Status: corev1.ConditionUnknown,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			ready: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.ready, IsPodReady(tc.pod))
		})
	}
}

func TestValidateLabelKey(t *testing.T) {
	testCases := []struct {
		name string
		key  string
		err  string
	}{
		{name: "valid", key: "example.com/accelerator-domain"},
		{
			name: "empty",
			err:  `acceleratorDomainSourceLabel "" is not a valid Kubernetes label key`,
		},
		{
			name: "invalid",
			key:  "not a label",
			err:  `acceleratorDomainSourceLabel "not a label" is not a valid Kubernetes label key`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLabelKey("acceleratorDomainSourceLabel", tc.key)
			if tc.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.err)
		})
	}
}
