/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package slinky

import (
	"context"
	"github.com/dsx-ai-factory/topograph/pkg/engines/slurm"
	kubernetesprovider "github.com/dsx-ai-factory/topograph/pkg/providers/kubernetes"
	"github.com/dsx-ai-factory/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestKubernetesProviderConfigMap(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	for _, name := range []string{"alpha", "bravo"} {
		_, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"rack": "r1", "zone": "z1"}, Annotations: map[string]string{topology.KeyNodeInstance: name, topology.KeyNodeRegion: "local"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}, metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = client.CoreV1().Pods("slurm").Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "slurm", Labels: map[string]string{"app": "slurmd"}}, Spec: corev1.PodSpec{NodeName: name, Hostname: name}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	p, err := kubernetesprovider.New(client, map[string]any{"topologyLabels": []string{"rack", "zone"}})
	require.NoError(t, err)
	generate := func() string {
		eng := &SlinkyEngine{client: client, params: &Params{BaseParams: slurm.BaseParams{Plugin: "topology/tree"}, Namespace: "slurm", ConfigMapName: "topology", ConfigPath: "topology.conf", podListOpt: &metav1.ListOptions{LabelSelector: "app=slurmd"}}}
		instances, herr := eng.ResolveComputeInstances(ctx, nil, p)
		require.Nil(t, herr)
		graph, herr := p.GenerateTopologyConfig(ctx, nil, instances)
		require.Nil(t, herr)
		_, herr = eng.GenerateOutput(ctx, graph, nil)
		require.Nil(t, herr)
		cm, err := client.CoreV1().ConfigMaps("slurm").Get(ctx, "topology", metav1.GetOptions{})
		require.NoError(t, err)
		return cm.Data["topology.conf"]
	}
	initial := generate()
	require.Contains(t, initial, "alpha")
	require.Contains(t, initial, "bravo")
	node, err := client.CoreV1().Nodes().Get(ctx, "bravo", metav1.GetOptions{})
	require.NoError(t, err)
	node.Labels["rack"] = "r2"
	node, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NotEqual(t, initial, generate())
	node.Status.Conditions[0].Status = corev1.ConditionFalse
	node, err = client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NotContains(t, generate(), "bravo")
	node.Status.Conditions[0].Status = corev1.ConditionTrue
	_, err = client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.Contains(t, generate(), "bravo")
	require.NoError(t, client.CoreV1().Nodes().Delete(ctx, "bravo", metav1.DeleteOptions{}))
	require.NotContains(t, generate(), "bravo")
	require.NoError(t, client.CoreV1().Nodes().Delete(ctx, "alpha", metav1.DeleteOptions{}))
	empty := generate()
	require.NotContains(t, empty, "alpha")
	require.NotContains(t, empty, "bravo")
}
