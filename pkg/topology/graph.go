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
	"fmt"
	"maps"
	"sort"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/metrics"
)

type band int

const (
	blockBand band = iota + 1
	spineBand
	datacenterBand
)

type ClusterTopology struct {
	Instances []*InstanceTopology
}

type InstanceTopology struct {
	InstanceID     string
	BlockID        string
	BlockName      string // optional
	SpineID        string
	SpineName      string // optional
	DatacenterID   string
	DatacenterName string // optional
	AcceleratorID  string
}

func (inst *InstanceTopology) String() string {
	var buf strings.Builder
	buf.WriteString("Instance:" + inst.InstanceID)
	if len(inst.BlockID) != 0 {
		buf.WriteString(" Block:" + inst.BlockID)
		if len(inst.BlockName) != 0 {
			buf.WriteString(" (" + inst.BlockName + ")")
		}
	}
	if len(inst.SpineID) != 0 {
		buf.WriteString(" Spine:" + inst.SpineID)
		if len(inst.SpineName) != 0 {
			buf.WriteString(" (" + inst.SpineName + ")")
		}
	}
	if len(inst.DatacenterID) != 0 {
		buf.WriteString(" Datacenter:" + inst.DatacenterID)
		if len(inst.DatacenterName) != 0 {
			buf.WriteString(" (" + inst.DatacenterName + ")")
		}
	}
	if len(inst.AcceleratorID) != 0 {
		buf.WriteString(" Accelerator:" + inst.AcceleratorID)
	}

	return buf.String()
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

func (c *ClusterTopology) ToThreeTierGraph(provider string, cis []ComputeInstances, normalize bool) (*Vertex, error) {
	i2n := make(map[string]string)
	for _, ci := range cis {
		maps.Copy(i2n, ci.Instances)
	}

	forest := make(map[string]*Vertex)
	nodes := make(map[string]*Vertex)
	domainMap := NewDomainMap()

	if normalize {
		c.Normalize()
	}

	for _, inst := range c.Instances {
		nodeName, ok := i2n[inst.InstanceID]
		if !ok {
			continue
		}

		klog.V(4).InfoS("Found", "node", nodeName, "instance", inst.InstanceID)
		delete(i2n, inst.InstanceID)

		instance := &Vertex{
			Name: nodeName,
			ID:   inst.InstanceID,
		}

		if len(inst.AcceleratorID) != 0 {
			domainMap.AddHost(inst.AcceleratorID, inst.InstanceID, nodeName)
		}

		swNames := [3]string{inst.BlockName, inst.SpineName, inst.DatacenterName}
		for i, swID := range []string{inst.BlockID, inst.SpineID, inst.DatacenterID} {
			if len(swID) == 0 {
				continue
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
	maps.Copy(treeRoot.Vertices, forest)

	root := &Vertex{
		Vertices: make(map[string]*Vertex),
	}
	root.Vertices[TopologyTree] = treeRoot

	if len(domainMap) != 0 {
		root.Vertices[TopologyBlock] = domainMap.ToBlocks()
	}

	return root, nil
}

func (c *ClusterTopology) Normalize() {
	// sort by network hierarchy
	sort.Slice(c.Instances, func(i, j int) bool {
		if c.Instances[i].DatacenterID != c.Instances[j].DatacenterID {
			return c.Instances[i].DatacenterID < c.Instances[j].DatacenterID
		}

		if c.Instances[i].SpineID != c.Instances[j].SpineID {
			return c.Instances[i].SpineID < c.Instances[j].SpineID
		}

		if c.Instances[i].BlockID != c.Instances[j].BlockID {
			return c.Instances[i].BlockID < c.Instances[j].BlockID
		}

		return c.Instances[i].InstanceID < c.Instances[j].InstanceID
	})

	// normalize switch names
	bandCounts := map[band]int{blockBand: 0, spineBand: 0, datacenterBand: 0}

	switches := make(map[string]string)
	for i, inst := range c.Instances {
		name, ok := switches[inst.BlockID]
		if !ok {
			bandCounts[blockBand]++
			name = fmt.Sprintf("switch.%d.%d", blockBand, bandCounts[blockBand])
			switches[inst.BlockID] = name
		}
		c.Instances[i].BlockName = name

		name, ok = switches[inst.SpineID]
		if !ok {
			bandCounts[spineBand]++
			name = fmt.Sprintf("switch.%d.%d", spineBand, bandCounts[spineBand])
			switches[inst.SpineID] = name
		}
		c.Instances[i].SpineName = name

		name, ok = switches[inst.DatacenterID]
		if !ok {
			bandCounts[datacenterBand]++
			name = fmt.Sprintf("switch.%d.%d", datacenterBand, bandCounts[datacenterBand])
			switches[inst.DatacenterID] = name
		}
		c.Instances[i].DatacenterName = name
	}
}
