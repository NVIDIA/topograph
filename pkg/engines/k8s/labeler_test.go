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
	root, _ := translate.GetTreeTestSet(true)
	labeler := &testLabeler{data: make(map[string]map[string]string)}
	data := map[string]map[string]string{
		"Node201": {"network.topology.nvidia.com/block": "S2", "network.topology.nvidia.com/spine": "S1"},
		"Node202": {"network.topology.nvidia.com/block": "S2", "network.topology.nvidia.com/spine": "S1"},
		"Node205": {"network.topology.nvidia.com/block": "S2", "network.topology.nvidia.com/spine": "S1"},
		"Node304": {"network.topology.nvidia.com/block": "xf946c4acef2d5939", "network.topology.nvidia.com/spine": "S1"},
		"Node305": {"network.topology.nvidia.com/block": "xf946c4acef2d5939", "network.topology.nvidia.com/spine": "S1"},
		"Node306": {"network.topology.nvidia.com/block": "xf946c4acef2d5939", "network.topology.nvidia.com/spine": "S1"},
	}

	err := NewTopologyLabeler().ApplyNodeLabels(context.TODO(), root, labeler)
	require.NoError(t, err)
	require.Equal(t, data, labeler.data)
}

func TestApplyNodeLabelsWithBlock(t *testing.T) {
	root, _ := translate.GetBlockWithMultiIBTestSet()
	labeler := &testLabeler{data: make(map[string]map[string]string)}
	data := map[string]map[string]string{
		"Node104": {
			"network.topology.nvidia.com/accelerator": "B1",
			"network.topology.nvidia.com/block":       "S2",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node105": {
			"network.topology.nvidia.com/accelerator": "B1",
			"network.topology.nvidia.com/block":       "S2",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node106": {
			"network.topology.nvidia.com/accelerator": "B1",
			"network.topology.nvidia.com/block":       "S2",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node201": {
			"network.topology.nvidia.com/accelerator": "B2",
			"network.topology.nvidia.com/block":       "S3",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node202": {
			"network.topology.nvidia.com/accelerator": "B2",
			"network.topology.nvidia.com/block":       "S3",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node205": {
			"network.topology.nvidia.com/accelerator": "B2",
			"network.topology.nvidia.com/block":       "S3",
			"network.topology.nvidia.com/spine":       "S1",
			"network.topology.nvidia.com/datacenter":  "ibRoot2",
		},
		"Node301": {
			"network.topology.nvidia.com/accelerator": "B3",
			"network.topology.nvidia.com/block":       "S5",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
		"Node302": {
			"network.topology.nvidia.com/accelerator": "B3",
			"network.topology.nvidia.com/block":       "S5",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
		"Node303": {
			"network.topology.nvidia.com/accelerator": "B3",
			"network.topology.nvidia.com/block":       "S5",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
		"Node401": {
			"network.topology.nvidia.com/accelerator": "B4",
			"network.topology.nvidia.com/block":       "S6",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
		"Node402": {
			"network.topology.nvidia.com/accelerator": "B4",
			"network.topology.nvidia.com/block":       "S6",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
		"Node403": {
			"network.topology.nvidia.com/accelerator": "B4",
			"network.topology.nvidia.com/block":       "S6",
			"network.topology.nvidia.com/spine":       "S4",
			"network.topology.nvidia.com/datacenter":  "ibRoot1",
		},
	}

	err := NewTopologyLabeler().ApplyNodeLabels(context.TODO(), root, labeler)
	require.NoError(t, err)
	require.Equal(t, data, labeler.data)
}
