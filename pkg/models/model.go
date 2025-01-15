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

package models

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/NVIDIA/topograph/pkg/topology"
)

type Model struct {
	Switches       []*Switch        `yaml:"switches"`
	CapacityBlocks []*CapacityBlock `yaml:"capacity_blocks"`

	// derived
	Nodes     map[string]*Node
	Instances []topology.ComputeInstances
}

type Switch struct {
	Name           string            `yaml:"name"`
	Metadata       map[string]string `yaml:"metadata"`
	Switches       []string          `yaml:"switches"`
	CapacityBlocks []string          `yaml:"capacity_blocks"`
}

type CapacityBlock struct {
	Name   string   `yaml:"name"`
	Type   string   `yaml:"type"`
	NVLink string   `yaml:"nvlink,omitempty"`
	Nodes  []string `yaml:"nodes"`
}

type Node struct {
	Name          string
	Metadata      map[string]string
	Type          string
	NVLink        string
	NetLayers     []string
	CapacityBlock string
}

func (n *Node) String() string {
	return fmt.Sprintf("Node: %s Metadata: %v Type: %s NVL: %s NetLayers: %v CBlock: %s",
		n.Name, n.Metadata, n.Type, n.NVLink, n.NetLayers, n.CapacityBlock)
}

func NewModelFromFile(fname string) (*Model, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", fname, err)
	}

	model := &Model{}
	if err = yaml.Unmarshal(data, model); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", fname, err)
	}

	if err = model.setNodeMap(); err != nil {
		return nil, err
	}

	return model, err
}

func (m *Model) setNodeMap() error {
	// switch map child:parent
	swmap := make(map[string]*Switch)
	// capacity block map cb:switch
	cbmap := make(map[string]*Switch)

	for _, parent := range m.Switches {
		for _, sw := range parent.Switches {
			if p, ok := swmap[sw]; ok {
				// a child switch cannot have more than one parent switch
				return fmt.Errorf("switch %q has two parent switches %q and %q", sw, parent.Name, p)
			}
			swmap[sw] = parent
		}
		for _, cb := range parent.CapacityBlocks {
			if p, ok := cbmap[cb]; ok {
				// a capacity block cannot have more than one switch
				return fmt.Errorf("capacity block %q has two switches %q and %q", cb, parent.Name, p)
			}
			cbmap[cb] = parent
		}
	}

	m.Nodes = make(map[string]*Node)
	regions := make(map[string]map[string]string)
	for _, cb := range m.CapacityBlocks {
		var netLayers []string
		var metadata map[string]string
		var err error

		sw, ok := cbmap[cb.Name]
		if ok {
			netLayers, metadata, err = getNetworkLayers(sw, swmap)
			if err != nil {
				return err
			}
		}

		for _, name := range cb.Nodes {
			if _, ok := m.Nodes[name]; ok {
				return fmt.Errorf("duplicated node name %q", name)
			}
			m.Nodes[name] = &Node{
				Name:          name,
				Metadata:      metadata,
				Type:          cb.Type,
				NVLink:        cb.NVLink,
				NetLayers:     netLayers,
				CapacityBlock: cb.Name,
			}

			region, ok := metadata["region"]
			if !ok {
				region = "none"
			}
			r, ok := regions[region]
			if !ok {
				r = make(map[string]string)
				regions[region] = r
			}
			r[name] = name
		}
	}

	regionNames := make([]string, 0, len(regions))
	for region := range regions {
		regionNames = append(regionNames, region)
	}
	sort.Strings(regionNames)

	m.Instances = make([]topology.ComputeInstances, 0, len(regions))
	for _, region := range regionNames {
		m.Instances = append(m.Instances, topology.ComputeInstances{
			Region:    region,
			Instances: regions[region],
		})
	}

	return nil
}

func getNetworkLayers(sw *Switch, swmap map[string]*Switch) ([]string, map[string]string, error) {
	visited := make(map[string]struct{})
	layers := []string{}
	metadata := make(map[string]string)

	for {
		name := sw.Name
		// check for circular switch topology
		if _, ok := visited[name]; ok {
			return nil, nil, fmt.Errorf("circular dependency detected in topology for switch %q", name)
		}
		visited[name] = struct{}{}
		layers = append(layers, name)
		for k, v := range sw.Metadata {
			metadata[k] = v
		}
		parent, ok := swmap[name]
		if !ok {
			return layers, metadata, nil
		}
		sw = parent
	}
}

func (model *Model) ToGraph() (*topology.Vertex, map[string]string) {
	instance2node := make(map[string]string)
	nodeVertexMap := make(map[string]*topology.Vertex)
	swVertexMap := make(map[string]*topology.Vertex)
	swRootMap := make(map[string]bool)
	blockVertexMap := make(map[string]*topology.Vertex)
	var block_topology bool = false

	// Create all the vertices for each node
	for k, v := range model.Nodes {
		instance2node[k] = k
		nodeVertexMap[k] = &topology.Vertex{ID: v.Name, Name: v.Name}
	}

	// Initialize all the vertices for each switch (setting each on to be a possible root)
	for _, sw := range model.Switches {
		swVertexMap[sw.Name] = &topology.Vertex{ID: sw.Name, Vertices: make(map[string]*topology.Vertex)}
		swRootMap[sw.Name] = true
	}

	// Initializes all the block vertices
	for _, cb := range model.CapacityBlocks {
		blockVertexMap[cb.Name] = &topology.Vertex{ID: cb.Name, Vertices: make(map[string]*topology.Vertex)}
		for _, node := range cb.Nodes {
			blockVertexMap[cb.Name].Vertices[node] = nodeVertexMap[node]
		}
		if len(cb.NVLink) != 0 {
			block_topology = true
		}
	}

	// Connect all the switches to their sub-switches and sub-nodes
	for _, sw := range model.Switches {
		for _, subsw := range sw.Switches {
			swRootMap[subsw] = false
			swVertexMap[sw.Name].Vertices[subsw] = swVertexMap[subsw]
		}
		for _, cbname := range sw.CapacityBlocks {
			for _, block := range model.CapacityBlocks {
				if cbname == block.Name {
					for _, node := range block.Nodes {
						swVertexMap[sw.Name].Vertices[node] = nodeVertexMap[node]
					}
					break
				}
			}
		}
	}

	// Connects all root vertices to the hidden root
	treeRoot := &topology.Vertex{Vertices: make(map[string]*topology.Vertex)}
	for k, v := range swRootMap {
		if v {
			treeRoot.Vertices[k] = swVertexMap[k]
		}
	}
	blockRoot := &topology.Vertex{Vertices: make(map[string]*topology.Vertex)}
	for k, v := range blockVertexMap {
		blockRoot.Vertices[k] = v
	}

	rootNode := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}

	if block_topology {
		rootNode.Vertices[topology.TopologyBlock] = blockRoot
	}

	rootNode.Vertices[topology.TopologyTree] = treeRoot
	return rootNode, instance2node
}
