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

package topology

import (
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/metrics"
)

type ClusterTopology struct {
	Instances []*InstanceTopology
}

type InstanceTopology struct {
	InstanceID      string
	AcceleratorID   string
	AcceleratorName string // optional
	BlockID         string
	BlockName       string // optional
	SpineID         string
	SpineName       string // optional
	DatacenterID    string
	DatacenterName  string // optional
}

func NewClusterTopology() *ClusterTopology {
	return &ClusterTopology{Instances: []*InstanceTopology{}}
}

func (c *ClusterTopology) Append(inst *InstanceTopology) {
	c.Instances = append(c.Instances, inst)
}

func (c *ClusterTopology) Len() int {
	return len(c.Instances)
}

func (c *ClusterTopology) ToThreeTierGraph(provider string, cis []ComputeInstances) (*Vertex, error) {
	i2n := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			i2n[instance] = node
		}
	}

	forest := make(map[string]*Vertex)
	nodes := make(map[string]*Vertex)
	domainMap := NewDomainMap()

	for _, inst := range c.Instances {
		nodeName, ok := i2n[inst.InstanceID]
		if !ok {
			continue
		}

		klog.V(4).Infof("Found node %q instance %q", nodeName, inst.InstanceID)
		delete(i2n, inst.InstanceID)

		instance := &Vertex{
			Name: nodeName,
			ID:   inst.InstanceID,
		}

		if len(inst.AcceleratorID) != 0 {
			domainMap.AddHost(inst.AcceleratorID, nodeName)
		}

		swNames := [3]string{inst.BlockName, inst.SpineName, inst.DatacenterName}
		for i, swID := range []string{inst.BlockID, inst.SpineID, inst.DatacenterID} {
			if len(swID) == 0 {
				break
			}

			sw, ok := nodes[swID]
			if !ok {
				sw = &Vertex{
					ID:       swID,
					Name:     swNames[i],
					Vertices: make(map[string]*Vertex),
				}
				nodes[swID] = sw
			}
			sw.Vertices[instance.ID] = instance
			instance = sw
		}
		forest[instance.ID] = instance
	}

	if len(i2n) != 0 {
		klog.V(4).Infof("Adding nodes w/o topology: %v", i2n)

		sw := &Vertex{
			ID:       NoTopology,
			Vertices: make(map[string]*Vertex),
		}
		for instanceID, nodeName := range i2n {
			sw.Vertices[instanceID] = &Vertex{
				Name: nodeName,
				ID:   instanceID,
			}
			metrics.SetMissingTopology(provider, nodeName)
		}
		forest[NoTopology] = sw
	}

	treeRoot := &Vertex{
		Vertices: make(map[string]*Vertex),
	}
	for name, node := range forest {
		treeRoot.Vertices[name] = node
	}

	root := &Vertex{
		Vertices: make(map[string]*Vertex),
	}
	root.Vertices[TopologyTree] = treeRoot

	if len(domainMap) != 0 {
		root.Vertices[TopologyBlock] = domainMap.ToBlocks()
	}

	return root, nil
}
