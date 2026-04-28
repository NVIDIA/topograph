/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package sim

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/topology"
)

// Wire JSON shape matches the DSX GET …/topology/…/nodes response body.

type topologyWire struct {
	Switches map[string]switchWire `json:"switches"`
}

type switchWire struct {
	Switches []string   `json:"switches,omitempty"`
	Nodes    []nodeWire `json:"nodes,omitempty"`
}

type nodeWire struct {
	NodeID               string `json:"node_id"`
	AcceleratedNetworkID string `json:"accelerated_network_id,omitempty"`
}

// responseBytesFromModelFile loads a topology YAML via [models.NewModelFromFile] (basename-only
// names resolve to embedded tests/models), converts [models.Model.ToGraph] under topology/tree to
// DSX JSON (using the instance→wired-node map from the same ToGraph call), and marshals the result.
func responseBytesFromModelFile(modelPath string) ([]byte, error) {
	m, err := models.NewModelFromFile(modelPath)
	if err != nil {
		return nil, err
	}
	root, instance2node := m.ToGraph()
	tree := root.Vertices[topology.TopologyTree]
	if tree == nil {
		return nil, fmt.Errorf("model: missing %q subtree in ToGraph output", topology.TopologyTree)
	}
	doc, err := topologyWireFromGraph(m, tree, instance2node)
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

func topologyWireFromGraph(m *models.Model, tree *topology.Vertex, instance2node map[string]string) (*topologyWire, error) {
	switchIDs := make(map[string]struct{}, len(m.Switches))
	for _, sw := range m.Switches {
		switchIDs[sw.Name] = struct{}{}
	}

	vertexByID := indexVerticesByID(tree)
	cbByName := make(map[string]*models.CapacityBlock, len(m.CapacityBlocks))
	for _, cb := range m.CapacityBlocks {
		cbByName[cb.Name] = cb
	}

	out := &topologyWire{Switches: make(map[string]switchWire, len(m.Switches))}
	for _, sw := range m.Switches {
		v := vertexByID[sw.Name]
		if v == nil {
			return nil, fmt.Errorf("switch %q not found under topology tree", sw.Name)
		}
		swWire, err := switchWireFromVertex(m, v, switchIDs, instance2node, cbByName)
		if err != nil {
			return nil, err
		}
		out.Switches[sw.Name] = swWire
	}
	return out, nil
}

// indexVerticesByID walks the tree once so switch (and other) vertices resolve in O(1).
// Later DFS visits overwrite earlier entries on duplicate IDs (same as a full-tree search).
func indexVerticesByID(root *topology.Vertex) map[string]*topology.Vertex {
	out := make(map[string]*topology.Vertex)
	var walk func(*topology.Vertex)
	walk = func(v *topology.Vertex) {
		if v == nil {
			return
		}
		if v.ID != "" {
			out[v.ID] = v
		}
		for _, ch := range v.Vertices {
			walk(ch)
		}
	}
	walk(root)
	return out
}

func switchWireFromVertex(m *models.Model, v *topology.Vertex, switchIDs map[string]struct{}, instance2node map[string]string, cbByName map[string]*models.CapacityBlock) (switchWire, error) {
	var swNames, nodeKeys []string
	for key := range v.Vertices {
		ch := v.Vertices[key]
		if ch == nil {
			continue
		}
		if _, ok := switchIDs[ch.ID]; ok {
			swNames = append(swNames, ch.ID)
			continue
		}
		nodeKeys = append(nodeKeys, nodeKeyFromVertex(key, ch))
	}
	sort.Strings(swNames)
	sort.Strings(nodeKeys)

	info := switchWire{Switches: swNames}
	for _, nk := range nodeKeys {
		wired, ok := instance2node[nk]
		if !ok {
			return switchWire{}, fmt.Errorf("node %q has no entry in ToGraph instance2node map", nk)
		}
		info.Nodes = append(info.Nodes, nodeWire{
			NodeID:               wired,
			AcceleratedNetworkID: nodeAccelFromModel(cbByName, m.Nodes[nk]),
		})
	}
	return info, nil
}

func nodeKeyFromVertex(mapKey string, v *topology.Vertex) string {
	if v.Name != "" {
		return v.Name
	}
	if v.ID != "" {
		return v.ID
	}
	return mapKey
}

func nodeAccelFromModel(cbByName map[string]*models.CapacityBlock, n *models.Node) string {
	if n == nil {
		return ""
	}
	cb := cbByName[n.CapacityBlock]
	if cb == nil {
		return ""
	}
	return acceleratedNetworkID(cb)
}

func acceleratedNetworkID(cb *models.CapacityBlock) string {
	if cb.NVLink != "" {
		return cb.NVLink
	}
	return strings.ToLower(cb.Name) + "-" + strings.ToLower(cb.Type)
}
