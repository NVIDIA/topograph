/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

// ConfigureClientRateLimits applies optional client-go rate-limit overrides.
// When only one value is configured, the other uses the client-go default.
func ConfigureClientRateLimits(config *rest.Config, qps float32, burst int) {
	if qps == 0 && burst == 0 {
		return
	}
	qps, burst = normalizedClientRateLimits(qps, burst)
	config.QPS = qps
	config.Burst = burst
}

// ConfigureSharedClientRateLimiter installs one token bucket when either rate
// limit is configured, so clients created from config share it.
func ConfigureSharedClientRateLimiter(config *rest.Config, qps float32, burst int) {
	if qps == 0 && burst == 0 {
		return
	}
	qps, burst = normalizedClientRateLimits(qps, burst)
	config.QPS = qps
	config.Burst = burst
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(qps, burst)
}

func normalizedClientRateLimits(qps float32, burst int) (float32, int) {
	if qps == 0 {
		qps = rest.DefaultQPS
	}
	if burst == 0 {
		burst = rest.DefaultBurst
	}
	return qps, burst
}
