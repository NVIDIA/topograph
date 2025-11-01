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

package aws

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

var defaultPageSize int32 = 100

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, *httperr.Error) {
	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := p.generateRegionInstanceTopology(ctx, pageSize, &ci, topo); err != nil {
			return nil, err
		}
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, pageSize *int, ci *topology.ComputeInstances, topo *topology.ClusterTopology) *httperr.Error {
	if len(ci.Region) == 0 {
		return httperr.NewError(http.StatusBadRequest, "must specify region")
	}
	klog.Infof("Getting instance topology for %s region", ci.Region)

	client, err := p.clientFactory(ci.Region, pageSize)
	if err != nil {
		return httperr.NewError(http.StatusBadGateway,
			fmt.Sprintf("failed to get client: %v", err))
	}
	input := &ec2.DescribeInstanceTopologyInput{}

	// AWS allows up to 100 explicitly specified instance IDs
	if n := len(ci.Instances); n <= 100 {
		klog.Infof("Getting instance topology for %d instances", n)
		input.InstanceIds = make([]string, 0, n)
		for instanceID := range ci.Instances {
			input.InstanceIds = append(input.InstanceIds, instanceID)
		}
	} else {
		klog.Infof("Getting instance topology with page size %d", pageSize)
		input.MaxResults = client.PageSize()
	}

	var cycle, total int
	for {
		cycle++
		klog.V(4).Infof("Starting cycle %d", cycle)
		start := time.Now()
		output, err := client.ec2.DescribeInstanceTopology(ctx, input)
		duration := time.Since(start).Seconds()
		if err != nil {
			apiLatency.WithLabelValues(ci.Region, "Error").Observe(duration)
			return httperr.NewError(http.StatusBadGateway,
				fmt.Sprintf("failed to describe instance topology: %v", err))
		}
		apiLatency.WithLabelValues(ci.Region, "Success").Observe(duration)
		total += len(output.Instances)
		for _, elem := range output.Instances {
			if _, ok := ci.Instances[*elem.InstanceId]; ok {
				topo.Append(convert(&elem))
			}
		}
		klog.V(4).Infof("Received instance topology for %d nodes; processed %d; selected %d", len(output.Instances), total, topo.Len())

		if output.NextToken == nil {
			break
		} else {
			input.NextToken = output.NextToken
		}
	}

	return nil
}

func convert(inst *types.InstanceTopology) *topology.InstanceTopology {
	topo := &topology.InstanceTopology{
		InstanceID:   *inst.InstanceId,
		BlockID:      inst.NetworkNodes[2],
		SpineID:      inst.NetworkNodes[1],
		DatacenterID: inst.NetworkNodes[0],
	}
	if inst.CapacityBlockId != nil {
		topo.AcceleratorID = *inst.CapacityBlockId
	}
	klog.V(4).Infof("Adding instance topology %s", topo.String())
	return topo
}

func setPageSize(val *int) int32 {
	if val == nil {
		return defaultPageSize
	}

	return int32(*val)
}
