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
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

// maxPaginationPages is the upper bound on pages fetched in a single topology
// request. It is a var so tests can lower it without allocating thousands of
// mock responses. At the default page size of 100, 10 000 pages covers
// 1 000 000 nodes — well beyond any realistic cluster.
var maxPaginationPages = 10_000

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, *httperr.Error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get client: %v", err))
	}

	// Build want set once — it never changes across pages.
	want := make(map[string]struct{})
	var nodeIDs []string
	for _, ci := range cis {
		for instanceID := range ci.Instances {
			want[instanceID] = struct{}{}
			nodeIDs = append(nodeIDs, instanceID)
		}
	}

	pageSizeVal := 0
	if pageSize != nil {
		pageSizeVal = *pageSize
	}

	// Phase 1: accumulate all switch entries across pages before resolving topology.
	// Building parentOf page-by-page would lose cross-page ancestry (e.g. spine on
	// page 1, leaves on page 2).
	//
	// seenTokens tracks every token we have already requested with. A cycle of any
	// length (A→B→A, A→B→C→A, …) is caught because any token we would send next
	// was necessarily already in the set.
	seenTokens := map[string]struct{}{"": {}}
	var allSwitches []map[string]SwitchAdjacency
	var pageToken string
	for page := 1; ; page++ {
		if page > maxPaginationPages {
			return nil, httperr.NewError(http.StatusBadGateway,
				fmt.Sprintf("DSX API exceeded maximum page limit (%d)", maxPaginationPages))
		}
		response, apiErr := client.GetTopology(ctx, "", nodeIDs, pageSizeVal, pageToken)
		if apiErr != nil {
			return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("API error: %v", apiErr))
		}
		allSwitches = append(allSwitches, response.Switches...)
		if response.NextPageToken == "" {
			break
		}
		if _, seen := seenTokens[response.NextPageToken]; seen {
			return nil, httperr.NewError(http.StatusBadGateway, "DSX API returned a page token cycle")
		}
		seenTokens[response.NextPageToken] = struct{}{}
		pageToken = response.NextPageToken
	}

	// Phase 2: resolve topology from the complete cross-page switch set.
	return buildClusterTopology(allSwitches, want), nil
}

// buildClusterTopology translates the complete ordered switch list into per-instance
// topology records. It must be called with the full set of switches across all pages
// so that cross-page parent-child relationships are resolved correctly.
func buildClusterTopology(switches []map[string]SwitchAdjacency, want map[string]struct{}) *topology.ClusterTopology {
	// First pass: build the parent map from the complete switch list.
	parentOf := make(map[string]string)
	for _, entry := range switches {
		if len(entry) > 1 {
			klog.Warningf("DSX API returned a SwitchEntry with %d keys; expected 1 (ordering non-deterministic)", len(entry))
		}
		for swName, adj := range entry {
			for _, child := range adj.Switches {
				parentOf[child] = swName
			}
		}
	}

	// Second pass: emit an InstanceTopology for each node attached to a leaf switch.
	topo := topology.NewClusterTopology()
	for _, entry := range switches {
		for swName, adj := range entry {
			for _, n := range adj.Nodes {
				if _, ok := want[n.NodeID]; !ok {
					continue
				}
				leafID := swName
				spineID := parentOf[leafID]
				coreID := ""
				if spineID != "" {
					coreID = parentOf[spineID]
				}

				// Only include non-empty tier IDs to avoid FabricTier{ID:""} entries
				// for nodes on 2-tier networks or when ancestry is absent.
				tierIDs := []string{leafID}
				if spineID != "" {
					tierIDs = append(tierIDs, spineID)
				}
				if coreID != "" {
					tierIDs = append(tierIDs, coreID)
				}

				inst := &topology.InstanceTopology{
					InstanceID:  n.NodeID,
					FabricTiers: topology.ClosestFirstFabricTiers(tierIDs...),
				}
				if n.AcceleratedNetworkID != "" {
					inst.XclrDomainID = n.AcceleratedNetworkID
				}
				klog.V(4).Infof("Adding instance topology %s", inst.String())
				topo.Append(inst)
			}
		}
	}

	return topo
}
