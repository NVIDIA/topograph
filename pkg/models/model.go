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

	"gopkg.in/yaml.v3"

	"github.com/NVIDIA/topograph/pkg/topology"
)

type Model struct {
	Switches       []Switch         `yaml:"switches"`
	CapacityBlocks []CapacityBlock  `yaml:"capacity_blocks"`
	PhysicalLayers []PhysicalLayers `yaml:"physical_layers"`

	// defived
	Nodes map[string]*Node
}

type Switch struct {
	Name           string   `yaml:"name"`
	Switches       []string `yaml:"switches"`
	CapacityBlocks []string `yaml:"capacity_blocks"`
}

type CapacityBlock struct {
	Name   string   `yaml:"name"`
	Type   string   `yaml:"type"`
	NVLink string   `yaml:"nvlink,omitempty"`
	Nodes  []string `yaml:"nodes"`
}

type PhysicalLayers struct {
	Name           string   `yaml:"name"`
	Type           string   `yaml:"type"`
	SubLayers      []string `yaml:"sub_layers"`
	CapacityBlocks []string `yaml:"capacity_blocks"`
}

const (
	PhysicalLayerRegion = "region"
	PhysicalLayerAZ     = "availability_zone"
)

type Node struct {
	Name          string
	Type          string
	NVLink        string
	NetLayers     []string
	CapacityBlock string
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

	if err = validateLayers(model.PhysicalLayers); err != nil {
		return nil, err
	}

	return model, err
}

func (m *Model) setNodeMap() error {
	// switch map child:parent
	swmap := make(map[string]string)
	// capacity block map cb:switch
	cbmap := make(map[string]string)
	for _, parent := range m.Switches {
		for _, sw := range parent.Switches {
			if p, ok := swmap[sw]; ok {
				// a child switch cannot have more than one parent switch
				return fmt.Errorf("switch %q has two parent switches %q and %q", sw, parent.Name, p)
			}
			swmap[sw] = parent.Name
		}
		for _, cb := range parent.CapacityBlocks {
			if p, ok := cbmap[cb]; ok {
				// a capacity block cannot have more than one switch
				return fmt.Errorf("capacity block %q has two switches %q and %q", cb, parent.Name, p)
			}
			cbmap[cb] = parent.Name
		}
	}

	m.Nodes = make(map[string]*Node)
	for _, cb := range m.CapacityBlocks {
		var netLayers []string
		sw, ok := cbmap[cb.Name]
		if ok {
			net, err := getNetworkLayers(sw, swmap)
			if err != nil {
				return err
			}
			netLayers = net
		}

		for _, name := range cb.Nodes {
			if _, ok := m.Nodes[name]; ok {
				return fmt.Errorf("duplicated node name %q", name)
			}
			m.Nodes[name] = &Node{
				Name:          name,
				Type:          cb.Type,
				NVLink:        cb.NVLink,
				NetLayers:     netLayers,
				CapacityBlock: cb.Name,
			}
		}
	}

	return nil
}

func getNetworkLayers(name string, swmap map[string]string) ([]string, error) {
	sw := make(map[string]bool)
	res := []string{}
	for {
		// check for circular switch topology
		if _, ok := sw[name]; ok {
			return nil, fmt.Errorf("circular topology for switch %q", name)
		}
		sw[name] = true
		res = append(res, name)

		parent, ok := swmap[name]
		if !ok {
			return res, nil
		}
		name = parent
	}
}

// Check to make sure each layer is unique and has only a single parent, if any, and enumerates them into a map of layer name to parent index within the layers list
// If the layer has no entry within the map, it means that the layer has no parent
func getLayerParentMap(layers []PhysicalLayers) (map[string]int, error) {
	parentMap := make(map[string]int)
	for i, layer := range layers {
		layerName := layer.Name
		var parentCount int = 0
		for j, checkLayer := range layers {
			if i == j {
				continue
			}
			if layerName == checkLayer.Name {
				return nil, fmt.Errorf("duplicated physical layer name %q", layerName)
			}
			for _, subLayerName := range checkLayer.SubLayers {
				if layerName == subLayerName {
					parentMap[layerName] = j
					parentCount++
					break
				}
			}
		}
		if parentCount > 1 {
			return nil, fmt.Errorf("physical layer with name %q has more than one parent (%d parents)", layerName, parentCount)
		}
	}
	return parentMap, nil
}

func validateLayers(layers []PhysicalLayers) error {
	// Validates the parent structure of the layers and gets the map of nodes to parents
	parentMap, err := getLayerParentMap(layers)
	if err != nil {
		return err
	}

	// Check to make sure there are no loops among the physical layers
	for _, layer := range layers {
		layerName := layer.Name
		currLayerIdx, ok := parentMap[layerName]
		if ok {
			currLayerName := layers[currLayerIdx].Name
			for {
				currLayerIdx, ok = parentMap[currLayerName]
				if !ok {
					break
				}
				currLayerName = layers[currLayerIdx].Name
				if currLayerName == layerName {
					return fmt.Errorf("circular layer dependencies involving layer with name %q", layerName)
				}
			}
		}
	}
	return nil
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
	if block_topology {
		root := &topology.Vertex{
			Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
			Metadata: map[string]string{topology.KeyPlugin: topology.TopologyBlock},
		}
		return root, instance2node
	}
	treeRoot.Metadata = map[string]string{topology.KeyPlugin: topology.TopologyTree}
	return treeRoot, instance2node
}

// Get a map that maps from the name of each node in the model to the cloestest physical layer that shares the given type.
// If no entry exists for the node in the returned map, then there is no layer the node exists in with the given type
func (model *Model) NodeToLayerMap(layerType string) (map[string]string, error) {
	// Maps each capacity block to a parent physical layer index
	cbToLayer := make(map[string]int)
	for idx, layer := range model.PhysicalLayers {
		for _, cbName := range layer.CapacityBlocks {
			cbToLayer[cbName] = idx
		}
	}

	// Goes through each node, gets the capacity block, and walks up the parent tree to find the cloest layer of the given type
	nodeToLayer := make(map[string]string)
	layerParentMap, err := getLayerParentMap(model.PhysicalLayers)
	if err != nil {
		return nil, err
	}
	for _, node := range model.Nodes {
		cb := node.CapacityBlock
		layerIdx, ok := cbToLayer[cb]
		if !ok {
			return nil, fmt.Errorf("capacity block %q not found in any physical layer", cb)
		}
		for {
			layer := model.PhysicalLayers[layerIdx]
			if layer.Type == layerType {
				nodeToLayer[node.Name] = layer.Name
				break
			}
			layerIdx, ok = layerParentMap[layer.Name]
			if !ok {
				break
			}
		}
	}

	return nodeToLayer, nil
}
