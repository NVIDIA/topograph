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

package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/agrea/ptr"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, fmt.Errorf("unable to get client: %v", err)
	}

	projectID, err := client.ProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get project ID: %v", err)
	}

	topo := topology.NewClusterTopology()

	maxRes := castPageSize(pageSize)
	for _, ci := range cis {
		p.generateRegionInstanceTopology(ctx, client, projectID, maxRes, topo, &ci)
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, client Client, projectID string, maxRes *uint32, topo *topology.ClusterTopology, ci *topology.ComputeInstances) {
	klog.InfoS("Getting instance topology", "region", ci.Region, "project", projectID)

	req := computepb.ListInstancesRequest{
		Project:    projectID,
		Zone:       ci.Region,
		MaxResults: maxRes,
		PageToken:  nil,
	}

	var cycle int
	for {
		cycle++
		klog.V(4).Infof("Starting cycle %d", cycle)
		instances, token := client.Instances(ctx, &req)
		for _, instance := range instances {
			instanceId := strconv.FormatUint(*instance.Id, 10)
			klog.V(4).Infof("Checking instance %s", instanceId)

			if _, ok := ci.Instances[instanceId]; ok {
				if instance.ResourceStatus == nil {
					klog.InfoS("ResourceStatus is not set", "instance", instanceId)
					resourceStatusNotFound.WithLabelValues(instanceId).Set(1)
					continue
				}
				resourceStatusNotFound.WithLabelValues(instanceId).Set(0)

				if instance.ResourceStatus.PhysicalHost == nil {
					klog.InfoS("PhysicalHost is not set", "instance", instanceId)
					physicalHostNotFound.WithLabelValues(instanceId).Set(1)
					continue
				}
				physicalHostNotFound.WithLabelValues(instanceId).Set(0)

				tokens := strings.Split(*instance.ResourceStatus.PhysicalHost, "/")
				physicalHostIDChunks.WithLabelValues(instanceId).Set(float64(getTokenCount(tokens)))
				inst := &topology.InstanceTopology{
					InstanceID: instanceId,
					SpineID:    tokens[1],
					BlockID:    tokens[2],
				}
				klog.InfoS("Topology", "instance", instanceId, "cluster", inst.SpineID, "rack", inst.BlockID)
				topo.Append(inst)
			}
		}

		if len(token) == 0 {
			klog.V(4).Infof("Total processed nodes: %d", topo.Len())
			return
		}
		req.PageToken = &token
	}
}

func getTokenCount(tokens []string) int {
	c := 0
	for _, q := range tokens {
		if len(q) > 0 {
			c += 1
		}
	}
	return c
}

func castPageSize(val *int) *uint32 {
	if val == nil {
		return nil
	}

	return ptr.Uint32(uint32(*val))
}
