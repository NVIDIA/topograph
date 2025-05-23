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

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/agrea/ptr"
	"google.golang.org/api/iterator"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, error) {
	client, err := p.clientFactory(pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %v", err)
	}

	projectID, err := client.ProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get project ID: %v", err)
	}

	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := p.generateRegionInstanceTopology(ctx, client, projectID, topo, &ci); err != nil {
			return nil, fmt.Errorf("failed to get instance topology: %v", err)
		}
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, client Client, projectID string, topo *topology.ClusterTopology, ci *topology.ComputeInstances) error {
	if len(ci.Region) == 0 {
		return fmt.Errorf("must specify region")
	}
	klog.InfoS("Getting instance topology", "region", ci.Region, "project", projectID)

	req := computepb.ListInstancesRequest{
		Project:    projectID,
		Zone:       ci.Region,
		MaxResults: client.PageSize(),
	}

	for {
		klog.V(4).InfoS("ListInstances", "request", req.String())
		iter, token := client.Instances(ctx, &req)
		for {
			instance, err := iter.Next()
			if err != nil {
				if err == iterator.Done {
					break
				} else {
					return err
				}
			}
			instanceId := strconv.FormatUint(*instance.Id, 10)
			klog.V(4).Infof("Checking instance %s", instanceId)

			if _, ok := ci.Instances[instanceId]; ok {
				if instance.ResourceStatus == nil {
					klog.InfoS("ResourceStatus is not set", "instance", instanceId)
					missingResourceStatus.WithLabelValues(instanceId).Inc()
					continue
				}

				if instance.ResourceStatus.PhysicalHostTopology == nil {
					klog.InfoS("PhysicalHostTopology is not set", "instance", instanceId)
					missingPhysicalHostTopology.WithLabelValues(instanceId).Inc()
					continue
				}

				if instance.ResourceStatus.PhysicalHostTopology.Cluster == nil ||
					instance.ResourceStatus.PhysicalHostTopology.Block == nil ||
					instance.ResourceStatus.PhysicalHostTopology.Subblock == nil {
					klog.InfoS("Missing topology info", "instance", instanceId)
					missingTopologyInfo.WithLabelValues(instanceId).Inc()
					continue
				}
				inst := &topology.InstanceTopology{
					InstanceID:   instanceId,
					DatacenterID: instance.ResourceStatus.PhysicalHostTopology.GetCluster(),
					SpineID:      instance.ResourceStatus.PhysicalHostTopology.GetBlock(),
					BlockID:      instance.ResourceStatus.PhysicalHostTopology.GetSubblock(),
				}
				inst.AcceleratorID = inst.BlockID
				klog.Infof("Adding topology: %s", inst.String())
				topo.Append(inst)
			}
		}

		if len(token) == 0 {
			klog.V(4).Infof("Total processed nodes: %d", topo.Len())
			return nil
		}
		req.PageToken = &token
	}
}

func castPageSize(val *int) *uint32 {
	if val == nil {
		return nil
	}

	return ptr.Uint32(uint32(*val))
}
