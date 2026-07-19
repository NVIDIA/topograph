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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

type testLabeler struct {
	data map[string]map[string]string
}

func (l *testLabeler) AddNodeLabels(_ context.Context, nodeName string, labels map[string]string) error {
	if _, ok := l.data[nodeName]; ok {
		return fmt.Errorf("duplicate entry for %s", nodeName)
	}
	l.data[nodeName] = labels
	return nil
}

func TestApplyNodeLabelsWithTree(t *testing.T) {
	InitLabels(DefaultFabricLabelPrefix, DefaultAcceleratedLabelPrefix)
	root, _ := translate.GetTreeTestSet(true)
	labeler := &testLabeler{data: make(map[string]map[string]string)}
	data := map[string]map[string]string{
		"Node201": {"network.topology.nvidia.com/level-0": "S2", "network.topology.nvidia.com/level-1": "S1"},
		"Node202": {"network.topology.nvidia.com/level-0": "S2", "network.topology.nvidia.com/level-1": "S1"},
		"Node205": {"network.topology.nvidia.com/level-0": "S2", "network.topology.nvidia.com/level-1": "S1"},
		"Node304": {"network.topology.nvidia.com/level-0": "xf946c4acef2d5939", "network.topology.nvidia.com/level-1": "S1"},
		"Node305": {"network.topology.nvidia.com/level-0": "xf946c4acef2d5939", "network.topology.nvidia.com/level-1": "S1"},
		"Node306": {"network.topology.nvidia.com/level-0": "xf946c4acef2d5939", "network.topology.nvidia.com/level-1": "S1"},
	}

	err := NewTopologyLabeler().ApplyNodeLabels(context.TODO(), root, labeler)
	require.NoError(t, err)
	require.Equal(t, data, labeler.data)
}

func TestApplyNodeLabelsWithBlock(t *testing.T) {
	InitLabels(DefaultFabricLabelPrefix, DefaultAcceleratedLabelPrefix)
	root, _ := translate.GetBlockWithMultiIBTestSet()
	labeler := &testLabeler{data: make(map[string]map[string]string)}
	data := map[string]map[string]string{
		"Node104": {
			"accelerated.topology.nvidia.com/level-0": "B1",
			"network.topology.nvidia.com/level-0":     "S2",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node105": {
			"accelerated.topology.nvidia.com/level-0": "B1",
			"network.topology.nvidia.com/level-0":     "S2",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node106": {
			"accelerated.topology.nvidia.com/level-0": "B1",
			"network.topology.nvidia.com/level-0":     "S2",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node201": {
			"accelerated.topology.nvidia.com/level-0": "B2",
			"network.topology.nvidia.com/level-0":     "S3",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node202": {
			"accelerated.topology.nvidia.com/level-0": "B2",
			"network.topology.nvidia.com/level-0":     "S3",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node205": {
			"accelerated.topology.nvidia.com/level-0": "B2",
			"network.topology.nvidia.com/level-0":     "S3",
			"network.topology.nvidia.com/level-1":     "S1",
			"network.topology.nvidia.com/level-2":     "IB2",
		},
		"Node301": {
			"accelerated.topology.nvidia.com/level-0": "B3",
			"network.topology.nvidia.com/level-0":     "S5",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
		"Node302": {
			"accelerated.topology.nvidia.com/level-0": "B3",
			"network.topology.nvidia.com/level-0":     "S5",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
		"Node303": {
			"accelerated.topology.nvidia.com/level-0": "B3",
			"network.topology.nvidia.com/level-0":     "S5",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
		"Node401": {
			"accelerated.topology.nvidia.com/level-0": "B4",
			"network.topology.nvidia.com/level-0":     "S6",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
		"Node402": {
			"accelerated.topology.nvidia.com/level-0": "B4",
			"network.topology.nvidia.com/level-0":     "S6",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
		"Node403": {
			"accelerated.topology.nvidia.com/level-0": "B4",
			"network.topology.nvidia.com/level-0":     "S6",
			"network.topology.nvidia.com/level-1":     "S4",
			"network.topology.nvidia.com/level-2":     "IB1",
		},
	}

	err := NewTopologyLabeler().ApplyNodeLabels(context.TODO(), root, labeler)
	require.NoError(t, err)
	require.Equal(t, data, labeler.data)
}

func TestInitLabels(t *testing.T) {
	InitLabels("fabric-", "accelerated-")
	require.Equal(t, TopologyLabelKeys{
		Fabric:      "fabric-",
		Accelerated: "accelerated-",
	}, CurrentTopologyLabelKeys())
}

func TestBuildNodeLabelsWithVariableLevels(t *testing.T) {
	InitLabels(DefaultFabricLabelPrefix, DefaultAcceleratedLabelPrefix)
	node := &topology.Vertex{ID: "instance-1", Name: "node-1"}
	fabric0 := &topology.Vertex{ID: "fabric-0", Vertices: map[string]*topology.Vertex{node.ID: node}}
	fabric1 := &topology.Vertex{ID: "fabric-1", Vertices: map[string]*topology.Vertex{fabric0.ID: fabric0}}
	fabric2 := &topology.Vertex{ID: "fabric-2", Vertices: map[string]*topology.Vertex{fabric1.ID: fabric1}}
	fabric3 := &topology.Vertex{ID: "fabric-3", Vertices: map[string]*topology.Vertex{fabric2.ID: fabric2}}
	graph := &topology.Graph{
		Tiers: &topology.Vertex{Vertices: map[string]*topology.Vertex{fabric3.ID: fabric3}},
		AcceleratedTiers: []topology.DomainMap{
			{"accelerated-0": {"node-1": &topology.HostInfo{}}},
			{"accelerated-1": {"node-1": &topology.HostInfo{}}},
		},
	}

	labels, err := NewTopologyLabeler().BuildNodeLabels(graph)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		topology.FabricLevelKey(0):      "fabric-0",
		topology.FabricLevelKey(1):      "fabric-1",
		topology.FabricLevelKey(2):      "fabric-2",
		topology.FabricLevelKey(3):      "fabric-3",
		topology.AcceleratedLevelKey(0): "accelerated-0",
		topology.AcceleratedLevelKey(1): "accelerated-1",
	}, labels["node-1"])
}
