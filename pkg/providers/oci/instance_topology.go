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

func GenerateInstanceTopology(ctx context.Context, factory ClientFactory, cis []topology.ComputeInstances) (*topology.ClusterTopology, error) {
	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := generateInstanceTopology(ctx, factory, &ci, topo); err != nil {
			return nil, err
		}
	}

	return topo, nil
}

func generateInstanceTopology(ctx context.Context, factory ClientFactory, ci *topology.ComputeInstances, topo *topology.ClusterTopology) error {
	client, err := factory(ci.Region)
	if err != nil {
		return err
	}

	if err := getBareMetalHostSummaries(ctx, client, topo, ci.Instances); err != nil {
		return fmt.Errorf("unable to populate compute capacity topology: %v", err)
	}

	return nil
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

func getBMHSummaryPerComputeCapacityTopology(ctx context.Context, client Client, topologyID string, topo *topology.ClusterTopology, instanceMap map[string]string) error {
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

		for _, bmh := range response.Items {
			inst, err := convert(&bmh)
			if err != nil {
				klog.Warning(err.Error())
				continue
			}

			if _, ok := instanceMap[*bmh.InstanceId]; ok {
				klog.V(4).Infof("Adding host topology %s", bmh.String())
				topo.Append(inst)
			} else {
				klog.V(4).Infof("Skipping bmhSummary %s", bmh.String())
			}
		}

		if response.OpcNextPage != nil {
			request.Page = response.OpcNextPage
		} else {
			break
		}
	}

	return nil
}

func getBareMetalHostSummaries(ctx context.Context, client Client, topo *topology.ClusterTopology, instanceMap map[string]string) error {
	computeCapacityTopology, err := getComputeCapacityTopologies(ctx, client)
	if err != nil {
		return fmt.Errorf("unable to get compute capacity topologies: %s", err.Error())
	}
	klog.V(4).Infof("Received computeCapacityTopology for %d groups", len(computeCapacityTopology))

	for _, cct := range computeCapacityTopology {
		if err := getBMHSummaryPerComputeCapacityTopology(ctx, client, *cct.Id, topo, instanceMap); err != nil {
			return fmt.Errorf("unable to get bare metal hosts info: %v", err)
		}
	}
	klog.V(4).Infof("Returning bareMetalHostSummaries for %d nodes", topo.Len())

	return nil
}

func convert(bmh *core.ComputeBareMetalHostSummary) (*topology.InstanceTopology, error) {
	if bmh.InstanceId == nil {
		return nil, fmt.Errorf("Instance ID is nil for bmhSummary %s", bmh.String())
	}

	if bmh.ComputeLocalBlockId == nil {
		missingAncestor.WithLabelValues("localBlock", *bmh.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("ComputeLocalBlockId is nil for instance %q", *bmh.InstanceId)
	}

	if bmh.ComputeNetworkBlockId == nil {
		missingAncestor.WithLabelValues("networkBlock", *bmh.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("ComputeNetworkBlockId is nil for instance %q", *bmh.InstanceId)
	}

	if bmh.ComputeHpcIslandId == nil {
		missingAncestor.WithLabelValues("hpcIsland", *bmh.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("ComputeHpcIslandId is nil for instance %q", *bmh.InstanceId)
	}

	topo := &topology.InstanceTopology{
		InstanceID:   *bmh.InstanceId,
		BlockID:      *bmh.ComputeLocalBlockId,
		SpineID:      *bmh.ComputeNetworkBlockId,
		DatacenterID: *bmh.ComputeHpcIslandId,
	}

	return topo, nil
}
