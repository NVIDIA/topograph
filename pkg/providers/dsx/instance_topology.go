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

package dsx

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (p *Provider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, []topology.ComputeInstances, *httperr.Error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get client: %v", err))
	}

	var nodeIDs []string
	for _, ci := range cis {
		for instanceID := range ci.Instances {
			nodeIDs = append(nodeIDs, instanceID)
		}
	}

	ps := pageSizeVal(pageSize, p.params)
	ctx = contextWithDSXPageSize(ctx, ps)

	response, cisEff, apiErr := client.GetTopology(ctx, p.params.VpcID, nodeIDs, cis)
	if apiErr != nil {
		var hse *httpStatusError
		if errors.As(apiErr, &hse) && hse.code >= 400 && hse.code < 500 {
			return nil, nil, httperr.NewError(hse.code, hse.Error())
		}
		return nil, nil, httperr.NewError(http.StatusBadGateway, apiErr.Error())
	}
	topo := responseToClusterTopology(response, cisEff)
	return topo, cisEff, nil
}

func pageSizeVal(pageSize *int, lp *Params) int {
	if pageSize != nil && *pageSize > 0 {
		return *pageSize
	}
	if lp.PageSize > 0 {
		return lp.PageSize
	}
	return 1000
}

// effectiveComputeInstances returns the request instances, or a synthetic map of
// node_id -> node_id when the request is empty so [topology.ClusterTopology.ToThreeTierGraph]
// can resolve instance IDs from an unconstrained API response.
func effectiveComputeInstances(response *TopologyResponse, cis []topology.ComputeInstances) []topology.ComputeInstances {
	if len(topology.GetNodeNameList(cis)) > 0 {
		return cis
	}
	m := make(map[string]string)
	for _, info := range response.Switches {
		for _, n := range info.Nodes {
			m[n.NodeID] = n.NodeID
		}
	}
	if len(m) == 0 {
		return cis
	}
	return []topology.ComputeInstances{{Region: defaultRegion, Instances: m}}
}

// responseToClusterTopology maps switch/node API output to per-instance records for ToThreeTierGraph.
func responseToClusterTopology(response *TopologyResponse, cis []topology.ComputeInstances) *topology.ClusterTopology {
	want, idByNodeID := computeInstanceLookups(cis)

	parentOf := make(map[string]string)
	for swName, info := range response.Switches {
		for _, child := range info.Switches {
			parentOf[child] = swName
		}
	}

	topo := topology.NewClusterTopology()
	for swName, info := range response.Switches {
		for _, n := range info.Nodes {
			if len(want) > 0 && !want[n.NodeID] {
				continue
			}
			leafID := swName
			spineID := parentOf[leafID]
			coreID := ""
			if spineID != "" {
				coreID = parentOf[spineID]
			}

			instID := idByNodeID[n.NodeID]
			if instID == "" {
				instID = n.NodeID
			}

			inst := &topology.InstanceTopology{
				InstanceID:    instID,
				LeafID:        leafID,
				SpineID:       spineID,
				CoreID:        coreID,
				AcceleratorID: n.AcceleratedNetworkID,
			}
			klog.V(4).InfoS("dsx instance topology", "record", inst.String())
			topo.Append(inst)
		}
	}

	return topo
}

// computeInstanceLookups returns accepted API node_id keys for filtering when cis is non-empty,
// and node_id (hostname or instance id string) -> instance ID for [topology.ClusterTopology.ToThreeTierGraph].
func computeInstanceLookups(cis []topology.ComputeInstances) (want map[string]bool, idByNodeID map[string]string) {
	if len(cis) == 0 {
		return nil, nil
	}
	want = make(map[string]bool)
	idByNodeID = make(map[string]string)
	for _, ci := range cis {
		for instID, nodeName := range ci.Instances {
			want[instID] = true
			want[nodeName] = true
			idByNodeID[nodeName] = instID
			idByNodeID[instID] = instID
		}
	}
	return want, idByNodeID
}
