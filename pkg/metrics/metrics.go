/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "requests_total",
			Help:      "Total number of topology generation requests.",
			Subsystem: "topograph",
		},
		[]string{"provider", "engine", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      "request_duration_seconds",
			Help:      "Topology generator request duration in seconds.",
			Subsystem: "topograph",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"provider", "engine", "status"},
	)

	missingTopologyNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "missing_topology",
			Help:      "Nodes with missing topology information.",
			Subsystem: "topograph",
		},
		[]string{"provider", "node"},
	)

	validationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "validation_error_total",
			Help:      "Total number of validation errors.",
			Subsystem: "topograph",
		},
		[]string{"type"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(missingTopologyNodes)
	prometheus.MustRegister(validationErrorsTotal)
}

func Add(provider, engine string, code int, duration time.Duration) {
	status := fmt.Sprintf("%d", code)
	httpRequestsTotal.WithLabelValues(provider, engine, status).Inc()
	httpRequestDuration.WithLabelValues(provider, engine, status).Observe(duration.Seconds())
}

func SetMissingTopology(provider, nodename string) {
	missingTopologyNodes.WithLabelValues(provider, nodename).Set(1.0)
}

func AddValidationError(errorType string) {
	validationErrorsTotal.WithLabelValues(errorType).Inc()
}
