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
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func getComputeHostSummary(ctx context.Context, client Client, availabilityDomain *string, topo *topology.ClusterTopology, instMap map[string]string) error {
	req := core.ListComputeHostsRequest{
		CompartmentId:      client.TenantID(),
		AvailabilityDomain: availabilityDomain,
		Limit:              client.Limit(),
	}

	for {
		klog.V(4).InfoS("ListComputeHosts", "request", req.String())
		start := time.Now()
		resp, err := client.ListComputeHosts(ctx, req)
		reportLatency(resp.HTTPResponse(), start, "ListComputeHosts")
		if err != nil {
			return err
		}

		for _, host := range resp.Items {
			inst, err := convert(&host)
			if err != nil {
				klog.Warning(err.Error())
				continue
			}

			if _, ok := instMap[inst.InstanceID]; ok {
				klog.V(4).Infof("Adding host %s", host.String())
				topo.Append(inst)
			} else {
				klog.V(4).Infof("Skipping host %s", host.String())
			}
		}

		if resp.OpcNextPage == nil {
			return nil
		}
		req.Page = resp.OpcNextPage
	}
}

func convert(host *core.ComputeHostSummary) (*topology.InstanceTopology, error) {
	if host.InstanceId == nil {
		return nil, fmt.Errorf("missing InstanceId in ComputeHostSummary")
	}

	if host.LocalBlockId == nil {
		missingHostData.WithLabelValues("localBlock", *host.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("missing LocalBlockId for instance %q", *host.InstanceId)
	}

	if host.NetworkBlockId == nil {
		missingHostData.WithLabelValues("networkBlock", *host.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("missing NetworkBlockId for instance %q", *host.InstanceId)
	}

	if host.HpcIslandId == nil {
		missingHostData.WithLabelValues("hpcIsland", *host.InstanceId).Add(float64(1))
		return nil, fmt.Errorf("missing HpcIslandId for instance %q", *host.InstanceId)
	}

	topo := &topology.InstanceTopology{
		InstanceID:   *host.InstanceId,
		BlockID:      *host.LocalBlockId,
		SpineID:      *host.NetworkBlockId,
		DatacenterID: *host.HpcIslandId,
	}

	if host.GpuMemoryFabricId != nil {
		topo.AcceleratorID = *host.GpuMemoryFabricId
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
