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
	"sort"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

type level int

const (
	localBlockLevel level = iota + 1
	networkBlockLevel
	hpcIslandLevel
)

func GenerateInstanceTopology(ctx context.Context, factory ClientFactory, pageSize *int, cis []topology.ComputeInstances) (hosts []core.ComputeHostSummary, blockMap map[string]string, err error) {
	blockMap = make(map[string]string)

	for _, ci := range cis {
		var client Client
		if client, err = factory(ci.Region, pageSize); err != nil {
			return
		}
		if hosts, err = getComputeHostInfo(ctx, client, hosts, blockMap); err != nil {
			return
		}
	}

	return
}

func getComputeHostSummary(ctx context.Context, client Client, availabilityDomain *string) ([]core.ComputeHostSummary, error) {
	var hosts []core.ComputeHostSummary

	req := core.ListComputeHostsRequest{
		CompartmentId:      client.TenancyOCID(),
		AvailabilityDomain: availabilityDomain,
		Limit:              client.Limit(),
	}

	for {
		timeStart := time.Now()
		resp, err := client.ListComputeHosts(ctx, req)
		requestLatency.WithLabelValues("ListComputeHosts", resp.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())
		if err != nil {
			return nil, err
		}

		hosts = append(hosts, resp.Items...)

		if resp.OpcNextPage != nil {
			req.Page = resp.OpcNextPage
		} else {
			break
		}
	}

	return hosts, nil
}

// getLocalBlockMap returns a map between LocalBlocks and ComputeGpuMemoryFabrics
func getLocalBlockMap(ctx context.Context, client Client, availabilityDomain *string, blockMap map[string]string) error {
	req := core.ListComputeGpuMemoryFabricsRequest{
		CompartmentId:      client.TenancyOCID(),
		AvailabilityDomain: availabilityDomain,
		Limit:              client.Limit(),
	}

	for {
		timeStart := time.Now()
		resp, err := client.ListComputeGpuMemoryFabrics(ctx, req)
		requestLatency.WithLabelValues("ListComputeGpuMemoryFabrics", resp.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())
		if err != nil {
			return err
		}

		for _, fabrics := range resp.Items {
			blockMap[*fabrics.ComputeLocalBlockId] = *fabrics.Id
		}

		if resp.OpcNextPage != nil {
			req.Page = resp.OpcNextPage
		} else {
			break
		}
	}

	return nil
}

func getComputeHostInfo(ctx context.Context, client Client, hosts []core.ComputeHostSummary, blockMap map[string]string) ([]core.ComputeHostSummary, error) {
	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId: client.TenancyOCID(),
	}

	timeStart := time.Now()
	resp, err := client.ListAvailabilityDomains(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("unable to get availability domains: %v", err)
	}
	requestLatency.WithLabelValues("ListAvailabilityDomains", resp.HTTPResponse().Status).Observe(time.Since(timeStart).Seconds())

	for _, ad := range resp.Items {
		summary, err := getComputeHostSummary(ctx, client, ad.Name)
		if err != nil {
			return nil, fmt.Errorf("unable to get hosts info: %v", err)
		}
		hosts = append(hosts, summary...)

		if err = getLocalBlockMap(ctx, client, ad.Name, blockMap); err != nil {
			return nil, fmt.Errorf("unable to get local block map: %v", err)
		}
	}

	klog.V(4).Infof("Returning host info for %d nodes and %d blocks", len(hosts), len(blockMap))

	return hosts, nil
}

func toGraph(hosts []core.ComputeHostSummary, blockMap map[string]string, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	instanceToNodeMap := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			instanceToNodeMap[instance] = node
		}
	}
	klog.V(4).Infof("Instance/Node map %v", instanceToNodeMap)

	nodes := make(map[string]*topology.Vertex)
	forest := make(map[string]*topology.Vertex)
	domainMap := translate.NewDomainMap()

	levelWiseSwitchCount := map[level]int{localBlockLevel: 0, networkBlockLevel: 0, hpcIslandLevel: 0}
	hosts = filterAndSort(hosts, instanceToNodeMap)
	for _, host := range hosts {
		nodeName := instanceToNodeMap[*host.Id]
		delete(instanceToNodeMap, *host.Id)

		instance := &topology.Vertex{
			Name: nodeName,
			ID:   *host.Id,
		}

		localBlockId := *host.LocalBlockId

		if blockDomain, ok := blockMap[localBlockId]; ok {
			domainMap.AddHost(blockDomain, nodeName)
		}

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

		networkBlockId := *host.NetworkBlockId
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

		hpcIslandId := *host.HpcIslandId
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
	if len(domainMap) != 0 {
		root.Vertices[topology.TopologyBlock] = domainMap.ToBlocks()
	}
	return root, nil

}

func filterAndSort(hosts []core.ComputeHostSummary, instanceToNodeMap map[string]string) []core.ComputeHostSummary {
	var filtered []core.ComputeHostSummary
	for _, host := range hosts {
		if host.Id == nil {
			klog.Warningf("InstanceID is nil for host %s", host.String())
			continue
		}

		if host.LocalBlockId == nil {
			klog.Warningf("LocalBlockId is nil for instance %q", *host.Id)
			missingAncestor.WithLabelValues("LocalBlock", *host.Id).Add(float64(1))
			continue
		}

		if host.NetworkBlockId == nil {
			klog.Warningf("NetworkBlockId is nil for instance %q", *host.Id)
			missingAncestor.WithLabelValues("networkBlock", *host.Id).Add(float64(1))
			continue
		}

		if host.HpcIslandId == nil {
			klog.Warningf("HpcIslandId is nil for instance %q", *host.Id)
			missingAncestor.WithLabelValues("hpcIsland", *host.Id).Add(float64(1))
			continue
		}

		if _, ok := instanceToNodeMap[*host.Id]; ok {
			klog.V(4).Infof("Adding host %s", host.String())
			filtered = append(filtered, host)
		} else {
			klog.V(4).Infof("Skipping host %s", host.String())
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].HpcIslandId != filtered[j].HpcIslandId {
			return *filtered[i].HpcIslandId < *filtered[j].HpcIslandId
		}

		if filtered[i].NetworkBlockId != filtered[j].NetworkBlockId {
			return *filtered[i].NetworkBlockId < *filtered[j].NetworkBlockId
		}

		if filtered[i].LocalBlockId != filtered[j].LocalBlockId {
			return *filtered[i].LocalBlockId < *filtered[j].LocalBlockId
		}

		return *filtered[i].Id < *filtered[j].Id
	})
	return filtered
}
