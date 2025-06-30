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

package slinky

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestGetParameters(t *testing.T) {
	testCases := []struct {
		name   string
		params map[string]any
		ret    *Params
		err    string
	}{
		{
			name: "Case 1: no params",
			err:  `must specify engine parameter "`,
		},
		{
			name: "Case 2: missing key",
			params: map[string]any{
				topology.KeyTopoConfigmapName: "name",
				topology.KeyNamespace:         "namespace",
			},
			err: `must specify engine parameter "`,
		},
		{
			name: "Case 3: minimal valid input",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodLabel:          "app.kubernetes.io/component=compute",
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			ret: &Params{
				Namespace:     "namespace",
				PodLabel:      "app.kubernetes.io/component=compute",
				ConfigPath:    "path",
				ConfigMapName: "name",
			},
		},
		{
			name: "Case 4: complete valid input",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodLabel:          "app.kubernetes.io/component=compute",
				topology.KeyPlugin:            topology.TopologyBlock,
				topology.KeyBlockSizes:        "16",
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			ret: &Params{
				Namespace:     "namespace",
				PodLabel:      "app.kubernetes.io/component=compute",
				Plugin:        topology.TopologyBlock,
				BlockSizes:    "16",
				ConfigPath:    "path",
				ConfigMapName: "name",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := getParameters(tc.params)
			if len(tc.err) != 0 {
				require.ErrorContains(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.ret, p)
			}
		})
	}
}

func TestGetComputeInstances(t *testing.T) {
	nodeErr1 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "err1"}}
	nodeErr2 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "err2", Annotations: map[string]string{topology.KeyNodeInstance: "instance"}}}
	node1 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "host1", Annotations: map[string]string{topology.KeyNodeInstance: "i1", topology.KeyNodeRegion: "r1"}}}
	node2 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "host2", Annotations: map[string]string{topology.KeyNodeInstance: "i2", topology.KeyNodeRegion: "r1"}}}
	node3 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "host3", Annotations: map[string]string{topology.KeyNodeInstance: "i3", topology.KeyNodeRegion: "r2"}}}
	nodeNone := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "none"}}
	nodeMap := map[string]string{"host1": "node1", "host2": "node2", "host3": "node3", "err1": "node1", "err2": "node2"}

	testCases := []struct {
		name  string
		nodes *corev1.NodeList
		cis   []topology.ComputeInstances
		err   string
	}{
		{
			name:  "Case 1: instance error",
			nodes: &corev1.NodeList{Items: []corev1.Node{node1, nodeErr1}},
			err:   `missing "topograph.nvidia.com/instance" annotation in node err1`,
		},
		{
			name:  "Case 2: region error",
			nodes: &corev1.NodeList{Items: []corev1.Node{nodeErr2, node2}},
			err:   `missing "topograph.nvidia.com/region" annotation in node err2`,
		},
		{
			name:  "Case 3: valid input",
			nodes: &corev1.NodeList{Items: []corev1.Node{node1, node2, node3, nodeNone}},
			cis: []topology.ComputeInstances{
				{
					Region:    "r1",
					Instances: map[string]string{"i1": "node1", "i2": "node2"},
				},
				{
					Region:    "r2",
					Instances: map[string]string{"i3": "node3"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cis, err := getComputeInstances(tc.nodes, nodeMap)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.cis, cis)
			}
		})
	}
}
