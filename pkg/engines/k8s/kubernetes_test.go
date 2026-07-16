/*
 * Copyright 2025-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/NVIDIA/topograph/pkg/topology"
)

var nodeGVR = corev1.SchemeGroupVersion.WithResource("nodes")

func TestGetComputeInstances(t *testing.T) {
	nodeErr1 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "err1"}}
	nodeErr2 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "err2", Annotations: map[string]string{topology.KeyNodeInstance: "instance"}}}
	node1 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Annotations: map[string]string{topology.KeyNodeInstance: "i1", topology.KeyNodeRegion: "r1"}}}
	node2 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2", Annotations: map[string]string{topology.KeyNodeInstance: "i2", topology.KeyNodeRegion: "r1"}}}
	node3 := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node3", Annotations: map[string]string{topology.KeyNodeInstance: "i3", topology.KeyNodeRegion: "r2"}}}

	testCases := []struct {
		name  string
		nodes *corev1.NodeList
		cis   []topology.ComputeInstances
	}{
		{
			name:  "Case 1: missing instance",
			nodes: &corev1.NodeList{Items: []corev1.Node{node1, nodeErr1}},
			cis: []topology.ComputeInstances{
				{Region: "r1", Instances: map[string]string{"i1": "node1"}},
			},
		},
		{
			name:  "Case 2: missing region",
			nodes: &corev1.NodeList{Items: []corev1.Node{nodeErr2, node2}},
			cis: []topology.ComputeInstances{
				{Region: "r1", Instances: map[string]string{"i2": "node2"}},
			},
		},
		{
			name:  "Case 3: valid input",
			nodes: &corev1.NodeList{Items: []corev1.Node{node1, node2, node3}},
			cis: []topology.ComputeInstances{
				{Region: "r1", Instances: map[string]string{"i1": "node1", "i2": "node2"}},
				{Region: "r2", Instances: map[string]string{"i3": "node3"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cis := getComputeInstances(tc.nodes)
			require.Equal(t, tc.cis, cis)
		})
	}
}

func TestGenerateOutputReconcilesTierDepthChanges(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	testCases := []struct {
		name       string
		initial    map[string]string
		tiers      []string
		wantLabels map[string]string
	}{
		{
			name: "three tiers to two",
			initial: map[string]string{
				DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
			tiers: []string{"leaf-a", "spine-a"},
			wantLabels: map[string]string{
				DefaultLabelLeaf: "leaf-a", DefaultLabelSpine: "spine-a",
			},
		},
		{
			name: "two tiers to one",
			initial: map[string]string{
				DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "stale-core",
			},
			tiers: []string{"leaf-a"},
			wantLabels: map[string]string{
				DefaultLabelLeaf: "leaf-a",
			},
		},
		{
			name: "one tier to three",
			initial: map[string]string{
				DefaultLabelLeaf: "old-leaf",
			},
			tiers: []string{"leaf-a", "spine-a", "core-a"},
			wantLabels: map[string]string{
				DefaultLabelLeaf: "leaf-a", DefaultLabelSpine: "spine-a", DefaultLabelCore: "core-a",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := newFakeClient(testNode("node-a", tc.initial))
			generateOutputSuccessfully(t, testEngine(client), graphWithTiers(map[string][]string{"node-a": tc.tiers}))

			require.Equal(t, tc.wantLabels, trackedNode(t, client, "node-a").Labels)
			assertNodeActionCounts(t, client, 1, 0, 1)
		})
	}
}

func TestGenerateOutputUpdatesChangedTierValues(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{
		DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
	}))

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf", "new-spine", "new-core"}}),
	)

	require.Equal(t, map[string]string{
		DefaultLabelLeaf: "new-leaf", DefaultLabelSpine: "new-spine", DefaultLabelCore: "new-core",
	}, trackedNode(t, client, "node-a").Labels)
	assertNodeActionCounts(t, client, 1, 0, 1)
}

func TestGenerateOutputNoOpUsesOnlyOneList(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{
		DefaultLabelLeaf: "leaf-a", DefaultLabelSpine: "spine-a", "workload.example/label": "keep",
	}))

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"leaf-a", "spine-a"}}),
	)

	assertNodeActionCounts(t, client, 1, 0, 0)
}

func TestGenerateOutputOnlyUpdatesChangedNodes(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(
		testNode("node-a", map[string]string{DefaultLabelLeaf: "leaf-a"}),
		testNode("node-b", map[string]string{DefaultLabelLeaf: "old-leaf"}),
		testNode("node-c", map[string]string{DefaultLabelLeaf: "leaf-a"}),
	)
	graph := graphWithTiers(map[string][]string{
		"node-a": {"leaf-a"},
		"node-b": {"leaf-a"},
		"node-c": {"leaf-a"},
	})

	generateOutputSuccessfully(t, testEngine(client), graph)

	assertNodeActionCounts(t, client, 1, 0, 1)
	require.Equal(t, []string{"node-b"}, updatedNodeNames(client))
}

func TestGenerateOutputPreservesUnmanagedNodeFields(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	node := testNode("node-a", map[string]string{
		DefaultLabelLeaf: "old-leaf", "workload.example/label": "keep",
	})
	node.Annotations = map[string]string{"workload.example/annotation": "keep"}
	node.Spec.Unschedulable = true
	node.Status.Phase = corev1.NodeRunning
	client := newFakeClient(node)

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
	)

	updated := trackedNode(t, client, "node-a")
	require.Equal(t, "keep", updated.Labels["workload.example/label"])
	require.Equal(t, node.Annotations, updated.Annotations)
	require.Equal(t, node.Spec, updated.Spec)
	require.Equal(t, node.Status, updated.Status)
	assertNodeActionCounts(t, client, 1, 0, 1)
}

func TestGenerateOutputUsesCustomKeysWithoutCleaningPreviousConfiguration(t *testing.T) {
	setTestLabels(t, "custom.example/accelerator", "custom.example/leaf", "custom.example/spine", "custom.example/core")
	node := testNode("node-a", map[string]string{
		"custom.example/leaf": "old-leaf",
		DefaultLabelLeaf:      "previous-config-kept",
	})
	client := newFakeClient(node)
	graph := graphWithTiers(map[string][]string{"node-a": {"new-leaf", "new-spine"}})
	graph.Domains = domainsByNode(map[string]string{"node-a": "domain-a"})

	generateOutputSuccessfully(t, testEngine(client), graph)

	require.Equal(t, map[string]string{
		"custom.example/accelerator": "domain-a",
		"custom.example/leaf":        "new-leaf",
		"custom.example/spine":       "new-spine",
		DefaultLabelLeaf:             "previous-config-kept",
	}, trackedNode(t, client, "node-a").Labels)
	assertNodeActionCounts(t, client, 1, 0, 1)
}

func TestGenerateOutputRespectsPlanAuthority(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	testCases := []struct {
		name       string
		initial    map[string]string
		graph      *topology.Graph
		wantLabels map[string]string
	}{
		{
			name: "domain only coordinates only accelerator",
			initial: map[string]string{
				DefaultLabelAccelerator: "old-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
			graph: &topology.Graph{Domains: domainsByNode(map[string]string{"node-a": "new-domain"})},
			wantLabels: map[string]string{
				DefaultLabelAccelerator: "new-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
		},
		{
			name: "tiers and domain merge all authority",
			initial: map[string]string{
				DefaultLabelAccelerator: "old-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
			graph: func() *topology.Graph {
				graph := graphWithTiers(map[string][]string{"node-a": {"new-leaf", "new-spine"}})
				graph.Domains = domainsByNode(map[string]string{"node-a": "new-domain"})
				return graph
			}(),
			wantLabels: map[string]string{
				DefaultLabelAccelerator: "new-domain", DefaultLabelLeaf: "new-leaf", DefaultLabelSpine: "new-spine",
			},
		},
		{
			name: "tiers only leave accelerator unchanged",
			initial: map[string]string{
				DefaultLabelAccelerator: "keep-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
				topology.KeyNvidiaGPUClique: "cluster-a.0",
			},
			graph: graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
			wantLabels: map[string]string{
				DefaultLabelAccelerator: "keep-domain", DefaultLabelLeaf: "new-leaf", topology.KeyNvidiaGPUClique: "cluster-a.0",
			},
		},
		{
			name: "empty tiers and domains do not coordinate tier labels",
			initial: map[string]string{
				DefaultLabelAccelerator: "old-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
			graph: &topology.Graph{
				Tiers:   &topology.Vertex{Vertices: map[string]*topology.Vertex{}},
				Domains: domainsByNode(map[string]string{"node-a": "new-domain"}),
			},
			wantLabels: map[string]string{
				DefaultLabelAccelerator: "new-domain", DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := newFakeClient(testNode("node-a", tc.initial))
			generateOutputSuccessfully(t, testEngine(client), tc.graph)
			require.Equal(t, tc.wantLabels, trackedNode(t, client, "node-a").Labels)
			assertNodeActionCounts(t, client, 1, 0, 1)
		})
	}
}

func TestGenerateOutputDoesNotTouchNodesOutsideGraph(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	outside := testNode("node-outside", map[string]string{
		DefaultLabelLeaf: "old-leaf", DefaultLabelSpine: "old-spine", DefaultLabelCore: "old-core",
	})
	client := newFakeClient(
		testNode("node-in-graph", map[string]string{DefaultLabelLeaf: "leaf-a"}),
		outside,
	)

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-in-graph": {"leaf-a"}}),
	)

	require.Equal(t, outside.Labels, trackedNode(t, client, "node-outside").Labels)
	assertNodeActionCounts(t, client, 1, 0, 0)
}

func TestGenerateOutputEmptyGraphDoesNotListOrWrite(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelCore: "keep"}))

	generateOutputSuccessfully(t, testEngine(client), &topology.Graph{})

	assertNodeActionCounts(t, client, 0, 0, 0)
}

func TestGenerateOutputSkipsGraphNodeMissingFromList(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient()

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"missing-node": {"leaf-a"}}),
	)

	assertNodeActionCounts(t, client, 1, 0, 0)
}

func TestGenerateOutputPreservesGPUClique(t *testing.T) {
	testCases := []struct {
		name             string
		acceleratorKey   string
		initial          map[string]string
		wantLabels       map[string]string
		wantAccelerator  bool
		acceleratorValue string
	}{
		{
			name:           "distinct accelerator key removes stale Topograph accelerator",
			acceleratorKey: DefaultLabelAccelerator,
			initial: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster-a.0", DefaultLabelAccelerator: "old-domain", DefaultLabelLeaf: "old-leaf",
			},
			wantLabels: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster-a.0", DefaultLabelLeaf: "new-leaf",
			},
		},
		{
			name:           "GPU clique key configured as accelerator remains protected",
			acceleratorKey: topology.KeyNvidiaGPUClique,
			initial: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster-a.0", DefaultLabelLeaf: "old-leaf",
			},
			wantLabels: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster-a.0", DefaultLabelLeaf: "new-leaf",
			},
		},
		{
			name:             "blank GPU clique does not suppress accelerator",
			acceleratorKey:   DefaultLabelAccelerator,
			initial:          map[string]string{topology.KeyNvidiaGPUClique: "  ", DefaultLabelLeaf: "new-leaf"},
			wantLabels:       map[string]string{topology.KeyNvidiaGPUClique: "  ", DefaultLabelLeaf: "new-leaf", DefaultLabelAccelerator: "api-domain"},
			wantAccelerator:  true,
			acceleratorValue: "api-domain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setTestLabels(t, tc.acceleratorKey, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
			client := newFakeClient(testNode("node-a", tc.initial))
			graph := graphWithTiers(map[string][]string{"node-a": {"new-leaf"}})
			graph.Domains = domainsByNode(map[string]string{"node-a": "api-domain"})

			generateOutputSuccessfully(t, testEngine(client), graph)

			updated := trackedNode(t, client, "node-a")
			require.Equal(t, tc.wantLabels, updated.Labels)
			value, found := updated.Labels[tc.acceleratorKey]
			if tc.wantAccelerator {
				require.True(t, found)
				require.Equal(t, tc.acceleratorValue, value)
			}
			assertNodeActionCounts(t, client, 1, 0, 1)
		})
	}
}

func TestGenerateOutputGPUCliqueSuppressesRedundantAcceleratorUpdate(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{
		topology.KeyNvidiaGPUClique: "cluster-a.0",
	}))

	generateOutputSuccessfully(
		t,
		testEngine(client),
		&topology.Graph{Domains: domainsByNode(map[string]string{"node-a": "api-domain"})},
	)

	require.Equal(t, map[string]string{
		topology.KeyNvidiaGPUClique: "cluster-a.0",
	}, trackedNode(t, client, "node-a").Labels)
	assertNodeActionCounts(t, client, 1, 0, 0)
}

func TestGenerateOutputProtectsGPUCliqueFromCustomTierKeys(t *testing.T) {
	testCases := []struct {
		name       string
		leafKey    string
		coreKey    string
		initial    map[string]string
		tierValues []string
	}{
		{
			name:       "does not overwrite clique through leaf key",
			leafKey:    topology.KeyNvidiaGPUClique,
			coreKey:    DefaultLabelCore,
			initial:    map[string]string{topology.KeyNvidiaGPUClique: "cluster-a.0"},
			tierValues: []string{"new-leaf"},
		},
		{
			name:    "does not delete clique through absent core tier",
			leafKey: DefaultLabelLeaf,
			coreKey: topology.KeyNvidiaGPUClique,
			initial: map[string]string{
				topology.KeyNvidiaGPUClique: "cluster-a.0",
				DefaultLabelLeaf:            "leaf-a",
			},
			tierValues: []string{"leaf-a"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setTestLabels(t, DefaultLabelAccelerator, tc.leafKey, DefaultLabelSpine, tc.coreKey)
			client := newFakeClient(testNode("node-a", tc.initial))

			generateOutputSuccessfully(
				t,
				testEngine(client),
				graphWithTiers(map[string][]string{"node-a": tc.tierValues}),
			)

			require.Equal(t, tc.initial, trackedNode(t, client, "node-a").Labels)
			assertNodeActionCounts(t, client, 1, 0, 0)
		})
	}
}

func TestGenerateOutputIgnoresAllEmptyManagedKeys(t *testing.T) {
	setTestLabels(t, "", " ", "", "\t")
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "keep"}))
	graph := graphWithTiers(map[string][]string{"node-a": {"leaf-a", "spine-a", "core-a"}})
	graph.Domains = domainsByNode(map[string]string{"node-a": "domain-a"})

	generateOutputSuccessfully(t, testEngine(client), graph)

	require.Equal(t, map[string]string{DefaultLabelLeaf: "keep"}, trackedNode(t, client, "node-a").Labels)
	assertNodeActionCounts(t, client, 0, 0, 0)
}

func TestGenerateOutputRetriesOnlyConflictingNode(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(
		testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf-a"}),
		testNode("node-b", map[string]string{DefaultLabelLeaf: "old-leaf-b"}),
	)
	updateCalls := 0
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		updateCalls++
		if updateCalls != 1 {
			return false, nil, nil
		}
		require.Equal(t, "node-a", action.(k8stesting.UpdateAction).GetObject().(*corev1.Node).Name)
		concurrent := trackedNode(t, client, "node-a").DeepCopy()
		concurrent.ResourceVersion = "2"
		concurrent.Labels["concurrent.example/label"] = "keep"
		require.NoError(t, client.Tracker().Update(nodeGVR, concurrent, ""))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "node-a", errors.New("conflict"))
	})

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{
			"node-a": {"new-leaf-a"},
			"node-b": {"new-leaf-b"},
		}),
	)

	updated := trackedNode(t, client, "node-a")
	require.Equal(t, "new-leaf-a", updated.Labels[DefaultLabelLeaf])
	require.Equal(t, "keep", updated.Labels["concurrent.example/label"])
	require.Equal(t, "new-leaf-b", trackedNode(t, client, "node-b").Labels[DefaultLabelLeaf])
	assertNodeActionCounts(t, client, 1, 1, 3)
	require.Equal(t, []string{"node-a"}, nodeGetNames(client))
	updates := nodeUpdateActions(client)
	require.Equal(t, "1", updates[0].GetObject().(*corev1.Node).ResourceVersion)
	require.Equal(t, "2", updates[1].GetObject().(*corev1.Node).ResourceVersion)
}

func TestGenerateOutputStopsRetryWhenConcurrentWriterAlreadyConverged(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		concurrent := trackedNode(t, client, "node-a").DeepCopy()
		concurrent.ResourceVersion = "2"
		concurrent.Labels[DefaultLabelLeaf] = "new-leaf"
		concurrent.Labels["concurrent.example/label"] = "keep"
		require.NoError(t, client.Tracker().Update(nodeGVR, concurrent, ""))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "node-a", errors.New("conflict"))
	})

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
	)

	updated := trackedNode(t, client, "node-a")
	require.Equal(t, "new-leaf", updated.Labels[DefaultLabelLeaf])
	require.Equal(t, "keep", updated.Labels["concurrent.example/label"])
	assertNodeActionCounts(t, client, 1, 1, 1)
}

func TestGenerateOutputRecomputesGPUCliqueAfterConflict(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelAccelerator: "old-domain"}))
	updateCalls := 0
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		updateCalls++
		if updateCalls != 1 {
			return false, nil, nil
		}
		concurrent := trackedNode(t, client, "node-a").DeepCopy()
		concurrent.ResourceVersion = "2"
		concurrent.Labels[topology.KeyNvidiaGPUClique] = "cluster-a.0"
		require.NoError(t, client.Tracker().Update(nodeGVR, concurrent, ""))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "node-a", errors.New("conflict"))
	})

	generateOutputSuccessfully(
		t,
		testEngine(client),
		&topology.Graph{Domains: domainsByNode(map[string]string{"node-a": "api-domain"})},
	)

	updated := trackedNode(t, client, "node-a")
	require.Equal(t, "cluster-a.0", updated.Labels[topology.KeyNvidiaGPUClique])
	require.NotContains(t, updated.Labels, DefaultLabelAccelerator)
	assertNodeActionCounts(t, client, 1, 1, 2)
}

func TestGenerateOutputReturnsNonConflictUpdateError(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("write failed")
	})

	_, err := testEngine(client).GenerateOutput(
		context.Background(),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
		nil,
	)
	require.NotNil(t, err)
	require.ErrorContains(t, err, "write failed")
	assertNodeActionCounts(t, client, 1, 0, 1)
}

func TestGenerateOutputListFailurePerformsNoUpdates(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list failed")
	})

	_, err := testEngine(client).GenerateOutput(
		context.Background(),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
		nil,
	)
	require.NotNil(t, err)
	require.ErrorContains(t, err, "list failed")
	assertNodeActionCounts(t, client, 1, 0, 0)
}

func TestGenerateOutputLaterListPageFailurePerformsNoUpdates(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		options := action.(interface{ GetListOptions() metav1.ListOptions }).GetListOptions()
		if options.Continue == "" {
			return true, &corev1.NodeList{
				ListMeta: metav1.ListMeta{Continue: "next-page"},
				Items:    []corev1.Node{*testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"})},
			}, nil
		}
		return true, nil, errors.New("second page failed")
	})

	_, err := testEngine(client).GenerateOutput(
		context.Background(),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
		nil,
	)
	require.NotNil(t, err)
	require.ErrorContains(t, err, "second page failed")
	assertNodeActionCounts(t, client, 2, 0, 0)
}

func TestGenerateOutputPlanFailurePerformsNoAPIAction(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", nil))
	graph := &topology.Graph{Domains: topology.DomainMap{
		"domain-a": {"node-a": &topology.HostInfo{HostName: "node-a"}},
		"domain-b": {"node-a": &topology.HostInfo{HostName: "node-a"}},
	}}

	_, err := testEngine(client).GenerateOutput(context.Background(), graph, nil)
	require.NotNil(t, err)
	require.ErrorContains(t, err, "multiple accelerator labels")
	assertNodeActionCounts(t, client, 0, 0, 0)
}

func TestGenerateOutputSkipsNodeDeletedAfterList(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, "node-a")
	})

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
	)
	assertNodeActionCounts(t, client, 1, 0, 1)
}

func TestGenerateOutputSkipsNodeDeletedDuringConflictRetry(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	client := newFakeClient(testNode("node-a", map[string]string{DefaultLabelLeaf: "old-leaf"}))
	client.PrependReactor("update", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		require.NoError(t, client.Tracker().Delete(nodeGVR, "", "node-a"))
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "node-a", errors.New("conflict"))
	})

	generateOutputSuccessfully(
		t,
		testEngine(client),
		graphWithTiers(map[string][]string{"node-a": {"new-leaf"}}),
	)
	assertNodeActionCounts(t, client, 1, 1, 1)
}

func TestGenerateOutputCompletesPaginatedListBeforeUpdates(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	alpha := testNode("alpha", map[string]string{"pool": "selected", DefaultLabelLeaf: "old-alpha"})
	beta := testNode("beta", map[string]string{"pool": "selected", DefaultLabelLeaf: "old-beta"})
	client := newFakeClient(alpha, beta)
	sharedOptions := &metav1.ListOptions{LabelSelector: "pool=selected", Continue: "do-not-mutate"}
	eng := &K8sEngine{client: client, params: &Params{nodeListOpt: sharedOptions}}
	var listOptions []metav1.ListOptions
	client.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		options := action.(interface{ GetListOptions() metav1.ListOptions }).GetListOptions()
		listOptions = append(listOptions, options)
		switch options.Continue {
		case "":
			return true, &corev1.NodeList{
				ListMeta: metav1.ListMeta{Continue: "next-page"},
				Items:    []corev1.Node{*beta.DeepCopy()},
			}, nil
		case "next-page":
			return true, &corev1.NodeList{Items: []corev1.Node{*alpha.DeepCopy()}}, nil
		default:
			return true, nil, errors.New("unexpected continue token")
		}
	})
	graph := graphWithTiers(map[string][]string{
		"beta":  {"new-beta"},
		"alpha": {"new-alpha"},
	})

	generateOutputSuccessfully(t, eng, graph)

	require.Equal(t, []metav1.ListOptions{
		{LabelSelector: "pool=selected"},
		{LabelSelector: "pool=selected", Continue: "next-page"},
	}, listOptions)
	require.Equal(t, "do-not-mutate", sharedOptions.Continue)
	require.Equal(t, []string{"alpha", "beta"}, updatedNodeNames(client))
	assertNodeActionCounts(t, client, 2, 0, 2)
	require.Equal(t, []string{"list", "list", "update", "update"}, nodeActionVerbs(client))
}

func generateOutputSuccessfully(t *testing.T, eng *K8sEngine, graph *topology.Graph) {
	t.Helper()
	output, err := eng.GenerateOutput(context.Background(), graph, nil)
	require.Nil(t, err)
	require.Equal(t, []byte("OK\n"), output)
}

func testEngine(client *k8sfake.Clientset) *K8sEngine {
	return &K8sEngine{client: client, params: &Params{}}
}

func testNode(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:            name,
		ResourceVersion: "1",
		Labels:          maps.Clone(labels),
	}}
}

func newFakeClient(nodes ...*corev1.Node) *k8sfake.Clientset {
	objects := make([]runtime.Object, 0, len(nodes))
	for _, node := range nodes {
		objects = append(objects, node)
	}
	return k8sfake.NewSimpleClientset(objects...)
}

func trackedNode(t *testing.T, client *k8sfake.Clientset, name string) *corev1.Node {
	t.Helper()
	object, err := client.Tracker().Get(nodeGVR, "", name)
	require.NoError(t, err)
	return object.(*corev1.Node).DeepCopy()
}

func nodeActionCount(client *k8sfake.Clientset, verb string) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "nodes" && action.GetVerb() == verb {
			count++
		}
	}
	return count
}

func assertNodeActionCounts(t *testing.T, client *k8sfake.Clientset, lists, gets, updates int) {
	t.Helper()
	require.Equal(t, lists, nodeActionCount(client, "list"), "unexpected Node List action count")
	require.Equal(t, gets, nodeActionCount(client, "get"), "unexpected Node GET action count")
	require.Equal(t, updates, nodeActionCount(client, "update"), "unexpected Node Update action count")
	require.Zero(t, nodeActionCount(client, "patch"), "unexpected Node Patch action count")
}

func nodeUpdateActions(client *k8sfake.Clientset) []k8stesting.UpdateAction {
	updates := []k8stesting.UpdateAction{}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "nodes" && action.GetVerb() == "update" {
			updates = append(updates, action.(k8stesting.UpdateAction))
		}
	}
	return updates
}

func updatedNodeNames(client *k8sfake.Clientset) []string {
	names := []string{}
	for _, action := range nodeUpdateActions(client) {
		names = append(names, action.GetObject().(*corev1.Node).Name)
	}
	return names
}

func nodeActionVerbs(client *k8sfake.Clientset) []string {
	verbs := []string{}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "nodes" {
			verbs = append(verbs, action.GetVerb())
		}
	}
	return verbs
}

func nodeGetNames(client *k8sfake.Clientset) []string {
	names := []string{}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "nodes" && action.GetVerb() == "get" {
			names = append(names, action.(k8stesting.GetAction).GetName())
		}
	}
	return names
}
