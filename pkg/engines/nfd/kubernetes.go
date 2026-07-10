/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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
