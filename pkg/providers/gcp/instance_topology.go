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

	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type InstanceTopology struct {
	instances []*InstanceInfo
}

type InstanceInfo struct {
	id        string
	clusterID string
	rackID    string
}

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*InstanceTopology, error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, fmt.Errorf("unable to get client: %v", err)
	}

	projectID, err := client.ProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get project ID: %v", err)
	}

	insTop := &InstanceTopology{
		instances: []*InstanceInfo{},
	}

	maxRes := castPageSize(pageSize)
	for _, ci := range cis {
		p.generateRegionInstanceTopology(ctx, client, projectID, maxRes, insTop, &ci)
	}

	return insTop, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, client Client, projectID string, maxRes *uint32, insTop *InstanceTopology, ci *topology.ComputeInstances) {
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
				instanceObj := &InstanceInfo{
					id:        instanceId,
					clusterID: tokens[1],
					rackID:    tokens[2],
				}
				klog.InfoS("Topology", "instance", instanceId, "cluster", instanceObj.clusterID, "rack", instanceObj.rackID)
				insTop.instances = append(insTop.instances, instanceObj)
			}
		}

		if len(token) == 0 {
			klog.V(4).Infof("Total processed nodes: %d", len(insTop.instances))
			return
		}
		req.PageToken = &token
	}
}

func (cfg *InstanceTopology) toGraph(cis []topology.ComputeInstances) (*topology.Vertex, error) {
	i2n := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			i2n[instance] = node
		}
	}

	forest := make(map[string]*topology.Vertex)
	nodes := make(map[string]*topology.Vertex)

	for _, c := range cfg.instances {
		nodeName, ok := i2n[c.id]
		if !ok {
			continue
		}
		klog.V(4).Infof("Found node %q instance %q", nodeName, c.id)
		delete(i2n, c.id)

		instance := &topology.Vertex{
			Name: nodeName,
			ID:   nodeName,
		}

		id2 := c.rackID
		sw2, ok := nodes[id2]
		if !ok {
			sw2 = &topology.Vertex{
				ID:       id2,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id2] = sw2
		}
		sw2.Vertices[instance.ID] = instance

		id1 := c.clusterID
		sw1, ok := nodes[id1]
		if !ok {
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
