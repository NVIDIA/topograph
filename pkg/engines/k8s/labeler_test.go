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

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/translate"
	"github.com/stretchr/testify/require"
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

func TestApplyNodeLabels(t *testing.T) {
	root, _ := translate.GetTreeTestSet(true)
	labeler := &testLabeler{data: make(map[string]map[string]string)}
	data := map[string]map[string]string{
		"Node201": {"topology.kubernetes.io/network-level-1": "S2", "topology.kubernetes.io/network-level-2": "S1"},
		"Node202": {"topology.kubernetes.io/network-level-1": "S2", "topology.kubernetes.io/network-level-2": "S1"},
		"Node205": {"topology.kubernetes.io/network-level-1": "S2", "topology.kubernetes.io/network-level-2": "S1"},
		"Node304": {"topology.kubernetes.io/network-level-1": "xf946c4acef2d5939", "topology.kubernetes.io/network-level-2": "S1"},
		"Node305": {"topology.kubernetes.io/network-level-1": "xf946c4acef2d5939", "topology.kubernetes.io/network-level-2": "S1"},
		"Node306": {"topology.kubernetes.io/network-level-1": "xf946c4acef2d5939", "topology.kubernetes.io/network-level-2": "S1"},
	}

	err := NewTopologyLabeler().ApplyNodeLabels(context.TODO(), root.Vertices[common.ValTopologyTree], labeler)
	require.NoError(t, err)
	require.Equal(t, data, labeler.data)
}
