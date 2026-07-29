/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	envKubeQPS   = "KUBE_QPS"
	envKubeBurst = "KUBE_BURST"
)

// ConfigureClientRateLimits applies client-go rate limits from the environment.
// When only one value is set, the other uses the client-go default. The
// explicit limiter is shared by every client created from config.
func ConfigureClientRateLimits(config *rest.Config) error {
	qps, hasQPS, err := envFloat32(envKubeQPS)
	if err != nil {
		return err
	}
	burst, hasBurst, err := envInt(envKubeBurst)
	if err != nil {
		return err
	}
	if !hasQPS && !hasBurst {
		return nil
	}

	qps, burst = normalizedClientRateLimits(qps, burst)
	config.QPS = qps
	config.Burst = burst
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(qps, burst)
	return nil
}

func envFloat32(name string) (float32, bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return 0, false, nil
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0, false, fmt.Errorf("%s must be a non-negative number", name)
	}
	return float32(parsed), true, nil
}

func envInt(name string) (int, bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return 0, false, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 0)
	if err != nil || parsed < 0 {
		return 0, false, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return int(parsed), true, nil
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
