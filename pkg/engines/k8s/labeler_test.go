/*
 * Copyright (c) 2024-2026, NVIDIA CORPORATION.  All rights reserved.
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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

type recordingReconciler struct {
	calls int
	plans nodeLabelPlans
}

func (r *recordingReconciler) reconcileNodeLabelPlans(_ context.Context, plans nodeLabelPlans) error {
	r.calls++
	r.plans = plans
	return nil
}

func TestBuildNodeLabelPlansWithTree(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph, _ := translate.GetTreeTestSet(true)
	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)

	want := map[string]map[string]string{
		"Node201": {DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1"},
		"Node202": {DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1"},
		"Node205": {DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1"},
		"Node304": {DefaultLabelLeaf: "xf946c4acef2d5939", DefaultLabelSpine: "S1"},
		"Node305": {DefaultLabelLeaf: "xf946c4acef2d5939", DefaultLabelSpine: "S1"},
		"Node306": {DefaultLabelLeaf: "xf946c4acef2d5939", DefaultLabelSpine: "S1"},
	}
	require.Equal(t, want, desiredLabelsByNode(plans))
	for nodeName := range want {
		require.Equal(t, keySet(DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore), plans[nodeName].ManagedKeys)
		require.Empty(t, plans[nodeName].acceleratorKey)
	}
}

func TestBuildNodeLabelPlansWithBlock(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph, _ := translate.GetBlockWithMultiIBTestSet()
	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)

	want := map[string]map[string]string{
		"Node104": {DefaultLabelAccelerator: "B1", DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node105": {DefaultLabelAccelerator: "B1", DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node106": {DefaultLabelAccelerator: "B1", DefaultLabelLeaf: "S2", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node201": {DefaultLabelAccelerator: "B2", DefaultLabelLeaf: "S3", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node202": {DefaultLabelAccelerator: "B2", DefaultLabelLeaf: "S3", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node205": {DefaultLabelAccelerator: "B2", DefaultLabelLeaf: "S3", DefaultLabelSpine: "S1", DefaultLabelCore: "IB2"},
		"Node301": {DefaultLabelAccelerator: "B3", DefaultLabelLeaf: "S5", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
		"Node302": {DefaultLabelAccelerator: "B3", DefaultLabelLeaf: "S5", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
		"Node303": {DefaultLabelAccelerator: "B3", DefaultLabelLeaf: "S5", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
		"Node401": {DefaultLabelAccelerator: "B4", DefaultLabelLeaf: "S6", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
		"Node402": {DefaultLabelAccelerator: "B4", DefaultLabelLeaf: "S6", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
		"Node403": {DefaultLabelAccelerator: "B4", DefaultLabelLeaf: "S6", DefaultLabelSpine: "S4", DefaultLabelCore: "IB1"},
	}
	require.Equal(t, want, desiredLabelsByNode(plans))
	for nodeName := range want {
		require.Equal(
			t,
			keySet(DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore),
			plans[nodeName].ManagedKeys,
		)
		require.Equal(t, DefaultLabelAccelerator, plans[nodeName].acceleratorKey)
	}
}

func TestBuildTierPlansByDepth(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	testCases := []struct {
		name    string
		tiers   []string
		desired map[string]string
	}{
		{name: "one tier", tiers: []string{"leaf-a"}, desired: map[string]string{DefaultLabelLeaf: "leaf-a"}},
		{name: "two tiers", tiers: []string{"leaf-a", "spine-a"}, desired: map[string]string{DefaultLabelLeaf: "leaf-a", DefaultLabelSpine: "spine-a"}},
		{name: "three tiers", tiers: []string{"leaf-a", "spine-a", "core-a"}, desired: map[string]string{DefaultLabelLeaf: "leaf-a", DefaultLabelSpine: "spine-a", DefaultLabelCore: "core-a"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plans, err := newTopologyLabeler().buildNodeLabelPlans(graphWithTiers(map[string][]string{"node-a": tc.tiers}))
			require.NoError(t, err)
			require.Equal(t, tc.desired, plans["node-a"].Desired)
			require.Equal(t, keySet(DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore), plans["node-a"].ManagedKeys)
		})
	}
}

func TestBuildNodeLabelPlansAuthority(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph := graphWithTiers(map[string][]string{
		"both":       {"leaf-b", "spine-b"},
		"tiers-only": {"leaf-t"},
	})
	graph.Domains = domainsByNode(map[string]string{
		"both":        "domain-b",
		"domain-only": "domain-d",
	})

	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)
	require.Equal(t, &nodeLabelPlan{
		Desired: map[string]string{
			DefaultLabelAccelerator: "domain-d",
		},
		ManagedKeys:    keySet(DefaultLabelAccelerator),
		acceleratorKey: DefaultLabelAccelerator,
	}, plans["domain-only"])
	require.Equal(t, &nodeLabelPlan{
		Desired: map[string]string{
			DefaultLabelAccelerator: "domain-b",
			DefaultLabelLeaf:        "leaf-b",
			DefaultLabelSpine:       "spine-b",
		},
		ManagedKeys:    keySet(DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore),
		acceleratorKey: DefaultLabelAccelerator,
	}, plans["both"])
	require.Equal(t, &nodeLabelPlan{
		Desired: map[string]string{
			DefaultLabelLeaf: "leaf-t",
		},
		ManagedKeys: keySet(DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore),
	}, plans["tiers-only"])
	require.NotContains(t, plans, "outside")
}

func TestBuildNodeLabelPlansWithEmptyTiersAndDomains(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph := &topology.Graph{
		Tiers:   &topology.Vertex{Vertices: map[string]*topology.Vertex{}},
		Domains: domainsByNode(map[string]string{"node-a": "domain-a"}),
	}

	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)
	require.Equal(t, map[string]string{DefaultLabelAccelerator: "domain-a"}, plans["node-a"].Desired)
	require.Equal(t, keySet(DefaultLabelAccelerator), plans["node-a"].ManagedKeys)
}

func TestBuildNodeLabelPlansEmptyGraph(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	for _, graph := range []*topology.Graph{
		nil,
		{},
		{Tiers: &topology.Vertex{Vertices: map[string]*topology.Vertex{}}},
	} {
		plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
		require.NoError(t, err)
		require.Empty(t, plans)
	}
}

func TestBuildNodeLabelPlansCustomAndEmptyKeys(t *testing.T) {
	setTestLabels(t, "custom.example/accelerator", "custom.example/leaf", "", "  ")
	graph := graphWithTiers(map[string][]string{"node-a": {"leaf-a", "spine-a", "core-a"}})
	graph.Domains = domainsByNode(map[string]string{"node-a": "domain-a"})

	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"custom.example/accelerator": "domain-a",
		"custom.example/leaf":        "leaf-a",
	}, plans["node-a"].Desired)
	require.Equal(t, keySet("custom.example/accelerator", "custom.example/leaf"), plans["node-a"].ManagedKeys)
}

func TestBuildNodeLabelPlansDeduplicatesKeys(t *testing.T) {
	setTestLabels(t, "custom.example/shared", "custom.example/shared", DefaultLabelSpine, DefaultLabelCore)
	graph := graphWithTiers(map[string][]string{"node-a": {"same", "spine-a"}})
	graph.Domains = domainsByNode(map[string]string{"node-a": "same"})

	plans, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"custom.example/shared": "same",
		DefaultLabelSpine:       "spine-a",
	}, plans["node-a"].Desired)
	require.Equal(t, keySet("custom.example/shared", DefaultLabelSpine, DefaultLabelCore), plans["node-a"].ManagedKeys)
}

func TestBuildNodeLabelPlansRejectsConflictingKeys(t *testing.T) {
	setTestLabels(t, "custom.example/shared", "custom.example/shared", DefaultLabelSpine, DefaultLabelCore)
	graph := graphWithTiers(map[string][]string{"node-a": {"leaf-a"}})
	graph.Domains = domainsByNode(map[string]string{"node-a": "domain-a"})

	_, err := newTopologyLabeler().buildNodeLabelPlans(graph)
	require.EqualError(
		t,
		err,
		`conflicting desired values "domain-a" and "leaf-a" for label "custom.example/shared" on node "node-a"`,
	)
}

func TestBuildNodeLabelPlansRejectsMultipleDomainsBeforeReconcile(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph := &topology.Graph{Domains: topology.DomainMap{
		"domain-a": {"node-a": &topology.HostInfo{HostName: "node-a"}},
		"domain-b": {"node-a": &topology.HostInfo{HostName: "node-a"}},
	}}
	reconciler := &recordingReconciler{}

	err := newTopologyLabeler().applyNodeLabels(context.Background(), graph, reconciler)
	require.EqualError(t, err, "multiple accelerator labels domain-a, domain-b for node node-a")
	require.Zero(t, reconciler.calls)
}

func TestBuildNodeLabelPlansRejectsNilVertexBeforeReconcile(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	graph := &topology.Graph{Tiers: &topology.Vertex{Vertices: map[string]*topology.Vertex{"bad": nil}}}
	reconciler := &recordingReconciler{}

	err := newTopologyLabeler().applyNodeLabels(context.Background(), graph, reconciler)
	require.EqualError(t, err, `topology vertex "" contains a nil child`)
	require.Zero(t, reconciler.calls)
}

func TestBuildNodeLabelPlansHashesLongValues(t *testing.T) {
	setTestLabels(t, DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	longDomain := strings.Repeat("domain", 20)
	plans, err := newTopologyLabeler().buildNodeLabelPlans(&topology.Graph{
		Domains: domainsByNode(map[string]string{"node-a": longDomain}),
	})
	require.NoError(t, err)
	require.Equal(t, "x98512e5ff9824935", plans["node-a"].Desired[DefaultLabelAccelerator])
}

func TestInitLabels(t *testing.T) {
	setTestLabels(t, "a", "b", "c", "d")
	require.Equal(t, "a", labelAccelerator)
	require.Equal(t, "b", labelLeaf)
	require.Equal(t, "c", labelSpine)
	require.Equal(t, "d", labelCore)
}

func setTestLabels(t *testing.T, accelerator, leaf, spine, core string) {
	t.Helper()
	InitLabels(accelerator, leaf, spine, core)
	t.Cleanup(func() {
		InitLabels(DefaultLabelAccelerator, DefaultLabelLeaf, DefaultLabelSpine, DefaultLabelCore)
	})
}

func keySet(keys ...string) labelKeySet {
	set := make(labelKeySet)
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}

func desiredLabelsByNode(plans nodeLabelPlans) map[string]map[string]string {
	desired := make(map[string]map[string]string, len(plans))
	for nodeName, plan := range plans {
		desired[nodeName] = plan.Desired
	}
	return desired
}

func domainsByNode(domains map[string]string) topology.DomainMap {
	domainMap := topology.NewDomainMap()
	for nodeName, domain := range domains {
		domainMap.AddHost(domain, nodeName, nodeName)
	}
	return domainMap
}

func graphWithTiers(tiersByNode map[string][]string) *topology.Graph {
	root := &topology.Vertex{Vertices: make(map[string]*topology.Vertex)}
	for nodeName, tiers := range tiersByNode {
		parent := root
		for i := len(tiers) - 1; i >= 0; i-- {
			tierID := tiers[i]
			vertex, ok := parent.Vertices[tierID]
			if !ok {
				vertex = &topology.Vertex{ID: tierID, Vertices: make(map[string]*topology.Vertex)}
				parent.Vertices[tierID] = vertex
			}
			parent = vertex
		}
		parent.Vertices[nodeName] = &topology.Vertex{Name: nodeName, ID: nodeName}
	}
	return &topology.Graph{Tiers: root}
}
