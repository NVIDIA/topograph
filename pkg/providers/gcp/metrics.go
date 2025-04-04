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

package gcp

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	physicalHostIDChunks = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "physical_host_id_chunks",
			Subsystem: "topograph_gcp",
			Help:      "Number of chunks in physical host ID as in /AA/BB/CCC",
		}, []string{"instance_name"},
	)

	resourceStatusNotFound = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "resource_status_not_found",
			Subsystem: "topograph_gcp",
			Help:      "Number of times resource status not found",
		}, []string{"instance_name"},
	)

	physicalHostNotFound = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "physical_host_not_found",
			Subsystem: "topograph_gcp",
			Help:      "Number of times physical host not found",
		}, []string{"instance_name"},
	)

	requestLatency = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "request_latency",
			Subsystem:  "topograph_gcp",
			Help:       "Latency of requests",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"method"},
	)
)

func init() {
	prometheus.MustRegister(physicalHostIDChunks)
	prometheus.MustRegister(resourceStatusNotFound)
	prometheus.MustRegister(physicalHostNotFound)
	prometheus.MustRegister(requestLatency)
}
