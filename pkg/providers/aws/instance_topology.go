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
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
)

var defaultPageSize int32 = 100

func (p *Provider) generateInstanceTopology(ctx context.Context, pageSize int32, cis []topology.ComputeInstances) ([]types.InstanceTopology, error) {
	var err error
	topology := []types.InstanceTopology{}
	for _, ci := range cis {
		if topology, err = p.generateInstanceTopologyForRegionInstances(ctx, pageSize, &ci, topology); err != nil {
			return nil, err
		}
	}

	return topology, nil
}

func (p *Provider) generateInstanceTopologyForRegionInstances(ctx context.Context, pageSize int32, ci *topology.ComputeInstances, topology []types.InstanceTopology) ([]types.InstanceTopology, error) {
	if len(ci.Region) == 0 {
		return nil, fmt.Errorf("must specify region to query instance topology")
	}
	klog.Infof("Getting instance topology for %s region", ci.Region)

	client, err := p.clientFactory(ci.Region)
	if err != nil {
		return nil, err
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
		if pageSize == 0 {
			pageSize = defaultPageSize
		}
		klog.Infof("Getting instance topology with page size %d", pageSize)
		input.MaxResults = &pageSize
	}

	var cycle, total int
	for {
		cycle++
		klog.V(4).Infof("Starting cycle %d", cycle)
		start := time.Now()
		output, err := client.EC2.DescribeInstanceTopology(ctx, input)
		if err != nil {
			apiLatency.WithLabelValues(ci.Region, "Error").Observe(time.Since(start).Seconds())
			return nil, fmt.Errorf("failed to describe instance topology: %v", err)
		}
		apiLatency.WithLabelValues(ci.Region, "Success").Observe(time.Since(start).Seconds())
		total += len(output.Instances)
		for _, elem := range output.Instances {
			if _, ok := ci.Instances[*elem.InstanceId]; ok {
				topology = append(topology, elem)
			}
		}
		klog.V(4).Infof("Received instance topology for %d nodes; processed %d; selected %d", len(output.Instances), total, len(topology))

		if output.NextToken == nil {
			break
		} else {
			input.NextToken = output.NextToken
		}
	}

	klog.Infof("Returning instance topology for %d nodes", len(topology))
	return topology, nil
}

func toGraph(top []types.InstanceTopology, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	i2n := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			i2n[instance] = node
		}
	}
	klog.V(4).Infof("Instance/Node map %v", i2n)

	forest := make(map[string]*topology.Vertex)
	nodes := make(map[string]*topology.Vertex)

	for _, inst := range top {
		//klog.V(4).Infof("Checking instance %q", c.InstanceId)
		nodeName, ok := i2n[*inst.InstanceId]
		if !ok {
			continue
		}
		klog.V(4).Infof("Found node %q instance %q", nodeName, *inst.InstanceId)
		delete(i2n, *inst.InstanceId)

		instance := &topology.Vertex{
			Name: nodeName,
			ID:   *inst.InstanceId,
		}
		// process level 3 node
		id3 := inst.NetworkNodes[2]
		sw3, ok := nodes[id3]
		if !ok { //
			sw3 = &topology.Vertex{
				ID:       id3,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id3] = sw3
		}
		sw3.Vertices[instance.ID] = instance

		// process level 2 node
		id2 := inst.NetworkNodes[1]
		sw2, ok := nodes[id2]
		if !ok { //
			sw2 = &topology.Vertex{
				ID:       id2,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id2] = sw2
		}
		sw2.Vertices[id3] = sw3

		// process level 1 node
		id1 := inst.NetworkNodes[0]
		sw1, ok := nodes[id1]
		if !ok { //
			sw1 = &topology.Vertex{
				ID:       id1,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id1] = sw1
			forest[id1] = sw1
		}
		sw1.Vertices[id2] = sw2
	}

	if len(i2n) != 0 {
		klog.V(4).Infof("Adding nodes w/o topology: %v", i2n)
		metrics.SetMissingTopology(NAME, len(i2n))
		sw := &topology.Vertex{
			ID:       topology.NoTopology,
			Vertices: make(map[string]*topology.Vertex),
		}
		for instanceID, nodeName := range i2n {
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
