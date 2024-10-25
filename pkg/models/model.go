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
)

type Model struct {
	Switches       []Switch        `yaml:"switches"`
	CapacityBlocks []CapacityBlock `yaml:"capacity_blocks"`

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

type Node struct {
	Name      string
	Type      string
	NVLink    string
	NetLayers []string
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
				Name:      name,
				Type:      cb.Type,
				NVLink:    cb.NVLink,
				NetLayers: netLayers,
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
