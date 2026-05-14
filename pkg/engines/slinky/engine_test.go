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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestGetParameters(t *testing.T) {
	podSelector := map[string]any{
		"matchLabels": map[string]string{"key": "value"},
	}
	nodeSelector := map[string]string{"key": "value"}
	invalidSelector := map[string]any{
		"matchExpressions": []metav1.LabelSelectorRequirement{
			{Operator: "BAD"},
		},
	}
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
	}

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
			name: "Case 3: bad label selector",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       "BAD",
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			err: `could not decode configuration:`,
		},
		{
			name: "Case 4: invalid pod label selector",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       invalidSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			err: `"BAD" is not a valid label selector operator`,
		},
		{
			name: "Case 5: nil topology",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
				topology.KeyTopologies:        map[string]any{"topo": nil},
			},
			err: `topology "topo": nil entry`,
		},
		{
			name: "Case 6: invalid topology",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
				topology.KeyTopologies: map[string]any{
					"topo": map[string]any{
						"plugin":      topology.TopologyBlock,
						"blockSizes":  []int{16, 32},
						"nodes":       []string{"node1", "node2"},
						"podSelector": podSelector,
					},
				},
			},
			err: `topology "topo": cannot set both nodes and podSelector`,
		},
		{
			name: "Case 7: minimal valid input",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			ret: &Params{
				Namespace:     "namespace",
				PodSelector:   labelSelector,
				ConfigPath:    "path",
				ConfigMapName: "name",
				podListOpt:    &metav1.ListOptions{LabelSelector: "key=value"},
			},
		},
		{
			name: "Case 8: cluster-wide valid parameters",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyNodeSelector:      nodeSelector,
				topology.KeyPlugin:            topology.TopologyBlock,
				topology.KeyBlockSizes:        []int{16},
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
			},
			ret: &Params{
				BaseParams: slurm.BaseParams{
					Plugin:     topology.TopologyBlock,
					BlockSizes: []int{16},
				},
				Namespace:     "namespace",
				PodSelector:   labelSelector,
				NodeSelector:  nodeSelector,
				ConfigPath:    "path",
				ConfigMapName: "name",
				podListOpt:    &metav1.ListOptions{LabelSelector: "key=value"},
				nodeListOpt:   &metav1.ListOptions{LabelSelector: "key=value"},
			},
		},
		{
			name: "Case 9: per-partition valid parameters",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyNodeSelector:      nodeSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
				topology.KeyTopologies: map[string]any{
					"topo1": map[string]any{
						"plugin":     topology.TopologyBlock,
						"blockSizes": []int{16, 32},
						"nodes":      []string{"node1", "node2"},
					},
					"topo2": map[string]any{
						topology.KeyPlugin: topology.TopologyTree,
						"podSelector":      podSelector,
					},
				},
			},
			ret: &Params{
				Namespace:     "namespace",
				PodSelector:   labelSelector,
				NodeSelector:  nodeSelector,
				ConfigPath:    "path",
				ConfigMapName: "name",
				Topologies: map[string]*Topology{
					"topo1": {
						Topology: slurm.Topology{
							Plugin:     topology.TopologyBlock,
							BlockSizes: []int{16, 32},
							Nodes:      []string{"node1", "node2"},
						},
					},
					"topo2": {
						Topology: slurm.Topology{
							Plugin: topology.TopologyTree,
						},
						PodSelector: labelSelector,
					},
				},
				podListOpt:  &metav1.ListOptions{LabelSelector: "key=value"},
				nodeListOpt: &metav1.ListOptions{LabelSelector: "key=value"},
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
			cis: []topology.ComputeInstances{
				{
					Region:    "r1",
					Instances: map[string]string{"i1": "node1"},
				},
			},
		},
		{
			name:  "Case 2: region error",
			nodes: &corev1.NodeList{Items: []corev1.Node{nodeErr2, node2}},
			cis: []topology.ComputeInstances{
				{
					Region:    "r1",
					Instances: map[string]string{"i2": "node2"},
				},
			},
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
				require.Nil(t, err)
				require.Equal(t, tc.cis, cis)
			}
		})
	}
}

// Helper for annotation checks
func requireAnnotation(t *testing.T, annotations map[string]string, key, expected string) {
	val, ok := annotations[key]
	require.True(t, ok, "annotation %s should exist", key)
	require.Equal(t, expected, val, "annotation %s should have correct value", key)
}

func TestConfigMapAnnotationsAndMetadata(t *testing.T) {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{"app.kubernetes.io/component": "compute"},
	}
	testCases := []struct {
		name       string
		params     *Params
		wantPlugin bool
		wantBlock  bool
	}{
		{
			name: "minimal params, no plugin/block",
			params: &Params{
				Namespace:     "test-namespace",
				PodSelector:   labelSelector,
				ConfigPath:    "topology.conf",
				ConfigMapName: "slurm-topology",
			},
			wantPlugin: false, wantBlock: false,
		},
		{
			name: "with plugin only",
			params: &Params{Namespace: "test-namespace",
				BaseParams: slurm.BaseParams{
					Plugin: topology.TopologyBlock,
				},
				PodSelector:   labelSelector,
				ConfigPath:    "topology.conf",
				ConfigMapName: "slurm-topology",
			},
			wantPlugin: true, wantBlock: false,
		},
		{
			name: "with block sizes only",
			params: &Params{
				BaseParams: slurm.BaseParams{
					BlockSizes: []int{8, 16, 32},
				},
				Namespace:     "test-namespace",
				PodSelector:   labelSelector,
				ConfigPath:    "topology.conf",
				ConfigMapName: "slurm-topology",
			},
			wantPlugin: false, wantBlock: true,
		},
		{
			name: "with plugin and block sizes",
			params: &Params{
				BaseParams: slurm.BaseParams{
					Plugin:     topology.TopologyBlock,
					BlockSizes: []int{8, 16, 32},
				},
				Namespace:     "test-namespace",
				PodSelector:   labelSelector,
				ConfigPath:    "topology.conf",
				ConfigMapName: "slurm-topology",
			},
			wantPlugin: true, wantBlock: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &SlinkyEngine{params: tc.params}
			annotations := engine.generateConfigMapAnnotations()

			// Required annotation checks
			requireAnnotation(t, annotations, topology.KeyConfigMapEngine, NAME)
			requireAnnotation(t, annotations, topology.KeyConfigMapTopologyManagedBy, "topograph")
			requireAnnotation(t, annotations, topology.KeyConfigMapNamespace, tc.params.Namespace)
			timestamp, ok := annotations[topology.KeyConfigMapLastUpdated]
			require.True(t, ok)
			_, err := time.Parse(time.RFC3339, timestamp)
			require.NoError(t, err)

			if tc.wantPlugin {
				requireAnnotation(t, annotations, topology.KeyConfigMapPlugin, tc.params.Plugin)
			} else {
				require.NotContains(t, annotations, topology.KeyConfigMapPlugin)
			}
			if tc.wantBlock {
				requireAnnotation(t, annotations, topology.KeyConfigMapBlockSizes, intToStr(tc.params.BlockSizes))
			} else {
				require.NotContains(t, annotations, topology.KeyConfigMapBlockSizes)
			}
		})
	}
}

const (
	//medium.yaml - tree topology skeleton
	mediumTreeTopologyYamlSkeleton = `- topology: topo-0
  cluster_default: false
  tree:
    switches:
        - switch: sw3
          children: sw[21-22]
        - switch: sw21
          children: sw11
        - switch: sw22
          children: sw14
        - switch: sw11
        - switch: sw14
`
	//medium.yaml - full tree topology
	mediumTreeTopologyYamlFull = `- topology: topo-0
  cluster_default: false
  tree:
    switches:
        - switch: sw3
          children: sw[21-22]
        - switch: sw21
          children: sw11
        - switch: sw22
          children: sw14
        - switch: sw11
          nodes: "1101"
        - switch: sw14
          nodes: "1402"
`
	//medium.yaml - block topology skeleton
	mediumBlockTopologyYamlSkeleton = `- topology: topo-0
  cluster_default: false
  block:
    block_sizes:
        - 1
        - 2
    blocks:
        - block: block1
        - block: block2
`
	//medium.yaml - full block topology
	mediumBlockTopologyYamlFull = `- topology: topo-0
  cluster_default: false
  block:
    block_sizes:
        - 1
        - 2
    blocks:
        - block: block1
          nodes: "1101"
        - block: block2
          nodes: "1301"
`
	//medium.yaml - combined topology skeleton
	mediumCombinedTopologyYamlSkeleton = `- topology: topo-0
  cluster_default: false
  tree:
    switches:
        - switch: sw3
          children: sw[21-22]
        - switch: sw21
          children: sw11
        - switch: sw22
          children: sw13
        - switch: sw11
        - switch: sw13
- topology: topo-1
  cluster_default: false
  block:
    block_sizes:
        - 1
        - 2
    blocks:
        - block: block1
        - block: block2
`
	//medium.yaml - combined topology full
	mediumCombinedTopologyYamlFull = `- topology: topo-0
  cluster_default: false
  tree:
    switches:
        - switch: sw3
          children: sw[21-22]
        - switch: sw21
          children: sw11
        - switch: sw22
          children: sw13
        - switch: sw11
          nodes: "1101"
        - switch: sw13
          nodes: "1302"
- topology: topo-1
  cluster_default: false
  block:
    block_sizes:
        - 1
        - 2
    blocks:
        - block: block1
          nodes: "1101"
        - block: block2
          nodes: "1302"
`
	noUpdateConfigMap = `existing: topology`
)

// slurmTopologiesForDynamicTest builds per-partition slurm.Topology entries for BaseParams.Topologies.
// Each entry includes podSelector under Other (seeRemain) for getPartitionNodes, matching engine decoding in getPartitionNodes.
func slurmTopologiesForDynamicTest(plugins []string) map[string]*Topology {
	out := make(map[string]*Topology, len(plugins))
	for i, plugin := range plugins {
		key := fmt.Sprintf("topo-%d", i)
		out[key] = &Topology{
			Topology: slurm.Topology{
				Plugin:    plugin,
				Partition: key,
			},
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "slinky"}},
		}
	}
	return out
}

func TestGenerateDynamicNodesOutput(t *testing.T) {
	slinkyPodSel := metav1.LabelSelector{MatchLabels: map[string]string{"app": "slinky"}}

	fakeSuccessClient := func(slurmNames []string, createConfigMap bool) *fake.Clientset {
		client := fake.NewSimpleClientset()
		for i, slurmName := range slurmNames {
			// Add nodes
			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("k8s-node-%d", i),
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			}
			_, err := client.CoreV1().Nodes().Create(context.Background(), node1, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add pods
			pod1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("k8s-pod-%d", i),
					Namespace: "test-ns",
					Labels: map[string]string{
						"app":             "slinky",
						"slurm.node.name": slurmName,
					},
				},
				Spec: corev1.PodSpec{
					NodeName: fmt.Sprintf("k8s-node-%d", i),
					Containers: []corev1.Container{
						{Name: "test", Image: "test"},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,

					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			_, err = client.CoreV1().Pods("test-ns").Create(context.Background(), pod1, metav1.CreateOptions{})
			require.NoError(t, err)
		}
		// Add config map
		if createConfigMap {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "slurm-config",
					Namespace: "test-ns",
				},
				Data: map[string]string{
					"topology.yaml": "existing: topology",
				},
			}
			_, err := client.CoreV1().ConfigMaps("test-ns").Create(context.Background(), cm, metav1.CreateOptions{})
			require.NoError(t, err)
		}

		return client
	}

	testCases := []struct {
		name                  string
		k8sClient             func([]string, bool) *fake.Clientset
		createConfigMap       bool
		topologyFile          string
		topologyConfig        []string
		slurmName             []string
		slurmConfigUpdateMode string
		expectTopologyYaml    string
		expectTopologySpec    []string
		expectError           bool
		errorMsg              string
	}{
		{
			name:                  "successful dynamic nodes for tree topology with skeleton only update",
			k8sClient:             fakeSuccessClient,
			createConfigMap:       true,
			topologyFile:          "medium.yaml",
			topologyConfig:        []string{topology.TopologyTree},
			slurmName:             []string{"1101", "1402"},
			slurmConfigUpdateMode: "skeleton-only",
			expectTopologyYaml:    mediumTreeTopologyYamlSkeleton,
			expectTopologySpec:    []string{"topo-0:sw3:sw21:sw11", "topo-0:sw3:sw22:sw14"},
			expectError:           false,
		},
		{
			name:               "successful dynamic nodes for tree topology with full update",
			k8sClient:          fakeSuccessClient,
			createConfigMap:    true,
			topologyFile:       "medium.yaml",
			topologyConfig:     []string{topology.TopologyTree},
			slurmName:          []string{"1101", "1402"},
			expectTopologyYaml: mediumTreeTopologyYamlFull,
			expectTopologySpec: []string{"topo-0:sw3:sw21:sw11", "topo-0:sw3:sw22:sw14"},
			expectError:        false,
		},
		{
			name:                  "successful dynamic nodes for tree topology with no update",
			k8sClient:             fakeSuccessClient,
			createConfigMap:       true,
			topologyFile:          "medium.yaml",
			topologyConfig:        []string{topology.TopologyTree},
			slurmName:             []string{"1101", "1402"},
			slurmConfigUpdateMode: "none",
			expectTopologyYaml:    noUpdateConfigMap,
			expectTopologySpec:    []string{"topo-0:sw3:sw21:sw11", "topo-0:sw3:sw22:sw14"},
			expectError:           false,
		},
		{
			name:                  "successful dynamic nodes for block topology with skeleton only update",
			k8sClient:             fakeSuccessClient,
			createConfigMap:       true,
			topologyFile:          "medium.yaml",
			topologyConfig:        []string{topology.TopologyBlock},
			slurmName:             []string{"1101", "1301"},
			slurmConfigUpdateMode: "skeleton-only",
			expectTopologyYaml:    mediumBlockTopologyYamlSkeleton,
			expectTopologySpec:    []string{"topo-0:block1", "topo-0:block2"},
			expectError:           false,
		},
		{
			name:               "successful dynamic nodes for block topology with full update",
			k8sClient:          fakeSuccessClient,
			createConfigMap:    true,
			topologyFile:       "medium.yaml",
			topologyConfig:     []string{topology.TopologyBlock},
			slurmName:          []string{"1101", "1301"},
			expectTopologyYaml: mediumBlockTopologyYamlFull,
			expectTopologySpec: []string{"topo-0:block1", "topo-0:block2"},
			expectError:        false,
		},
		{
			name:                  "successful dynamic nodes for block topology with no update",
			k8sClient:             fakeSuccessClient,
			createConfigMap:       true,
			topologyFile:          "medium.yaml",
			topologyConfig:        []string{topology.TopologyBlock},
			slurmName:             []string{"1101", "1301"},
			slurmConfigUpdateMode: "none",
			expectTopologyYaml:    noUpdateConfigMap,
			expectTopologySpec:    []string{"topo-0:block1", "topo-0:block2"},
			expectError:           false,
		},
		{
			name:                  "successful dynamic nodes for combined topology with skeleton only update",
			k8sClient:             fakeSuccessClient,
			createConfigMap:       false,
			topologyFile:          "medium.yaml",
			topologyConfig:        []string{topology.TopologyTree, topology.TopologyBlock},
			slurmName:             []string{"1101", "1302"},
			slurmConfigUpdateMode: "skeleton-only",
			expectTopologyYaml:    mediumCombinedTopologyYamlSkeleton,
			expectTopologySpec:    []string{"topo-0:sw3:sw21:sw11,topo-1:block1", "topo-0:sw3:sw22:sw13,topo-1:block2"},
			expectError:           false,
		},
		{
			name:               "successful dynamic nodes for combined topology with full update",
			k8sClient:          fakeSuccessClient,
			createConfigMap:    false,
			topologyFile:       "medium.yaml",
			topologyConfig:     []string{topology.TopologyTree, topology.TopologyBlock},
			slurmName:          []string{"1101", "1302"},
			expectTopologyYaml: mediumCombinedTopologyYamlFull,
			expectTopologySpec: []string{"topo-0:sw3:sw21:sw11,topo-1:block1", "topo-0:sw3:sw22:sw13,topo-1:block2"},
			expectError:        false,
		},
		{
			name: "error getting pods",
			k8sClient: func([]string, bool) *fake.Clientset {
				client := fake.NewSimpleClientset()
				client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.NewInternalError(fmt.Errorf("failed to list pods"))
				})
				return client
			},
			topologyFile:   "medium.yaml",
			topologyConfig: []string{topology.TopologyTree},
			expectError:    true,
			errorMsg:       `topology "topo-0": failed to list pods with selector "app=slinky": Internal error occurred: failed to list pods`,
		},
		{
			name: "error getting config map",
			k8sClient: func(_ []string, _ bool) *fake.Clientset {
				client := fakeSuccessClient([]string{"1101", "1402"}, true)
				client.PrependReactor("get", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.NewInternalError(fmt.Errorf("failed to get config map"))
				})
				return client
			},
			topologyFile:   "medium.yaml",
			topologyConfig: []string{topology.TopologyTree},
			expectError:    true,
			errorMsg:       "failed to get config map",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.k8sClient(tc.slurmName, tc.createConfigMap)

			model, err := models.NewModelFromFile(tc.topologyFile)
			require.NoError(t, err)
			topo, _ := model.ToGraph()

			podListSel, err := metav1.LabelSelectorAsSelector(&slinkyPodSel)
			require.NoError(t, err)
			podListOpt := &metav1.ListOptions{LabelSelector: podListSel.String()}

			params := &Params{
				Namespace:        "test-ns",
				ConfigMapName:    "slurm-config",
				ConfigPath:       "topology.yaml",
				PodSelector:      slinkyPodSel,
				UseDynamicNodes:  true,
				podListOpt:       podListOpt,
				nodeListOpt:      &metav1.ListOptions{},
				ConfigUpdateMode: tc.slurmConfigUpdateMode,
				Topologies:       slurmTopologiesForDynamicTest(tc.topologyConfig),
			}
			engine := &SlinkyEngine{
				client: client,
				params: params,
			}

			result, httpErr := engine.GenerateOutput(context.Background(), topo, nil)

			if tc.expectError {
				require.Error(t, httpErr)
				if tc.errorMsg != "" {
					require.Contains(t, httpErr.Error(), tc.errorMsg)
				}
				return
			}
			require.Nil(t, httpErr)

			cm, err := client.CoreV1().ConfigMaps(params.Namespace).Get(context.Background(), params.ConfigMapName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.expectTopologyYaml, cm.Data[params.ConfigPath])

			for i, topoSpec := range tc.expectTopologySpec {
				updatedNode, err := client.CoreV1().Nodes().Get(context.Background(), fmt.Sprintf("k8s-node-%d", i), metav1.GetOptions{})
				require.NoError(t, err)
				requireAnnotation(t, updatedNode.Annotations, topology.KeySlinkyTopologySpec, topoSpec)
				require.Equal(t, []byte("OK\n"), result)
			}

		})
	}
}

func TestResolveTopologies(t *testing.T) {
	makePod := func(name, slurmName, partition string, ready bool) *corev1.Pod {
		status := corev1.ConditionTrue
		if !ready {
			status = corev1.ConditionFalse
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "test-ns",
				Labels: map[string]string{
					"partition":               partition,
					topology.KeySlurmNodeName: slurmName,
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "i"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: status},
				},
			},
		}
	}

	ctx := context.Background()
	client := fake.NewSimpleClientset()
	for _, p := range []*corev1.Pod{
		makePod("p1", "node1", "a", true),
		makePod("p2", "node2", "a", true),
		makePod("p3", "node3", "a", false), // not ready, must be skipped
		makePod("p4", "node4", "b", true),
	} {
		_, err := client.CoreV1().Pods("test-ns").Create(ctx, p, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	selA := metav1.LabelSelector{MatchLabels: map[string]string{"partition": "a"}}
	selB := metav1.LabelSelector{MatchLabels: map[string]string{"partition": "b"}}

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace: "test-ns",
			Topologies: map[string]*Topology{
				"byNodes":     {Topology: slurm.Topology{Plugin: topology.TopologyTree, Nodes: []string{"n1", "n2"}}},
				"bySelectorA": {Topology: slurm.Topology{Plugin: topology.TopologyBlock}, PodSelector: selA},
				"bySelectorB": {Topology: slurm.Topology{Plugin: topology.TopologyTree}, PodSelector: selB},
				"fallback":    {Topology: slurm.Topology{Plugin: topology.TopologyFlat, Partition: "scontrol-partition"}},
			},
		},
	}

	got, err := eng.resolveTopologies(ctx)
	require.NoError(t, err)
	require.Len(t, got, 4)

	require.Equal(t, []string{"n1", "n2"}, got["byNodes"].Nodes)
	require.ElementsMatch(t, []string{"node1", "node2"}, got["bySelectorA"].Nodes)
	require.Equal(t, []string{"node4"}, got["bySelectorB"].Nodes)
	// fallback entry: Nodes empty so slurm.GetTranslateConfig falls back to the finder
	require.Empty(t, got["fallback"].Nodes)
	require.Equal(t, "scontrol-partition", got["fallback"].Partition)
}

func TestGetParametersTopologyValidation(t *testing.T) {
	testCases := []struct {
		name  string
		nodes any
	}{
		{
			name:  "non-empty nodes and pod selector",
			nodes: []string{"n1"},
		},
		{
			name:  "empty nodes and pod selector",
			nodes: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{
				topology.KeyNamespace:         "test-ns",
				topology.KeyPodSelector:       map[string]any{"matchLabels": map[string]string{"app": "slurm"}},
				topology.KeyTopoConfigPath:    "topology.conf",
				topology.KeyTopoConfigmapName: "slurm-config",
				"topologies": map[string]any{
					"bad": map[string]any{
						"plugin": topology.TopologyTree,
						"nodes":  tc.nodes,
						"podSelector": map[string]any{
							"matchLabels": map[string]string{"partition": "a"},
						},
					},
				},
			}

			_, err := getParameters(params)
			require.ErrorContains(t, err, `cannot set both nodes and podSelector`)
		})
	}
}
