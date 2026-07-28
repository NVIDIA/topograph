/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestConfigureClientRateLimits(t *testing.T) {
	testCases := []struct {
		name      string
		qps       float32
		burst     int
		wantQPS   float32
		wantBurst int
	}{
		{
			name: "no overrides",
		},
		{
			name:      "both overrides",
			qps:       50,
			burst:     100,
			wantQPS:   50,
			wantBurst: 100,
		},
		{
			name:      "QPS only",
			qps:       50,
			wantQPS:   50,
			wantBurst: rest.DefaultBurst,
		},
		{
			name:      "burst only",
			burst:     100,
			wantQPS:   rest.DefaultQPS,
			wantBurst: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &rest.Config{}
			ConfigureClientRateLimits(config, tc.qps, tc.burst)
			require.Equal(t, tc.wantQPS, config.QPS)
			require.Equal(t, tc.wantBurst, config.Burst)
			require.Nil(t, config.RateLimiter)
		})
	}
}

func TestConfigureSharedClientRateLimiter(t *testing.T) {
	config := &rest.Config{}
	ConfigureSharedClientRateLimiter(config, 0, 0)
	require.Equal(t, rest.DefaultQPS, config.QPS)
	require.Equal(t, rest.DefaultBurst, config.Burst)
	require.NotNil(t, config.RateLimiter)
}
