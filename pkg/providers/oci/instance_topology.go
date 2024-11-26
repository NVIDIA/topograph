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
	"sort"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type level int

const (
	localBlockLevel level = iota + 1
	networkBlockLevel
	hpcIslandLevel
)

func GenerateInstanceTopology(ctx context.Context, factory ClientFactory, cis []topology.ComputeInstances) ([]*core.ComputeBareMetalHostSummary, error) {
	var err error
	bareMetalHostSummaries := []*core.ComputeBareMetalHostSummary{}
	for _, ci := range cis {
		if bareMetalHostSummaries, err = generateInstanceTopology(ctx, factory, &ci, bareMetalHostSummaries); err != nil {
			return nil, err
		}
	}

	return bareMetalHostSummaries, nil
}

func getComputeCapacityTopologies(ctx context.Context, client Client) (cct []core.ComputeCapacityTopologySummary, err error) {
	compartmentId := client.TenancyOCID()

	adRequest := identity.ListAvailabilityDomainsRequest{
		CompartmentId: &compartmentId,
	}

	timeStart := time.Now()
	ads, err := client.ListAvailabilityDomains(ctx, adRequest)
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
			resp, err := client.ListComputeCapacityTopologies(ctx, cctRequest)
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

func getBMHSummaryPerComputeCapacityTopology(ctx context.Context, client Client, topologyID string) (bmhSummary []core.ComputeBareMetalHostSummary, err error) {
	compartmentId := client.TenancyOCID()
	request := core.ListComputeCapacityTopologyComputeBareMetalHostsRequest{
		ComputeCapacityTopologyId: &topologyID,
		CompartmentId:             &compartmentId,
	}
	for {
		timeStart := time.Now()
		response, err := client.ListComputeCapacityTopologyComputeBareMetalHosts(ctx, request)
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

func getBareMetalHostSummaries(ctx context.Context, client Client) ([]core.ComputeBareMetalHostSummary, error) {
	computeCapacityTopology, err := getComputeCapacityTopologies(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("unable to get compute capacity topologies: %s", err.Error())
	}
	klog.V(4).Infof("Received computeCapacityTopology for %d groups", len(computeCapacityTopology))

	var bareMetalHostSummaries []core.ComputeBareMetalHostSummary
	for _, cct := range computeCapacityTopology {
		bareMetalHostSummary, err := getBMHSummaryPerComputeCapacityTopology(ctx, client, *cct.Id)
		if err != nil {
			return nil, fmt.Errorf("unable to get bare metal hosts info: %s", err.Error())
		}
		bareMetalHostSummaries = append(bareMetalHostSummaries, bareMetalHostSummary...)
	}
	klog.V(4).Infof("Returning bareMetalHostSummaries for %d nodes", len(bareMetalHostSummaries))

	return bareMetalHostSummaries, nil
}

func toGraph(bareMetalHostSummaries []*core.ComputeBareMetalHostSummary, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	instanceToNodeMap := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			instanceToNodeMap[instance] = node
		}
	}
	klog.V(4).Infof("Instance/Node map %v", instanceToNodeMap)

	nodes := make(map[string]*topology.Vertex)
	forest := make(map[string]*topology.Vertex)
	levelWiseSwitchCount := map[level]int{localBlockLevel: 0, networkBlockLevel: 0, hpcIslandLevel: 0}
	bareMetalHostSummaries = filterAndSort(bareMetalHostSummaries, instanceToNodeMap)
	for _, bmhSummary := range bareMetalHostSummaries {
		nodeName := instanceToNodeMap[*bmhSummary.InstanceId]
		delete(instanceToNodeMap, *bmhSummary.InstanceId)

		instance := &topology.Vertex{
			Name: nodeName,
			ID:   *bmhSummary.InstanceId,
		}

		localBlockId := *bmhSummary.ComputeLocalBlockId
		localBlock, ok := nodes[localBlockId]
		if !ok {
			levelWiseSwitchCount[localBlockLevel]++
			localBlock = &topology.Vertex{
				ID:       localBlockId,
				Vertices: make(map[string]*topology.Vertex),
				Name:     fmt.Sprintf("Switch.%d.%d", localBlockLevel, levelWiseSwitchCount[localBlockLevel]),
			}
			nodes[localBlockId] = localBlock
		}
		localBlock.Vertices[instance.ID] = instance

		networkBlockId := *bmhSummary.ComputeNetworkBlockId
		networkBlock, ok := nodes[networkBlockId]
		if !ok {
			levelWiseSwitchCount[networkBlockLevel]++
			networkBlock = &topology.Vertex{
				ID:       networkBlockId,
				Vertices: make(map[string]*topology.Vertex),
				Name:     fmt.Sprintf("Switch.%d.%d", networkBlockLevel, levelWiseSwitchCount[networkBlockLevel]),
			}
			nodes[networkBlockId] = networkBlock
		}
		networkBlock.Vertices[localBlockId] = localBlock

		hpcIslandId := *bmhSummary.ComputeHpcIslandId
		hpcIsland, ok := nodes[hpcIslandId]
		if !ok {
			levelWiseSwitchCount[hpcIslandLevel]++
			hpcIsland = &topology.Vertex{
				ID:       hpcIslandId,
				Vertices: make(map[string]*topology.Vertex),
				Name:     fmt.Sprintf("Switch.%d.%d", hpcIslandLevel, levelWiseSwitchCount[hpcIslandLevel]),
			}
			nodes[hpcIslandId] = hpcIsland
			forest[hpcIslandId] = hpcIsland
		}
		hpcIsland.Vertices[networkBlockId] = networkBlock
	}

	if len(instanceToNodeMap) != 0 {
		klog.V(4).Infof("Adding nodes w/o topology: %v", instanceToNodeMap)
		metrics.SetMissingTopology(NAME, len(instanceToNodeMap))
		sw := &topology.Vertex{
			ID:       topology.NoTopology,
			Vertices: make(map[string]*topology.Vertex),
		}
		for instanceID, nodeName := range instanceToNodeMap {
			sw.Vertices[instanceID] = &topology.Vertex{
				Name: nodeName,
				ID:   instanceID,
			}
		}
		forest[topology.NoTopology] = sw
	}

	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	for name, node := range forest {
		treeRoot.Vertices[name] = node
	}

	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	root.Vertices[topology.TopologyTree] = treeRoot
	return root, nil

}

func filterAndSort(bareMetalHostSummaries []*core.ComputeBareMetalHostSummary, instanceToNodeMap map[string]string) []*core.ComputeBareMetalHostSummary {
	var filtered []*core.ComputeBareMetalHostSummary
	for _, bmh := range bareMetalHostSummaries {
		if bmh.InstanceId == nil {
			klog.V(5).Infof("Instance ID is nil for bmhSummary %s", bmh.String())
			continue
		}

		if bmh.ComputeLocalBlockId == nil {
			klog.Warningf("ComputeLocalBlockId is nil for instance %q", *bmh.InstanceId)
			missingAncestor.WithLabelValues("localBlock", *bmh.InstanceId).Add(float64(1))
			continue
		}

		if bmh.ComputeNetworkBlockId == nil {
			klog.Warningf("ComputeNetworkBlockId is nil for instance %q", *bmh.InstanceId)
			missingAncestor.WithLabelValues("networkBlock", *bmh.InstanceId).Add(float64(1))
			continue
		}

		if bmh.ComputeHpcIslandId == nil {
			klog.Warningf("ComputeHpcIslandId is nil for instance %q", *bmh.InstanceId)
			missingAncestor.WithLabelValues("hpcIsland", *bmh.InstanceId).Add(float64(1))
			continue
		}

		if _, ok := instanceToNodeMap[*bmh.InstanceId]; ok {
			klog.V(4).Infof("Adding bmhSummary %s", bmh.String())
			filtered = append(filtered, bmh)
		} else {
			klog.V(4).Infof("Skipping bmhSummary %s", bmh.String())
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].ComputeHpcIslandId != filtered[j].ComputeHpcIslandId {
			return *filtered[i].ComputeHpcIslandId < *filtered[j].ComputeHpcIslandId
		}

		if filtered[i].ComputeNetworkBlockId != filtered[j].ComputeNetworkBlockId {
			return *filtered[i].ComputeNetworkBlockId < *filtered[j].ComputeNetworkBlockId
		}

		if filtered[i].ComputeLocalBlockId != filtered[j].ComputeLocalBlockId {
			return *filtered[i].ComputeLocalBlockId < *filtered[j].ComputeLocalBlockId
		}

		return *filtered[i].InstanceId < *filtered[j].InstanceId
	})
	return filtered
}

func generateInstanceTopology(ctx context.Context, factory ClientFactory, ci *topology.ComputeInstances, bareMetalHostSummaries []*core.ComputeBareMetalHostSummary) ([]*core.ComputeBareMetalHostSummary, error) {
	client, err := factory(ci.Region)
	if err != nil {
		return nil, err
	}

	bmh, err := getBareMetalHostSummaries(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("unable to populate compute capacity topology: %s", err.Error())
	}

	for _, bm := range bmh {
		bareMetalHostSummaries = append(bareMetalHostSummaries, &bm)
	}
	return bareMetalHostSummaries, nil
}
