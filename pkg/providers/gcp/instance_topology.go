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

	"cloud.google.com/go/compute/apiv2alpha/computepb"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

type InstanceTopology struct {
	instances []*InstanceInfo
}

type InstanceInfo struct {
	clusterID  string
	blockID    string
	subBlockID string
	name       string
}

func (p *baseProvider) generateInstanceTopology(ctx context.Context, instanceToNodeMap map[string]string) (*InstanceTopology, error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, err
	}

	projectID, err := client.ProjectID(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get project ID: %s", err.Error())
	}

	listZoneRequest := computepb.ListZonesRequest{Project: projectID}
	zones := client.Zones(ctx, &listZoneRequest)

	instanceTopology := &InstanceTopology{instances: make([]*InstanceInfo, 0)}

	for _, zone := range zones {
		listInstanceRequest := computepb.ListInstancesRequest{Project: projectID, Zone: zone}
		instances := client.Instances(ctx, &listInstanceRequest)

		for _, instance := range instances {
			_, isNodeInCluster := instanceToNodeMap[*instance.Name]

			if instance.ResourceStatus == nil {
				missingResourceStatus.WithLabelValues(*instance.Name).Inc()
				continue
			}

			if instance.ResourceStatus.PhysicalHostTopology == nil {
				missingPhysicalHostTopology.WithLabelValues(*instance.Name).Inc()
				continue
			}

			// TODO: manage orphan inctances
			if isNodeInCluster {
				if instance.ResourceStatus.PhysicalHostTopology.Cluster == nil ||
					instance.ResourceStatus.PhysicalHostTopology.Block == nil ||
					instance.ResourceStatus.PhysicalHostTopology.Subblock == nil {
					missingTopologyInfo.WithLabelValues(*instance.Name).Inc()
				} else {
					instanceObj := &InstanceInfo{
						name:       *instance.Name,
						clusterID:  instance.ResourceStatus.PhysicalHostTopology.GetCluster(),
						blockID:    instance.ResourceStatus.PhysicalHostTopology.GetBlock(),
						subBlockID: instance.ResourceStatus.PhysicalHostTopology.GetSubblock(),
					}
					instanceTopology.instances = append(instanceTopology.instances, instanceObj)
				}
			}
		}
	}

	return instanceTopology, nil
}

func (cfg *InstanceTopology) toGraph() (*topology.Vertex, error) {
	forest := make(map[string]*topology.Vertex)
	nodes := make(map[string]*topology.Vertex)
	domainMap := translate.NewDomainMap()

	for _, c := range cfg.instances {
		instance := &topology.Vertex{
			Name: c.name,
			ID:   c.name,
		}

		domainMap.AddHost(c.subBlockID, c.name)

		id1 := c.subBlockID
		sw1, ok := nodes[id1]
		if !ok {
			sw1 = &topology.Vertex{
				ID:       id1,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id1] = sw1
		}
		sw1.Vertices[instance.ID] = instance

		id2 := c.blockID
		sw2, ok := nodes[id2]
		if !ok {
			sw2 = &topology.Vertex{
				ID:       id2,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id2] = sw2
		}
		sw2.Vertices[instance.ID] = sw1

		id3 := c.clusterID
		sw3, ok := nodes[id3]
		if !ok {
			sw3 = &topology.Vertex{
				ID:       id3,
				Vertices: make(map[string]*topology.Vertex),
			}
			nodes[id3] = sw3
			forest[id3] = sw3
		}
		sw3.Vertices[id2] = sw2
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

func getTokenCount(tokens []string) int {
	c := 0
	for _, q := range tokens {
		if len(q) > 0 {
			c += 1
		}
	}
	return c
}
