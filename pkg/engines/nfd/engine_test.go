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

package nfd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	k8sengine "github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestNamedLoader(t *testing.T) {
	name, loader := NamedLoader()
	require.Equal(t, NAME, name)
	require.NotNil(t, loader)
}

func TestGenerateOutputCreatesNodeFeaturesAndGroups(t *testing.T) {
	k8sengine.InitLabels(
		k8sengine.DefaultLabelAccelerator,
		k8sengine.DefaultLabelLeaf,
		k8sengine.DefaultLabelSpine,
		k8sengine.DefaultLabelCore,
	)

	client := k8sfake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: "node-b",
			Labels: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster.0",
			},
		}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-c"}},
	)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			nodeFeatureGVR:      "NodeFeatureList",
			nodeFeatureGroupGVR: "NodeFeatureGroupList",
		},
	)
	eng := &NfdEngine{
		client:        client,
		dynamicClient: dynamicClient,
		params:        &Params{Cleanup: true},
	}

	out, httpErr := eng.GenerateOutput(context.Background(), testGraph(), nil)
	require.Nil(t, httpErr)
	require.Equal(t, "OK nodeFeatures=3 nodeFeatureGroups=6\n", string(out))

	features, err := dynamicClient.Resource(nodeFeatureGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, features.Items, 3)

	nodeA := findNodeFeature(t, features.Items, "node-a")
	require.Equal(t, map[string]string{nfdNodeName: "node-a"}, attributeElements(t, nodeA, nfdSystemName))
	require.Equal(t, map[string]string{
		topologyTypeAccelerator: "nvl-a",
		topologyTypeLeaf:        "leaf-1",
		topologyTypeSpine:       "spine-1",
	}, attributeElements(t, nodeA, nfdFeatureSet))

	nodeB := findNodeFeature(t, features.Items, "node-b")
	require.Equal(t, map[string]string{nfdNodeName: "node-b"}, attributeElements(t, nodeB, nfdSystemName))
	require.Equal(t, map[string]string{
		topologyTypeAccelerator: "cluster.0",
		topologyTypeLeaf:        "leaf-1",
		topologyTypeSpine:       "spine-1",
	}, attributeElements(t, nodeB, nfdFeatureSet))

	groups, err := dynamicClient.Resource(nodeFeatureGroupGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, groups.Items, 6)

	leafGroup := findGroup(t, groups.Items, topologyTypeLeaf, "leaf-1")
	require.Equal(t, []interface{}{"leaf-1"}, groupRuleValues(t, leafGroup, topologyTypeLeaf))
	cliqueGroup := findGroup(t, groups.Items, topologyTypeAccelerator, "cluster.0")
	require.Equal(t, topology.KeyNvidiaGPUClique, cliqueGroup.GetAnnotations()[annotationTopologyLabelKey])
	require.Equal(t, []interface{}{"cluster.0"}, groupRuleValues(t, cliqueGroup, topologyTypeAccelerator))
}

func TestGenerateOutputCleansStaleObjects(t *testing.T) {
	k8sengine.InitLabels(
		k8sengine.DefaultLabelAccelerator,
		k8sengine.DefaultLabelLeaf,
		k8sengine.DefaultLabelSpine,
		k8sengine.DefaultLabelCore,
	)

	staleFeature, err := makeNodeFeature("stale-node", map[string]string{topologyTypeLeaf: "stale-leaf"})
	require.NoError(t, err)
	staleGroup, err := makeNodeFeatureGroup(topologyTypeLeaf, "stale-leaf", k8sengine.DefaultLabelLeaf)
	require.NoError(t, err)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			nodeFeatureGVR:      "NodeFeatureList",
			nodeFeatureGroupGVR: "NodeFeatureGroupList",
		},
		staleFeature,
		staleGroup,
	)
	eng := &NfdEngine{
		client:        k8sfake.NewSimpleClientset(),
		dynamicClient: dynamicClient,
		params:        &Params{Cleanup: true},
	}

	out, httpErr := eng.GenerateOutput(context.Background(), nil, nil)
	require.Nil(t, httpErr)
	require.Equal(t, "OK nodeFeatures=0 nodeFeatureGroups=0\n", string(out))

	features, err := dynamicClient.Resource(nodeFeatureGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, features.Items)
	groups, err := dynamicClient.Resource(nodeFeatureGroupGVR).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, groups.Items)
}

func TestBuildNFDObjectsRejectsInvalidNFDNodeNameLabelValue(t *testing.T) {
	k8sengine.InitLabels(
		k8sengine.DefaultLabelAccelerator,
		k8sengine.DefaultLabelLeaf,
		k8sengine.DefaultLabelSpine,
		k8sengine.DefaultLabelCore,
	)

	nodeLabels := k8sengine.NodeLabelMap{
		"node-name-that-is-too-long-for-a-kubernetes-label-value-because-it-has-more-than-sixty-three-characters": {
			k8sengine.DefaultLabelLeaf: "leaf-1",
		},
	}

	_, _, err := buildNFDObjects(nodeLabels, nil)
	require.ErrorContains(t, err, "cannot be used as")
}

func testGraph() *topology.Graph {
	domains := topology.NewDomainMap()
	domains.AddHost("nvl-a", "inst-a", "node-a")
	domains.AddHost("nvl-a", "inst-b", "node-b")
	domains.AddHost("nvl-b", "inst-c", "node-c")

	leaf1 := &topology.Vertex{
		ID: "leaf-1",
		Vertices: map[string]*topology.Vertex{
			"inst-a": {ID: "inst-a", Name: "node-a"},
			"inst-b": {ID: "inst-b", Name: "node-b"},
		},
	}
	leaf2 := &topology.Vertex{
		ID: "leaf-2",
		Vertices: map[string]*topology.Vertex{
			"inst-c": {ID: "inst-c", Name: "node-c"},
		},
	}
	spine := &topology.Vertex{
		ID: "spine-1",
		Vertices: map[string]*topology.Vertex{
			"leaf-1": leaf1,
			"leaf-2": leaf2,
		},
	}

	return &topology.Graph{
		Tiers: &topology.Vertex{
			Vertices: map[string]*topology.Vertex{
				"spine-1": spine,
			},
		},
		Domains: domains,
	}
}

func findNodeFeature(t *testing.T, items []unstructured.Unstructured, nodeName string) unstructured.Unstructured {
	t.Helper()
	for _, item := range items {
		if item.GetAnnotations()[annotationNodeName] == nodeName {
			return item
		}
	}
	require.FailNowf(t, "missing NodeFeature", "node %q not found", nodeName)
	return unstructured.Unstructured{}
}

func findGroup(t *testing.T, items []unstructured.Unstructured, kind, value string) unstructured.Unstructured {
	t.Helper()
	for _, item := range items {
		annotations := item.GetAnnotations()
		if item.GetLabels()[labelGroupType] == kind && annotations[annotationTopologyValue] == value {
			return item
		}
	}
	require.FailNowf(t, "missing NodeFeatureGroup", "kind %q value %q not found", kind, value)
	return unstructured.Unstructured{}
}

func attributeElements(t *testing.T, item unstructured.Unstructured, feature string) map[string]string {
	t.Helper()
	elements, found, err := unstructured.NestedStringMap(item.Object, "spec", "features", "attributes", feature, "elements")
	require.NoError(t, err)
	require.True(t, found)
	return elements
}

func groupRuleValues(t *testing.T, item unstructured.Unstructured, kind string) []interface{} {
	t.Helper()
	spec, ok := item.Object["spec"].(map[string]interface{})
	require.True(t, ok)
	rules, ok := spec["featureGroupRules"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, rules)
	rule, ok := rules[0].(map[string]interface{})
	require.True(t, ok)
	matchFeatures, ok := rule["matchFeatures"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, matchFeatures)
	matcher, ok := matchFeatures[0].(map[string]interface{})
	require.True(t, ok)
	matchExpressions, ok := matcher["matchExpressions"].(map[string]interface{})
	require.True(t, ok)
	expression, ok := matchExpressions[kind].(map[string]interface{})
	require.True(t, ok)
	values, ok := expression["value"].([]interface{})
	require.True(t, ok)
	return values
}
