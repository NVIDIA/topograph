/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nfd

import (
	"context"
	"net/http"

	"github.com/NVIDIA/topograph/internal/httperr"
	internalk8s "github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (eng *NfdEngine) GetComputeInstances(ctx context.Context, _ any) ([]topology.ComputeInstances, *httperr.Error) {
	nodes, err := internalk8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	eng.cachedNodes = nodes
	return internalk8s.GetComputeInstances(nodes), nil
}
