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
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/tests"
)

type Model struct {
	Switches       map[string]*Switch       `yaml:"switches"`
	Nodes          map[string]*Node         `yaml:"nodes"`
	CapacityBlocks map[string]CapacityBlock `yaml:"capacity_blocks"`

	// derived
	Instances []topology.ComputeInstances `yaml:"-"`
}

type Switch struct {
	Name     string            `yaml:"name,omitempty"`
	Metadata map[string]string `yaml:"metadata"`
	Switches []string          `yaml:"switches"`
	Nodes    []string          `yaml:"nodes"`
}

type BasicNodeAttributes struct {
	NVLink string `yaml:"nvlink,omitempty"`
}

type NodeAttributes struct {
	BasicNodeAttributes `yaml:",inline"`
	Status              string `yaml:"status,omitempty"`
	Timestamp           string `yaml:"timestamp,omitempty"`
	GPUs                []GPU  `yaml:"gpus,omitempty"`
}

type CapacityBlock struct {
	Nodes      []string            `yaml:"nodes"`
	Attributes BasicNodeAttributes `yaml:"attributes"`
}

type GPU struct {
	Index     int    `yaml:"index"`
	PCIBusID  string `yaml:"pci_bus_id"`
	UUID      string `yaml:"uuid"`
	Model     string `yaml:"model"`
	MemoryMiB int    `yaml:"memory_mib"`
}

type Node struct {
	Name          string         `yaml:"name,omitempty"`
	Attributes    NodeAttributes `yaml:"attributes"`
	CapacityBlock string         `yaml:"capacity_block_id"`

	Metadata  map[string]string `yaml:"-"`
	NetLayers []string          `yaml:"-"`
}

func (n *Node) String() string {
	return fmt.Sprintf("Node: %s Metadata: %v NetLayers: %v Attr: %v",
		n.Name, n.Metadata, n.NetLayers, n.Attributes)
}

func NewModelFromFile(fname string) (*Model, error) {
	var err error
	var data []byte
	//Check if the fileName includes the path (absolute or relative).
	//If it is just the fileName, load it from the testdata directory first, otherwise fallback to the given path
	dir, fileName := filepath.Split(fname)
	if len(dir) == 0 {
		data, err = tests.GetModelFileData(fileName)
	} else {
		data, err = os.ReadFile(fname)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", fname, err)
	}
	return NewModelFromData(data, fname)
}

func NewModelFromData(data []byte, fname string) (*Model, error) {
	model := &Model{}
	if err := yaml.Unmarshal(data, model); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", fname, err)
	}

	if err := model.derive(); err != nil {
		return nil, err
	}

	return model, nil
}

func (m *Model) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Switches       map[string]*Switch       `yaml:"switches"`
		Nodes          map[string]*Node         `yaml:"nodes"`
		CapacityBlocks map[string]CapacityBlock `yaml:"capacity_blocks"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	m.Switches = raw.Switches
	m.CapacityBlocks = raw.CapacityBlocks
	m.Nodes = raw.Nodes
	for name, sw := range m.Switches {
		if sw == nil {
			return fmt.Errorf("switch %q has empty definition", name)
		}
		if sw.Name != "" && sw.Name != name {
			return fmt.Errorf("switch key %q does not match switch name %q", name, sw.Name)
		}
		sw.Name = name
	}
	for name, node := range m.Nodes {
		if node == nil {
			return fmt.Errorf("node %q has empty definition", name)
		}
		if node.Name != "" && node.Name != name {
			return fmt.Errorf("node key %q does not match node name %q", name, node.Name)
		}
		node.Name = name
	}
	return nil
}

type switchMaps struct {
	parentBySwitch map[string]*Switch
	switchByNode   map[string]*Switch
}

func (m *Model) buildSwitchMaps() (*switchMaps, error) {
	// switch map child:parent
	swmap := make(map[string]*Switch)
	nodeMap := make(map[string]*Switch)

	for _, parent := range m.Switches {
		for _, sw := range parent.Switches {
			if p, ok := swmap[sw]; ok {
				// a child switch cannot have more than one parent switch
				return nil, fmt.Errorf("switch %q has two parent switches %q and %q", sw, parent.Name, p.Name)
			}
			swmap[sw] = parent
		}
		parent.Nodes = cluset.Expand(parent.Nodes)
		for _, node := range parent.Nodes {
			if p, ok := nodeMap[node]; ok {
				// a node cannot be attached to more than one switch
				return nil, fmt.Errorf("node %q has two switches %q and %q", node, parent.Name, p.Name)
			}
			nodeMap[node] = parent
		}
	}

	return &switchMaps{
		parentBySwitch: swmap,
		switchByNode:   nodeMap,
	}, nil
}

func (m *Model) derive() error {
	if err := m.completeCapacityBlocks(); err != nil {
		return err
	}

	maps, err := m.buildSwitchMaps()
	if err != nil {
		return err
	}

	if len(m.Nodes) == 0 && len(maps.switchByNode) != 0 {
		return fmt.Errorf("switches reference nodes but top-level nodes section is empty")
	}

	for node := range maps.switchByNode {
		if _, ok := m.Nodes[node]; !ok {
			return fmt.Errorf("switch references unknown node %q", node)
		}
	}

	regions := make(map[string]map[string]string)
	for _, node := range m.Nodes {
		var netLayers []string
		var metadata map[string]string

		if sw, ok := maps.switchByNode[node.Name]; ok {
			netLayers, metadata, err = getNetworkLayers(sw, maps.parentBySwitch)
			if err != nil {
				return err
			}
		}

		node.Metadata = metadata
		node.NetLayers = netLayers
		addInstanceRegion(regions, metadata, node.Name)
	}

	m.setInstances(regions)
	return nil
}

func (m *Model) completeCapacityBlocks() error {
	hasTopLevelNodes := len(m.Nodes) != 0

	for capacityBlockID, capacityBlock := range m.CapacityBlocks {
		capacityBlock.Nodes = cluset.Expand(capacityBlock.Nodes)
		m.CapacityBlocks[capacityBlockID] = capacityBlock
		for _, name := range capacityBlock.Nodes {
			if !hasTopLevelNodes {
				if m.Nodes == nil {
					m.Nodes = make(map[string]*Node)
				}
				if _, ok := m.Nodes[name]; ok {
					return fmt.Errorf("node %q belongs to more than one capacity block", name)
				}
				m.Nodes[name] = &Node{
					Name:          name,
					Attributes:    copyNodeAttributes(capacityBlock.Attributes),
					CapacityBlock: capacityBlockID,
				}
				continue
			}

			node, ok := m.Nodes[name]
			if !ok {
				return fmt.Errorf("capacity block %q references unknown node %q", capacityBlockID, name)
			}
			if node.CapacityBlock != "" && node.CapacityBlock != capacityBlockID {
				return fmt.Errorf("node %q belongs to capacity blocks %q and %q", name, node.CapacityBlock, capacityBlockID)
			}
			node.CapacityBlock = capacityBlockID
			applyCapacityBlockAttributes(node, capacityBlock.Attributes)
		}
	}

	for _, node := range m.Nodes {
		if node.CapacityBlock == "" {
			continue
		}
		if m.CapacityBlocks == nil {
			m.CapacityBlocks = make(map[string]CapacityBlock)
		}
		capacityBlock := m.CapacityBlocks[node.CapacityBlock]
		capacityBlock.Nodes = appendUnique(capacityBlock.Nodes, node.Name)
		if capacityBlock.Attributes.NVLink == "" {
			capacityBlock.Attributes.NVLink = node.Attributes.NVLink
		}
		sort.Strings(capacityBlock.Nodes)
		m.CapacityBlocks[node.CapacityBlock] = capacityBlock
	}

	return nil
}

func copyNodeAttributes(attributes BasicNodeAttributes) NodeAttributes {
	return NodeAttributes{
		BasicNodeAttributes: BasicNodeAttributes{
			NVLink: attributes.NVLink,
		},
	}
}

func applyCapacityBlockAttributes(node *Node, attributes BasicNodeAttributes) {
	if attributes.NVLink != "" {
		node.Attributes.NVLink = attributes.NVLink
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func addInstanceRegion(regions map[string]map[string]string, metadata map[string]string, name string) {
	region, ok := metadata["region"]
	if !ok {
		region = "none"
	}
	r, ok := regions[region]
	if !ok {
		r = make(map[string]string)
		regions[region] = r
	}
	r[name] = fmt.Sprintf("n-%s", name)
}

func (m *Model) setInstances(regions map[string]map[string]string) {
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

func (model *Model) ToGraph() (*topology.Graph, map[string]string) {
	instance2node := make(map[string]string)
	nodeVertexMap := make(map[string]*topology.Vertex)
	swVertexMap := make(map[string]*topology.Vertex)
	swRootMap := make(map[string]bool)
	domainMap := topology.NewDomainMap()

	// Create all the vertices for each node
	for k, v := range model.Nodes {
		instance2node[k] = fmt.Sprintf("n-%s", k)
		nodeVertexMap[k] = &topology.Vertex{ID: v.Name, Name: v.Name}
	}

	// Initialize all the vertices for each switch (setting each on to be a possible root)
	for _, sw := range model.Switches {
		swVertexMap[sw.Name] = &topology.Vertex{ID: sw.Name, Vertices: make(map[string]*topology.Vertex)}
		swRootMap[sw.Name] = true
	}

	// Initializes accelerator domain membership from node attributes.
	for _, node := range model.Nodes {
		if node.Attributes.NVLink != "" {
			domainMap.AddHost(node.Attributes.NVLink, node.Name, node.Name)
		}
	}

	// Connect all the switches to their sub-switches and sub-nodes
	for _, sw := range model.Switches {
		for _, subsw := range sw.Switches {
			swRootMap[subsw] = false
			swVertexMap[sw.Name].Vertices[subsw] = swVertexMap[subsw]
		}
		for _, node := range sw.Nodes {
			swVertexMap[sw.Name].Vertices[node] = nodeVertexMap[node]
		}
	}

	// Connects all root vertices to the hidden root
	treeRoot := &topology.Vertex{Vertices: make(map[string]*topology.Vertex)}
	for k, v := range swRootMap {
		if v {
			treeRoot.Vertices[k] = swVertexMap[k]
		}
	}
	graph := &topology.Graph{
		Tiers: treeRoot,
	}

	if len(domainMap) != 0 {
		graph.Domains = domainMap
	}

	return graph, instance2node
}
