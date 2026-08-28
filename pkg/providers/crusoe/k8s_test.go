/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFabricLabels(t *testing.T) {
	testCases := []struct {
		name   string
		labels map[string]string
		want   fabricLabels
		ok     bool
	}{
		{
			name: "both labels present",
			labels: map[string]string{
				labelPartitionID: "partition-uuid",
				labelPodID:       "pod-uuid",
			},
			want: fabricLabels{PartitionID: "partition-uuid", PodID: "pod-uuid"},
			ok:   true,
		},
		{
			name: "surrounding whitespace is trimmed",
			labels: map[string]string{
				labelPartitionID: " partition-uuid ",
				labelPodID:       " pod-uuid ",
			},
			want: fabricLabels{PartitionID: "partition-uuid", PodID: "pod-uuid"},
			ok:   true,
		},
		{
			name:   "no labels at all",
			labels: map[string]string{},
		},
		{
			name:   "partition without a pod",
			labels: map[string]string{labelPartitionID: "partition-uuid"},
		},
		{
			name:   "pod without a partition",
			labels: map[string]string{labelPodID: "pod-uuid"},
		},
		{
			name: "empty label values",
			labels: map[string]string{
				labelPartitionID: "",
				labelPodID:       "",
			},
		},
		{
			name: "whitespace-only label values",
			labels: map[string]string{
				labelPartitionID: "   ",
				labelPodID:       "pod-uuid",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := readFabricLabels(tc.labels)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
