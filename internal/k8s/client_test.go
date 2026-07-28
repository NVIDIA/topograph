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
	defaultConfig := &rest.Config{}
	ConfigureSharedClientRateLimiter(defaultConfig, 0, 0)
	require.Zero(t, defaultConfig.QPS)
	require.Zero(t, defaultConfig.Burst)
	require.Nil(t, defaultConfig.RateLimiter)

	configuredConfig := &rest.Config{}
	ConfigureSharedClientRateLimiter(configuredConfig, 0.001, 3)
	require.Equal(t, float32(0.001), configuredConfig.QPS)
	require.Equal(t, 3, configuredConfig.Burst)
	require.Equal(t, float32(0.001), configuredConfig.RateLimiter.QPS())
	require.True(t, configuredConfig.RateLimiter.TryAccept())
	require.True(t, configuredConfig.RateLimiter.TryAccept())
	require.True(t, configuredConfig.RateLimiter.TryAccept())
	require.False(t, configuredConfig.RateLimiter.TryAccept())
}
