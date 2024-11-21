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

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func GenerateInstanceTopology(ctx context.Context, factory ClientFactory, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, error) {
	topo := topology.NewClusterTopology()
	blockMap := make(map[string]string)

	for _, ci := range cis {
		client, err := factory(ci.Region, pageSize)
		if err != nil {
			return nil, err
		}
		err = getComputeHostInfo(ctx, client, topo, ci.Instances, blockMap)
		if err != nil {
			return nil, err
		}
	}

	// set accelerator block domains
	for i, inst := range topo.Instances {
		if domain, ok := blockMap[inst.BlockID]; ok {
			topo.Instances[i].AcceleratorID = domain
		}
	}

	return topo, nil
}

func getComputeHostInfo(ctx context.Context, client Client, topo *topology.ClusterTopology, instMap, blockMap map[string]string) error {
	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId: &bcmnonprod_gb200_compartment_ocid,
		//CompartmentId: client.TenancyOCID(),
	}

	// TODO: remove print
	fmt.Printf("CALL ListAvailabilityDomains. CompartmentId=%s\n", *req.CompartmentId)
	start := time.Now()
	resp, err := client.ListAvailabilityDomains(ctx, req)
	reportLatency(resp.HTTPResponse(), start, "ListAvailabilityDomains")
	if err != nil {
		return fmt.Errorf("unable to get availability domains: %v", err)
	}

	// TODO: remove print loop
	for _, item := range resp.Items {
		fmt.Printf("AVAILABILITY DOMAIN %s\n", *item.Name)
	}

	for _, ad := range resp.Items {
		err := getComputeHostSummary(ctx, client, ad.Name, topo, instMap)
		if err != nil {
			return fmt.Errorf("unable to get hosts info: %v", err)
		}

		if err = getLocalBlockMap(ctx, client, ad.Name, blockMap); err != nil {
			return fmt.Errorf("unable to get local block map: %v", err)
		}
	}

	klog.V(4).Infof("Returning host info for %d nodes and %d blocks", topo.Len(), len(blockMap))

	return nil
}

func getComputeHostSummary(ctx context.Context, client Client, availabilityDomain *string, topo *topology.ClusterTopology, instMap map[string]string) error {
	// TODO: identify CompartmentId at runtime
	req := core.ListComputeHostsRequest{
		CompartmentId: &bcmnonprod_gb200_compartment_ocid,
		//CompartmentId:      client.TenancyOCID(),
		AvailabilityDomain: availabilityDomain,
		Limit:              client.Limit(),
	}

	for {
		// TODO: remove print
		fmt.Printf("CALL ListComputeHosts. CompartmentId=%s AD=%s\n", *req.CompartmentId, *availabilityDomain)
		start := time.Now()
		resp, err := client.ListComputeHosts(ctx, req)
		reportLatency(resp.HTTPResponse(), start, "ListComputeHosts")
		if err != nil {
			return err
		}
		// TODO: remove print
		fmt.Printf("RES ListComputeHosts: status=%s hosts=%d\n", resp.HTTPResponse().Status, len(resp.Items))
		for _, host := range resp.Items {
			// TODO: remove print
			fmt.Printf("--> INSTANCE %s\n", *host.Id)

			inst, err := convert(&host)
			if err != nil {
				klog.Warning(err.Error())
				continue
			}

			if _, ok := instMap[*host.Id]; ok {
				klog.V(4).Infof("Adding host %s", host.String())
				topo.Append(inst)
			} else {
				klog.V(4).Infof("Skipping host %s", host.String())
			}
		}

		if resp.OpcNextPage != nil {
			req.Page = resp.OpcNextPage
		} else {
			return nil
		}
	}
}

// getLocalBlockMap returns a map between LocalBlocks and ComputeGpuMemoryFabrics
func getLocalBlockMap(ctx context.Context, client Client, availabilityDomain *string, blockMap map[string]string) error {
	req := core.ListComputeGpuMemoryFabricsRequest{
		CompartmentId: &bcmnonprod_gb200_compartment_ocid,
		//CompartmentId:      client.TenancyOCID(),
		AvailabilityDomain: availabilityDomain,
		Limit:              client.Limit(),
	}

	for {
		// TODO: remove print
		fmt.Printf("CALL ListComputeGpuMemoryFabricsRequest. CompartmentId=%s\n", *req.CompartmentId)
		start := time.Now()
		resp, err := client.ListComputeGpuMemoryFabrics(ctx, req)
		reportLatency(resp.HTTPResponse(), start, "ListComputeGpuMemoryFabrics")
		if err != nil {
			return err
		}

		for _, fabrics := range resp.Items {
			blockMap[*fabrics.ComputeLocalBlockId] = *fabrics.Id
			// TODO: remove print
			fmt.Printf("--> ComputeLocalBlockId=%s fabricsId=%s\n", *fabrics.ComputeLocalBlockId, *fabrics.Id)
		}

		if resp.OpcNextPage != nil {
			req.Page = resp.OpcNextPage
		} else {
			return nil
		}
	}
}

func convert(host *core.ComputeHostSummary) (*topology.InstanceTopology, error) {
	if host.Id == nil {
		return nil, fmt.Errorf("InstanceID is nil for host %s", host.String())
	}

	if host.LocalBlockId == nil {
		missingHostData.WithLabelValues("LocalBlock", *host.Id).Add(float64(1))
		return nil, fmt.Errorf("LocalBlockId is nil for instance %q", *host.Id)
	}

	if host.NetworkBlockId == nil {
		missingHostData.WithLabelValues("networkBlock", *host.Id).Add(float64(1))
		return nil, fmt.Errorf("NetworkBlockId is nil for instance %q", *host.Id)
	}

	if host.HpcIslandId == nil {
		missingHostData.WithLabelValues("hpcIsland", *host.Id).Add(float64(1))
		return nil, fmt.Errorf("HpcIslandId is nil for instance %q", *host.Id)
	}

	topo := &topology.InstanceTopology{
		InstanceID:   *host.Id,
		BlockID:      *host.LocalBlockId,
		SpineID:      *host.NetworkBlockId,
		DatacenterID: *host.HpcIslandId,
	}

	return topo, nil
}

func reportLatency(resp *http.Response, since time.Time, method string) {
	duration := time.Since(since).Seconds()
	if resp != nil {
		requestLatency.WithLabelValues(method, resp.Status).Observe(duration)
	} else {
		requestLatency.WithLabelValues(method, "Fatal").Observe(duration)
	}
}
