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

var (
	labelAccelerator, labelLeaf, labelSpine, labelCore string

	switchNetworkHierarchy []string
)

func InitLabels(accelerator, leaf, spine, core string) {
	labelAccelerator = accelerator
	labelLeaf = leaf
	labelSpine = spine
	labelCore = core
	switchNetworkHierarchy = []string{labelLeaf, labelSpine, labelCore}
}

// map nodename:[label name: label value]
type nodeLabelMap map[string]map[string]string

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

func (l *topologyLabeler) ApplyNodeLabels(ctx context.Context, v *topology.Vertex, labeler Labeler) error {
	if v == nil || len(v.Vertices) == 0 {
		return nil
	}

	nodeMap := make(nodeLabelMap)
	if blockRoot, ok := v.Vertices[topology.TopologyBlock]; ok {
		if err := l.getBlockNodeLabels(blockRoot, nodeMap); err != nil {
			return err
		}
	}

	if treeRoot, ok := v.Vertices[topology.TopologyTree]; ok {
		layers := []string{}
		if len(treeRoot.ID) != 0 {
			layers = append(layers, treeRoot.ID)
		}
		if err := l.getTreeNodeLabels(treeRoot, nodeMap, layers); err != nil {
			return err
		}
	}

	for nodeName, labels := range nodeMap {
		if err := labeler.AddNodeLabels(ctx, nodeName, labels); err != nil {
			return err
		}
	}

	return nil
}

func (l *topologyLabeler) getTreeNodeLabels(v *topology.Vertex, nodeMap nodeLabelMap, layers []string) error {
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
		if err := l.getTreeNodeLabels(w, nodeMap, append([]string{w.ID}, layers...)); err != nil {
			return err
		}
	}

	return nil
}

func (l *topologyLabeler) getBlockNodeLabels(v *topology.Vertex, nodeMap nodeLabelMap) error {
	for _, block := range v.Vertices {
		for _, node := range block.Vertices {
			nodeName := node.Name
			labels, ok := nodeMap[nodeName]
			if !ok {
				labels = make(map[string]string)
				nodeMap[nodeName] = labels
			}
			if val, ok := labels[labelAccelerator]; ok {
				return fmt.Errorf("multiple accelerator labels %s, %s for node %s", val, block.ID, nodeName)
			}
			labels[labelAccelerator] = l.checkLabel(block.ID)
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
