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

package k8s

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	DefaultLabelAccelerator = "network.topology.nvidia.com/accelerator"
	DefaultLabelLeaf        = "network.topology.nvidia.com/leaf"
	DefaultLabelSpine       = "network.topology.nvidia.com/spine"
	DefaultLabelCore        = "network.topology.nvidia.com/core"
)

type TopologyLabelKeys struct {
	Accelerator string
	Leaf        string
	Spine       string
	Core        string
}

var topologyLabelKeys = TopologyLabelKeys{
	Accelerator: DefaultLabelAccelerator,
	Leaf:        DefaultLabelLeaf,
	Spine:       DefaultLabelSpine,
	Core:        DefaultLabelCore,
}

func InitLabels(accelerator, leaf, spine, core string) {
	topologyLabelKeys = TopologyLabelKeys{
		Accelerator: accelerator,
		Leaf:        leaf,
		Spine:       spine,
		Core:        core,
	}
}

func CurrentTopologyLabelKeys() TopologyLabelKeys {
	return topologyLabelKeys
}

// map nodename:[label name: label value]
type NodeLabelMap map[string]map[string]string

type Labeler interface {
	AddNodeLabels(context.Context, string, map[string]string) error
}

type topologyLabeler struct {
	mapper map[string]string
}

func NewTopologyLabeler() *topologyLabeler {
	return &topologyLabeler{
		mapper: make(map[string]string),
	}
}

func (l *topologyLabeler) ApplyNodeLabels(ctx context.Context, graph *topology.Graph, labeler Labeler) error {
	nodeMap, err := l.BuildNodeLabels(graph)
	if err != nil {
		return err
	}

	for nodeName, labels := range nodeMap {
		if err := labeler.AddNodeLabels(ctx, nodeName, labels); err != nil {
			return err
		}
	}

	return nil
}

func (l *topologyLabeler) BuildNodeLabels(graph *topology.Graph) (NodeLabelMap, error) {
	nodeMap := make(NodeLabelMap)

	if graph == nil || (graph.Domains == nil && graph.Tiers == nil) {
		return nodeMap, nil
	}

	if graph.Domains != nil {
		if err := l.getDomainLabels(graph.Domains, nodeMap); err != nil {
			return nil, err
		}
	}

	if treeRoot := graph.Tiers; treeRoot != nil {
		layers := []string{}
		if len(treeRoot.ID) != 0 {
			layers = append(layers, treeRoot.ID)
		}
		if err := l.getTierLabels(treeRoot, nodeMap, layers); err != nil {
			return nil, err
		}
	}

	return nodeMap, nil
}

func (l *topologyLabeler) getDomainLabels(domains topology.DomainMap, nodeMap NodeLabelMap) error {
	for domainName, domain := range domains {
		for nodeName := range domain {
			labels, ok := nodeMap[nodeName]
			if !ok {
				labels = make(map[string]string)
				nodeMap[nodeName] = labels
			}
			if val, ok := labels[topologyLabelKeys.Accelerator]; ok {
				return fmt.Errorf("multiple accelerator labels %s, %s for node %s", val, domainName, nodeName)
			}
			labels[topologyLabelKeys.Accelerator] = l.checkLabel(domainName)
		}
	}
	return nil
}

func (l *topologyLabeler) getTierLabels(v *topology.Vertex, nodeMap NodeLabelMap, layers []string) error {
	if len(v.Vertices) == 0 { // compute node
		if len(layers) != 0 {
			if v.ID != layers[0] {
				return fmt.Errorf("instance ID mismatch: expected %s, got %s", v.ID, layers[0])
			}
			nodeName := v.Name
			labels, ok := nodeMap[nodeName]
			if !ok {
				labels = make(map[string]string)
				nodeMap[nodeName] = labels
			}
			switchNetworkHierarchy := [...]string{
				topologyLabelKeys.Leaf,
				topologyLabelKeys.Spine,
				topologyLabelKeys.Core,
			}
			for i, sw := range layers[1:] {
				if len(sw) == 0 {
					break
				}
				if i < len(switchNetworkHierarchy) {
					labels[(switchNetworkHierarchy[i])] = l.checkLabel(sw)
				}
			}
		}
		return nil
	}

	for _, w := range v.Vertices {
		if err := l.getTierLabels(w, nodeMap, append([]string{w.ID}, layers...)); err != nil {
			return err
		}
	}

	return nil
}

// checkLabel checks the length of the label value.
// If more than 63 characters (Kubernetes limit), it will replace it with hash
func (l *topologyLabeler) checkLabel(val string) string {
	v, ok := l.mapper[val]
	if ok {
		return v
	}

	if len(val) <= 63 {
		v = val
	} else {
		h := fnv.New64a()
		h.Write([]byte(val))
		v = fmt.Sprintf("x%x", h.Sum64())
	}

	l.mapper[val] = v
	return v
}
