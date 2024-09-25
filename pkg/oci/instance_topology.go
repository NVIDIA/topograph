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

package oci

import (
	"context"
	"fmt"
	"net/http"
	"time"

	OCICommon "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
)

func GenerateInstanceTopology(ctx context.Context, creds OCICommon.ConfigurationProvider, cis []common.ComputeInstances) ([]*core.ComputeBareMetalHostSummary, error) {
	var err error
	bareMetalHostSummaries := []*core.ComputeBareMetalHostSummary{}
	for _, ci := range cis {
		if bareMetalHostSummaries, err = generateInstanceTopology(ctx, creds, &ci, bareMetalHostSummaries); err != nil {
			return nil, err
		}
	}

	return bareMetalHostSummaries, nil
}

func getComputeCapacityTopologies(ctx context.Context, computeClient core.ComputeClient, identityClient identity.IdentityClient,
	compartmentId string) (cct []core.ComputeCapacityTopologySummary, err error) {

	adRequest := identity.ListAvailabilityDomainsRequest{
		CompartmentId: &compartmentId,
	}

	timeStart := time.Now()
	ads, err := identityClient.ListAvailabilityDomains(ctx, adRequest)
	if err != nil {
		return cct, fmt.Errorf("unable to get AD: %v", err)
	}
	requestLatency.WithLabelValues("ListAvailabilityDomains", ads.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())

	for _, ad := range ads.Items {
		cctRequest := core.ListComputeCapacityTopologiesRequest{
			CompartmentId:      &compartmentId,
			AvailabilityDomain: ad.Name,
		}

		for {
			timeStart := time.Now()
			resp, err := computeClient.ListComputeCapacityTopologies(ctx, cctRequest)
			requestLatency.WithLabelValues("ListComputeCapacityTopologies", resp.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())
			if err != nil {
				if resp.HTTPResponse().StatusCode == http.StatusNotFound {
					return cct, fmt.Errorf("%v for getting ComputeCapacityTopology in %s: %v", resp.HTTPResponse().StatusCode, *ad.Name, err)
				} else {
					return cct, fmt.Errorf("unable to get ComputeCapacity Topologies in %s : %v", *ad.Name, err)
				}
			}
			cct = append(cct, resp.Items...)
			klog.V(4).Infof("Received computeCapacityTopology %d groups; processed %d", len(resp.Items), len(cct))
			if resp.OpcNextPage != nil {
				cctRequest.Page = resp.OpcNextPage
			} else {
				break
			}
		}
	}

	return cct, nil
}

func getBMHSummaryPerComputeCapacityTopology(ctx context.Context, computeClient core.ComputeClient, topologyID, compartmentId string) (bmhSummary []core.ComputeBareMetalHostSummary, err error) {
	request := core.ListComputeCapacityTopologyComputeBareMetalHostsRequest{
		ComputeCapacityTopologyId: &topologyID,
		CompartmentId:             &compartmentId,
	}
	for {
		timeStart := time.Now()
		response, err := computeClient.ListComputeCapacityTopologyComputeBareMetalHosts(ctx, request)
		requestLatency.WithLabelValues("ListComputeCapacityTopologyComputeBareMetalHosts", response.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())
		if err != nil {
			klog.Errorln(err.Error())
			break
		}

		bmhSummary = append(bmhSummary, response.Items...)

		if response.OpcNextPage != nil {
			request.Page = response.OpcNextPage
		} else {
			break
		}
	}
	return bmhSummary, nil
}

func getBareMetalHostSummaries(ctx context.Context, computeClient core.ComputeClient, identityClient identity.IdentityClient,
	compartmentId string) ([]core.ComputeBareMetalHostSummary, error) {

	computeCapacityTopology, err := getComputeCapacityTopologies(ctx, computeClient, identityClient, compartmentId)
	if err != nil {
		return nil, fmt.Errorf("unable to get compute capacity topologies: %s", err.Error())
	}
	klog.V(4).Infof("Received computeCapacityTopology for %d groups", len(computeCapacityTopology))

	var bareMetalHostSummaries []core.ComputeBareMetalHostSummary
	for _, cct := range computeCapacityTopology {
		bareMetalHostSummary, err := getBMHSummaryPerComputeCapacityTopology(ctx, computeClient, *cct.Id, compartmentId)
		if err != nil {
			return nil, fmt.Errorf("unable to get bare metal hosts info: %s", err.Error())
		}
		bareMetalHostSummaries = append(bareMetalHostSummaries, bareMetalHostSummary...)
	}
	klog.V(4).Infof("Returning bareMetalHostSummaries for %d nodes", len(bareMetalHostSummaries))

	return bareMetalHostSummaries, nil
}

func toGraph(bareMetalHostSummaries []*core.ComputeBareMetalHostSummary, cis []common.ComputeInstances) (*common.Vertex, error) {
	instanceToNodeMap := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			instanceToNodeMap[instance] = node
		}
	}
	klog.V(4).Infof("Instance/Node map %v", instanceToNodeMap)

	nodes := make(map[string]*common.Vertex)
	forest := make(map[string]*common.Vertex)

	for _, bmhSummary := range bareMetalHostSummaries {
		if bmhSummary.InstanceId == nil {
			klog.V(5).Infof("Skipped bmhSummary %s", bmhSummary.String())
			continue
		}
		nodeName, ok := instanceToNodeMap[*bmhSummary.InstanceId]
		if !ok {
			klog.V(5).Infof("Node not found for instance ID %s", *bmhSummary.InstanceId)
			continue
		}
		klog.V(4).Infof("Found node %q instance %q", nodeName, *bmhSummary.InstanceId)
		delete(instanceToNodeMap, *bmhSummary.InstanceId)

		instance := &common.Vertex{
			Name: nodeName,
			ID:   *bmhSummary.InstanceId,
		}

		localBlockId := "lb_nil"
		if bmhSummary.ComputeLocalBlockId != nil {
			localBlockId = *bmhSummary.ComputeLocalBlockId
		} else {
			klog.Warningf("ComputeLocalBlockId is nil for instance %q", *bmhSummary.InstanceId)
			missingAncestor.WithLabelValues("localBlock", nodeName).Add(float64(1))
		}

		localBlock, ok := nodes[localBlockId]
		if !ok {
			localBlock = &common.Vertex{
				ID:       localBlockId,
				Vertices: make(map[string]*common.Vertex),
			}
			nodes[localBlockId] = localBlock
		}
		localBlock.Vertices[instance.ID] = instance

		networkBlockId := "nw_nil"
		if bmhSummary.ComputeNetworkBlockId != nil {
			networkBlockId = *bmhSummary.ComputeNetworkBlockId
		} else {
			klog.Warningf("ComputeNetworkBlockId is nil for instance %q", *bmhSummary.InstanceId)
			missingAncestor.WithLabelValues("networkBlock", nodeName).Add(float64(1))
		}

		networkBlock, ok := nodes[networkBlockId]
		if !ok {
			networkBlock = &common.Vertex{
				ID:       networkBlockId,
				Vertices: make(map[string]*common.Vertex),
			}
			nodes[networkBlockId] = networkBlock
		}
		networkBlock.Vertices[localBlockId] = localBlock

		hpcIslandId := "hpc_nil"
		if bmhSummary.ComputeHpcIslandId != nil {
			hpcIslandId = *bmhSummary.ComputeHpcIslandId
		} else {
			klog.Warningf("ComputeHpcIslandId is nil for instance %q", *bmhSummary.InstanceId)
			missingAncestor.WithLabelValues("hpcIsland", nodeName).Add(float64(1))
		}
		hpcIsland, ok := nodes[hpcIslandId]
		if !ok {
			hpcIsland = &common.Vertex{
				ID:       hpcIslandId,
				Vertices: make(map[string]*common.Vertex),
			}
			nodes[hpcIslandId] = hpcIsland
			forest[hpcIslandId] = hpcIsland
		}
		hpcIsland.Vertices[networkBlockId] = networkBlock
	}

	if len(instanceToNodeMap) != 0 {
		klog.V(4).Infof("Adding unclaimed nodes %v", instanceToNodeMap)
		sw := &common.Vertex{
			ID:       "cpu-nodes",
			Vertices: make(map[string]*common.Vertex),
		}
		for instanceID, nodeName := range instanceToNodeMap {
			sw.Vertices[instanceID] = &common.Vertex{
				Name: nodeName,
				ID:   instanceID,
			}
		}
		forest["cpu-nodes"] = sw
	}

	root := &common.Vertex{
		Vertices: make(map[string]*common.Vertex),
	}
	for name, node := range forest {
		root.Vertices[name] = node
	}

	return root, nil

}

func generateInstanceTopology(ctx context.Context, provider OCICommon.ConfigurationProvider, ci *common.ComputeInstances, bareMetalHostSummaries []*core.ComputeBareMetalHostSummary) ([]*core.ComputeBareMetalHostSummary, error) {
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("unable to create identity client. Bailing out : %v", err)
	}

	tenacyOCID, err := provider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("unable to get tenancy OCID from config: %s", err.Error())
	}

	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("unable to get compute client: %s", err.Error())
	}

	if len(ci.Region) != 0 {
		klog.Infof("Use provided region %s", ci.Region)
		identityClient.SetRegion(ci.Region)
		computeClient.SetRegion(ci.Region)
	}

	bmh, err := getBareMetalHostSummaries(ctx, computeClient, identityClient, tenacyOCID)
	if err != nil {
		return nil, fmt.Errorf("unable to populate compute capacity topology: %s", err.Error())
	}

	for _, bm := range bmh {
		bareMetalHostSummaries = append(bareMetalHostSummaries, &bm)
	}
	return bareMetalHostSummaries, nil
}
