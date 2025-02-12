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
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/compute/metadata"
	"github.com/agrea/ptr"
	"google.golang.org/api/iterator"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

type InstanceTopology struct {
	instances []*InstanceInfo
}

type InstanceInfo struct {
	clusterID string
	rackID    string
	name      string
}

func (p *Provider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*InstanceTopology, error) {
	insTop := &InstanceTopology{
		instances: []*InstanceInfo{},
	}

	maxRes := castPageSize(pageSize)
	for _, ci := range cis {
		err := p.generateRegionInstanceTopology(ctx, insTop, maxRes, &ci)
		if err != nil {
			return nil, err
		}
	}

	return insTop, nil
}

func (p *Provider) generateRegionInstanceTopology(ctx context.Context, insTop *InstanceTopology, maxRes *uint32, ci *topology.ComputeInstances) error {
	client, err := p.clientFactory()
	if err != nil {
		return fmt.Errorf("unable to get client: %v", err)
	}

	projectID, err := metadata.ProjectIDWithContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to get project ID: %v", err)
	}

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

		timeNow := time.Now()
		resp := client.Instances.List(ctx, &req)
		requestLatency.WithLabelValues("ListInstances").Observe(time.Since(timeNow).Seconds())

		processInstanceList(insTop, resp, ci)

		klog.V(4).Infof("Processed %d nodes", len(insTop.instances))

		if token := resp.PageInfo().Token; token == "" {
			break
		} else {
			req.PageToken = &token
		}
	}

	return nil
}

func (cfg *InstanceTopology) toGraph() (*topology.Vertex, error) {
	forest := make(map[string]*topology.Vertex)
	nodes := make(map[string]*topology.Vertex)

	for _, c := range cfg.instances {
		instance := &topology.Vertex{
			Name: c.name,
			ID:   c.name,
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

func processInstanceList(insTop *InstanceTopology, resp *compute.InstanceIterator, ci *topology.ComputeInstances) {
	for {
		instance, err := resp.Next()
		if err == iterator.Done {
			return
		}
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
				name:      instanceId,
				clusterID: tokens[1],
				rackID:    tokens[2],
			}
			klog.InfoS("Topology", "instance", instanceId, "cluster", instanceObj.clusterID, "rack", instanceObj.rackID)
			insTop.instances = append(insTop.instances, instanceObj)
		}
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
