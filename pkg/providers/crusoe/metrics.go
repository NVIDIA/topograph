/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"github.com/prometheus/client_golang/prometheus"
)

var nodesProcessed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name:      "nodes_processed_total",
		Help:      "Nodes processed per topology generation, by outcome",
		Subsystem: "topograph_crusoe",
	},
	[]string{"outcome"}, // infiniband, no_infiniband, skipped
)

func init() {
	prometheus.MustRegister(nodesProcessed)
}
