/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package kubernetes

import (
	"context"
	"testing"

	"github.com/dsx-ai-factory/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func readyNode(name, rack, zone string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"rack": rack, "zone": zone}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
}

func TestTopologyTiers(t *testing.T) {
	for _, keys := range [][]string{{"rack"}, {"rack", "zone"}} {
		t.Run(keys[len(keys)-1], func(t *testing.T) {
			a, b, c := readyNode("a", "rack-1", "east"), readyNode("b", "rack-1", "east"), readyNode("c", "rack-1", "west")
			a.Annotations = map[string]string{topology.KeyNodeInstance: "instance-a"}
			p, err := New(fake.NewSimpleClientset(a, b, c), map[string]any{"topologyLabels": keys})
			require.NoError(t, err)
			graph, herr := p.GenerateTopologyConfig(context.Background(), nil, []topology.ComputeInstances{{Instances: map[string]string{"instance-a": "slurm-a", "b": "slurm-b", "c": "slurm-c"}}})
			require.Nil(t, herr)
			if len(keys) == 1 {
				require.Len(t, graph.Tiers.Vertices, 1)
				require.Len(t, graph.Tiers.Vertices["rack-1"].Vertices, 3)
				return
			}
			require.Len(t, graph.Tiers.Vertices, 2)
			east := graph.Tiers.Vertices["east"].Vertices["rack-1/east"]
			west := graph.Tiers.Vertices["west"].Vertices["rack-1/west"]
			require.Len(t, east.Vertices, 2)
			require.Len(t, west.Vertices, 1)
			require.Equal(t, "slurm-a", east.Vertices["instance-a"].Name)
			require.NotEqual(t, east.Name, west.Name)
		})
	}
}

func TestEligibleRequestedNodes(t *testing.T) {
	a, b, c, d := readyNode("a", "r1", "z1"), readyNode("b", "", ""), readyNode("c", "r1", "z1"), readyNode("d", "r2", "z1")
	a.Spec.Unschedulable = true
	b.Status.Conditions[0].Status = corev1.ConditionFalse
	now := metav1.Now()
	c.DeletionTimestamp = &now
	for _, node := range []*corev1.Node{a, b, c} {
		node.Labels["pool"] = "selected"
	}
	d.Labels["pool"] = "other"
	p, err := New(fake.NewSimpleClientset(a, b, c, d), map[string]any{"topologyLabels": []string{"rack"}, "nodeSelector": map[string]string{"pool": "selected"}})
	require.NoError(t, err)
	requested := []topology.ComputeInstances{{Instances: map[string]string{"a": "a", "b": "b", "c": "c", "d": "d", "deleted": "deleted"}}}
	graph, herr := p.GenerateTopologyConfig(context.Background(), nil, requested)
	require.Nil(t, herr)
	require.Len(t, graph.Tiers.Vertices, 1)
	require.Len(t, graph.Tiers.Vertices["r1"].Vertices, 1)
	require.Contains(t, graph.Tiers.Vertices["r1"].Vertices, "a")
	graph, herr = p.GenerateTopologyConfig(context.Background(), nil, nil)
	require.Nil(t, herr)
	require.Empty(t, graph.Tiers.Vertices)
	p, err = New(fake.NewSimpleClientset(a, b, c), map[string]any{"topologyLabels": []string{"rack"}})
	require.NoError(t, err)
	graph, herr = p.GenerateTopologyConfig(context.Background(), nil, requested)
	require.Nil(t, herr)
	require.Len(t, graph.Tiers.Vertices["r1"].Vertices, 1)
}

func TestInvalidTopologyLabel(t *testing.T) {
	for _, value := range []string{"", "not/a/value", " value "} {
		p, err := New(fake.NewSimpleClientset(readyNode("a", value, "z")), map[string]any{"topologyLabels": []string{"rack"}})
		require.NoError(t, err)
		graph, herr := p.GenerateTopologyConfig(context.Background(), nil, []topology.ComputeInstances{{Instances: map[string]string{"a": "a"}}})
		require.Nil(t, graph)
		require.NotNil(t, herr)
		require.Contains(t, herr.Error(), "rack")
		require.Contains(t, herr.Error(), `node "a"`)
	}
}

func TestParseParams(t *testing.T) {
	for _, keys := range []any{nil, []string{}, []string{""}, []string{"not/a/key"}, []string{"rack", "rack"}, 12, "rack", []any{1}, []any{nil}} {
		_, err := ParseParams(map[string]any{"topologyLabels": keys})
		require.Error(t, err)
	}
	_, err := ParseParams(map[string]any{"topologyLabels": []any{"rack", "zone"}})
	require.NoError(t, err)
	_, err = ParseParams(map[string]any{"topologyLabels": []string{"rack"}, "nodeSelector": map[string]string{"pool": "bad/value"}})
	require.Error(t, err)
}

func TestNodeEligibility(t *testing.T) {
	node := readyNode("a", "r", "z")
	require.True(t, IsNodeEligible(node))
	for _, status := range []corev1.ConditionStatus{corev1.ConditionFalse, corev1.ConditionUnknown} {
		node.Status.Conditions[0].Status = status
		require.False(t, IsNodeEligible(node))
	}
	node.Status.Conditions = nil
	require.False(t, IsNodeEligible(node))
}
