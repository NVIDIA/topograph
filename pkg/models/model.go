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

const (
	LabelTopologyRegion = "topology.kubernetes.io/region"
	LabelTopologyZone   = "topology.kubernetes.io/zone"
	LabelAccelerator    = topology.KeyTopologyAccelerator
)

// Switch is a switch vertex in a simulation model YAML tree (tests/models).
type Switch struct {
	Name     string            `yaml:"name,omitempty"`
	Labels   map[string]string `yaml:"labels"`
	Switches []string          `yaml:"switches"`
	Nodes    []string          `yaml:"-"`
}

// CapacityBlock is the blocks entry shape in simulation model YAML.
type CapacityBlock struct {
	Switch string            `yaml:"switch"`
	Nodes  []string          `yaml:"nodes"`
	Labels map[string]string `yaml:"labels"`
}

type Model struct {
	Switches       map[string]*Switch            `yaml:"switches"`
	Nodes          map[string]*topology.Instance `yaml:"-"`
	CapacityBlocks []CapacityBlock               `yaml:"blocks"`

	// derived
	Instances []topology.ComputeInstances `yaml:"-"`
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
		Switches map[string]*Switch `yaml:"switches"`
		Blocks   []CapacityBlock    `yaml:"blocks"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	m.Switches = raw.Switches
	m.CapacityBlocks = raw.Blocks
	for name, sw := range m.Switches {
		if sw == nil {
			return fmt.Errorf("switch %q has empty definition", name)
		}
		if sw.Name != "" && sw.Name != name {
			return fmt.Errorf("switch key %q does not match switch name %q", name, sw.Name)
		}
		sw.Name = name
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
		return fmt.Errorf("switches reference nodes that are not declared by blocks")
	}

	for node := range maps.switchByNode {
		if _, ok := m.Nodes[node]; !ok {
			return fmt.Errorf("switch references unknown node %q", node)
		}
	}

	regions := make(map[string]map[string]string)
	for _, node := range m.Nodes {
		var netLayers []string
		var labels map[string]string

		if sw, ok := maps.switchByNode[node.ID]; ok {
			netLayers, labels, err = getNetworkLayers(sw, maps.parentBySwitch)
			if err != nil {
				return err
			}
		}

		node.Labels = mergeLabels(labels, node.Labels)
		node.NetLayers = netLayers
		addInstanceRegion(regions, node.Labels, node.ID)
	}

	m.setInstances(regions)
	return nil
}

func (m *Model) completeCapacityBlocks() error {
	for capacityBlockIndex := range m.CapacityBlocks {
		capacityBlock := m.CapacityBlocks[capacityBlockIndex]
		capacityBlock.Nodes = cluset.Expand(capacityBlock.Nodes)
		if len(capacityBlock.Nodes) == 0 {
			return fmt.Errorf("capacity block at index %d must declare at least one node", capacityBlockIndex)
		}
		if capacityBlock.Switch != "" {
			sw, ok := m.Switches[capacityBlock.Switch]
			if !ok {
				return fmt.Errorf("capacity block at index %d references unknown switch %q", capacityBlockIndex, capacityBlock.Switch)
			}
			for _, node := range capacityBlock.Nodes {
				sw.Nodes = appendUnique(sw.Nodes, node)
			}
		}
		m.CapacityBlocks[capacityBlockIndex] = capacityBlock
		for _, name := range capacityBlock.Nodes {
			if m.Nodes == nil {
				m.Nodes = make(map[string]*topology.Instance)
			}
			if _, ok := m.Nodes[name]; ok {
				return fmt.Errorf("node %q belongs to more than one capacity block", name)
			}
			m.Nodes[name] = &topology.Instance{
				ID:     name,
				Labels: cloneStringMap(capacityBlock.Labels),
			}
		}
	}

	return nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func setMapValue(values map[string]string, key, value string) map[string]string {
	if values == nil {
		values = make(map[string]string)
	}
	values[key] = value
	return values
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for k, v := range values {
		clone[k] = v
	}
	return clone
}

func mergeLabels(base, overlay map[string]string) map[string]string {
	labels := cloneStringMap(base)
	for k, v := range overlay {
		labels = setMapValue(labels, k, v)
	}
	return labels
}

func addInstanceRegion(regions map[string]map[string]string, labels map[string]string, name string) {
	region := labels[LabelTopologyRegion]
	if region == "" {
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
	path := []*Switch{}

	for {
		name := sw.Name
		// check for circular switch topology
		if _, ok := visited[name]; ok {
			return nil, nil, fmt.Errorf("circular dependency detected in topology for switch %q", name)
		}
		visited[name] = struct{}{}
		layers = append(layers, name)
		path = append(path, sw)
		parent, ok := swmap[name]
		if !ok {
			labels := map[string]string(nil)
			for i := len(path) - 1; i >= 0; i-- {
				labels = mergeLabels(labels, path[i].Labels)
			}
			return layers, labels, nil
		}
		sw = parent
	}
}

func (model *Model) ToGraph(instances []topology.ComputeInstances) (*topology.Graph, map[string]string) {
	instance2node := make(map[string]string)
	nodeVertexMap := make(map[string]*topology.Vertex)
	swVertexMap := make(map[string]*topology.Vertex)
	swRootMap := make(map[string]bool)
	domainMap := topology.NewDomainMap()

	// Create all the vertices for each node
	for k, v := range model.Nodes {
		instance2node[k] = fmt.Sprintf("n-%s", k)
		nodeVertexMap[k] = &topology.Vertex{ID: v.ID, Name: v.ID}
	}

	// Initialize all the vertices for each switch (setting each on to be a possible root)
	for _, sw := range model.Switches {
		swVertexMap[sw.Name] = &topology.Vertex{ID: sw.Name, Vertices: make(map[string]*topology.Vertex)}
		swRootMap[sw.Name] = true
	}

	// Initializes accelerator domain membership from node labels.
	for _, node := range model.Nodes {
		if accelerator := node.AcceleratorID(); accelerator != "" {
			domainMap.AddHost(accelerator, node.ID, node.ID)
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
	if instanceMap := model.InstanceMap(instances); len(instanceMap) != 0 {
		graph.Instances = instanceMap
	}

	return graph, instance2node
}

func (model *Model) InstanceMap(computeInstances []topology.ComputeInstances) map[string]topology.Instance {
	wanted := requestedInstanceIDs(computeInstances)
	instances := make(map[string]topology.Instance)

	for instanceID, inst := range model.Nodes {
		if len(wanted) != 0 {
			if _, ok := wanted[instanceID]; !ok {
				continue
			}
		}
		instances[instanceID] = inst.CloneForTopology()
	}
	return instances
}

func requestedInstanceIDs(computeInstances []topology.ComputeInstances) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, ci := range computeInstances {
		for instanceID := range ci.Instances {
			ids[instanceID] = struct{}{}
		}
	}
	return ids
}
