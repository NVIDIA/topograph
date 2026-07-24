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
	"github.com/NVIDIA/topograph/pkg/translate"
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
		{
			name: "Case 10: use GPU clique label",
			params: map[string]any{
				topology.KeyNamespace:         "namespace",
				topology.KeyPodSelector:       podSelector,
				topology.KeyTopoConfigPath:    "path",
				topology.KeyTopoConfigmapName: "name",
				"useGpuCliqueLabel":           true,
			},
			ret: &Params{
				Namespace:         "namespace",
				PodSelector:       labelSelector,
				ConfigPath:        "path",
				ConfigMapName:     "name",
				UseGPUCliqueLabel: true,
				podListOpt:        &metav1.ListOptions{LabelSelector: "key=value"},
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

func TestWithGPUCliqueDomains(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-0",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: "clique-a"},
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-0"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-1",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: " clique-b "},
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-1"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-no-instance",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: "clique-c"},
				Annotations: map[string]string{},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-no-pod",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: "clique-d"},
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-3"},
			},
		},
	}
	for _, node := range nodes {
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	for _, pod := range []*corev1.Pod{
		makeReadySlurmdPod("pod-0", "k8s-node-0", "slurm-0"),
		makeReadySlurmdPod("pod-1", "k8s-node-1", "slurm-1"),
		makeReadySlurmdPod("pod-no-instance", "k8s-node-no-instance", "slurm-no-instance"),
	} {
		_, err := client.CoreV1().Pods("test-ns").Create(ctx, pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	existingDomains := topology.NewDomainMap()
	existingDomains.AddHost("provider-domain", "provider-instance", "provider-node")
	graph := &topology.Graph{
		Tiers:   &topology.Vertex{ID: "root"},
		Domains: existingDomains,
	}
	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:  "test-ns",
			podListOpt: &metav1.ListOptions{LabelSelector: "app=slinky"},
		},
	}

	clusterNodes, httpErr := eng.getClusterNodes(ctx)
	require.Nil(t, httpErr)
	got, httpErr := withGPUCliqueDomains(graph, clusterNodes)
	require.Nil(t, httpErr)
	require.NotSame(t, graph, got)
	require.Same(t, graph.Tiers, got.Tiers)

	expectedDomains := topology.NewDomainMap()
	expectedDomains.AddHost("clique-a", "instance-0", "slurm-0")
	expectedDomains.AddHost("clique-b", "instance-1", "slurm-1")
	require.Equal(t, expectedDomains, got.Domains)
	require.Equal(t, existingDomains, graph.Domains)
}

func TestWithGPUCliqueDomainsNoMatchingNodes(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	_, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "k8s-node-0",
			Annotations: map[string]string{topology.KeyNodeInstance: "instance-0"},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	_, err = client.CoreV1().Pods("test-ns").Create(ctx, makeReadySlurmdPod("pod-0", "k8s-node-0", "slurm-0"), metav1.CreateOptions{})
	require.NoError(t, err)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:  "test-ns",
			podListOpt: &metav1.ListOptions{LabelSelector: "app=slinky"},
		},
	}

	clusterNodes, httpErr := eng.getClusterNodes(ctx)
	require.Nil(t, httpErr)
	got, httpErr := withGPUCliqueDomains(&topology.Graph{}, clusterNodes)
	require.Nil(t, got)
	require.ErrorContains(t, httpErr, "useGpuCliqueLabel=true but no matching nodes found")
}

func TestGenerateOutputUsesGPUCliqueDomains(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	for _, node := range []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-0",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: "clique-a"},
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-0"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "k8s-node-1",
				Labels:      map[string]string{topology.KeyNvidiaGPUClique: "clique-b"},
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-1"},
			},
		},
	} {
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	for _, pod := range []*corev1.Pod{
		makeReadySlurmdPod("pod-0", "k8s-node-0", "alpha"),
		makeReadySlurmdPod("pod-1", "k8s-node-1", "beta"),
	} {
		_, err := client.CoreV1().Pods("test-ns").Create(ctx, pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	providerDomains := topology.NewDomainMap()
	providerDomains.AddHost("provider-domain", "instance-0", "alpha")
	providerDomains.AddHost("provider-domain", "instance-1", "beta")

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			BaseParams: slurm.BaseParams{
				Plugin:     topology.TopologyBlock,
				BlockSizes: []int{1},
			},
			Namespace:         "test-ns",
			ConfigMapName:     "slurm-config",
			ConfigPath:        "topology.conf",
			UseGPUCliqueLabel: true,
			podListOpt:        &metav1.ListOptions{LabelSelector: "app=slinky"},
		},
	}

	result, httpErr := eng.GenerateOutput(ctx, &topology.Graph{Domains: providerDomains}, nil)
	require.Nil(t, httpErr)
	require.Equal(t, []byte("OK\n"), result)

	cm, err := client.CoreV1().ConfigMaps("test-ns").Get(ctx, "slurm-config", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, `# block001=clique-a
BlockName=block001 Nodes=alpha
# block002=clique-b
BlockName=block002 Nodes=beta
BlockSizes=1
`, cm.Data["topology.conf"])
}

func TestUsesBlockTopology(t *testing.T) {
	require.False(t, usesBlockTopology(nil))
	require.False(t, usesBlockTopology(&translate.Config{Plugin: topology.TopologyTree}))
	require.True(t, usesBlockTopology(&translate.Config{Plugin: topology.TopologyBlock}))
	require.True(t, usesBlockTopology(&translate.Config{
		Topologies: map[string]*translate.TopologySpec{
			"block": {Plugin: topology.TopologyBlock},
		},
	}))
	require.False(t, usesBlockTopology(&translate.Config{
		Topologies: map[string]*translate.TopologySpec{
			"flat": {Plugin: topology.TopologyFlat},
			"nil":  nil,
		},
	}))
}

func makeReadySlurmdPod(name, nodeName, slurmName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
			Labels: map[string]string{
				"app":                     "slinky",
				topology.KeySlurmNodeName: slurmName,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
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
}

// createPhysicalNodes creates K8s nodes with a KeyNodeInstance annotation so
// they appear as physical inventory to the Slinky engine.
func createPhysicalNodes(t *testing.T, ctx context.Context, client *fake.Clientset, names []string) {
	t.Helper()
	for i, name := range names {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Annotations: map[string]string{topology.KeyNodeInstance: fmt.Sprintf("i-%d", i)},
			},
		}
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)
	}
}

// reconcileReadySlurmdPods deletes any slurmd pods whose node is not in
// k8sNodes and creates Ready slurmd pods for those that are, so subsequent
// engine calls observe exactly the requested Ready set.
func reconcileReadySlurmdPods(t *testing.T, ctx context.Context, client *fake.Clientset, namespace string, k8sNodes []string) {
	t.Helper()
	want := make(map[string]bool, len(k8sNodes))
	for _, name := range k8sNodes {
		want[name] = true
	}

	existing, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	have := make(map[string]bool, len(existing.Items))
	for _, pod := range existing.Items {
		nodeName := pod.Spec.NodeName
		if want[nodeName] {
			have[nodeName] = true
			continue
		}
		require.NoError(t, client.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
	}

	for _, k8sName := range k8sNodes {
		if have[k8sName] {
			continue
		}
		pod := makeReadySlurmdPod("pod-"+k8sName, k8sName, k8sName)
		_, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}
}

// readConfigMapKey returns the raw string stored at key inside the given
// ConfigMap. Used by tests that assert on the topology config emitted by
// GenerateOutput.
func readConfigMapKey(t *testing.T, ctx context.Context, client *fake.Clientset, namespace, name, key string) string {
	t.Helper()
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)
	return cm.Data[key]
}

func TestResolveSlurmNodeName(t *testing.T) {
	testCases := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "label takes precedence over hostname",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{topology.KeySlurmNodeName: "slurm-node-1"}},
				Spec:       corev1.PodSpec{Hostname: "host-1"},
			},
			want: "slurm-node-1",
		},
		{
			name: "present but empty label yields empty, no hostname fallback",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{topology.KeySlurmNodeName: ""}},
				Spec:       corev1.PodSpec{Hostname: "host-1"},
			},
			want: "",
		},
		{
			name: "falls back to hostname when label absent",
			pod:  &corev1.Pod{Spec: corev1.PodSpec{Hostname: "host-1"}},
			want: "host-1",
		},
		{
			name: "empty when neither label nor hostname set",
			pod:  &corev1.Pod{},
			want: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, resolveSlurmNodeName(tc.pod))
		})
	}
}

func TestGetClusterNodes(t *testing.T) {
	namespace := "test-ns"
	podSel := metav1.LabelSelector{MatchLabels: map[string]string{"app": "slinky"}}
	sel, err := metav1.LabelSelectorAsSelector(&podSel)
	require.NoError(t, err)

	readyCond := []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	pod := func(name, nodeName, slurmLabel, hostname string, ready bool) *corev1.Pod {
		labels := map[string]string{"app": "slinky"}
		if slurmLabel != "" {
			labels[topology.KeySlurmNodeName] = slurmLabel
		}
		status := corev1.PodStatus{Phase: corev1.PodRunning}
		if ready {
			status.Conditions = readyCond
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
			Spec:       corev1.PodSpec{NodeName: nodeName, Hostname: hostname},
			Status:     status,
		}
	}

	// pod-empty-label carries the slurm.node.name label with an empty value (distinct
	// from the label being absent): resolveSlurmNodeName must not fall back to the
	// hostname, so the guard still skips it.
	emptyLabelPod := pod("pod-empty-label", "k8s-5", "", "host-5", true)
	emptyLabelPod.Labels[topology.KeySlurmNodeName] = ""

	client := fake.NewSimpleClientset(
		pod("pod-label", "k8s-1", "slurm-1", "", true),     // mapped via label
		pod("pod-hostname", "k8s-2", "", "slurm-2", true),  // mapped via hostname fallback
		pod("pod-empty", "k8s-3", "", "", true),            // skipped: ready but no SLURM name (the guard)
		pod("pod-notready", "k8s-4", "slurm-4", "", false), // skipped: not Ready
		emptyLabelPod, // skipped: label present but empty (no hostname fallback)
	)
	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:   namespace,
			podListOpt:  &metav1.ListOptions{LabelSelector: sel.String()},
			nodeListOpt: &metav1.ListOptions{},
		},
	}

	clusterNodes, httpErr := eng.getClusterNodes(context.Background())
	require.Nil(t, httpErr)
	require.Equal(t, map[string]string{"k8s-1": "slurm-1", "k8s-2": "slurm-2"}, clusterNodes.nodeMap)
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
			topo, _ := model.ToGraph(nil)

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

// TestGetParametersBlockHostnameRegexValidation verifies parameter-time
// validation for blockHostnameRegex: it must be compatible with the config
// update mode and must not disagree between engine-level and per-partition
// settings.
func TestGetParametersBlockHostnameRegexValidation(t *testing.T) {
	baseParams := func(extras map[string]any) map[string]any {
		p := map[string]any{
			topology.KeyNamespace:         "test-ns",
			topology.KeyPodSelector:       map[string]any{"matchLabels": map[string]string{"app": "slurm"}},
			topology.KeyTopoConfigPath:    "topology.conf",
			topology.KeyTopoConfigmapName: "slurm-config",
		}
		for k, v := range extras {
			p[k] = v
		}
		return p
	}

	t.Run("regex with configUpdateMode none is rejected", func(t *testing.T) {
		params := baseParams(map[string]any{
			"blockHostnameRegex": `d(\d+)-T\d+`,
			"configUpdateMode":   "none",
		})
		_, err := getParameters(params)
		require.ErrorContains(t, err, `blockHostnameRegex is incompatible with configUpdateMode "none"`)
	})

	t.Run("per-partition regex with configUpdateMode none is rejected", func(t *testing.T) {
		params := baseParams(map[string]any{
			"configUpdateMode": "none",
			"topologies": map[string]any{
				"gpu-block": map[string]any{
					"plugin":             topology.TopologyBlock,
					"nodes":              []string{"n1"},
					"blockHostnameRegex": `d(\d+)-T\d+`,
				},
			},
		})
		_, err := getParameters(params)
		require.ErrorContains(t, err, `blockHostnameRegex is incompatible with configUpdateMode "none"`)
	})

	t.Run("conflicting per-partition regex is rejected", func(t *testing.T) {
		params := baseParams(map[string]any{
			"topologies": map[string]any{
				"a": map[string]any{
					"plugin":             topology.TopologyBlock,
					"nodes":              []string{"n1"},
					"blockHostnameRegex": `d(\d+)-T\d+`,
				},
				"b": map[string]any{
					"plugin":             topology.TopologyBlock,
					"nodes":              []string{"n2"},
					"blockHostnameRegex": `nvl(\d+)`,
				},
			},
		})
		_, err := getParameters(params)
		require.ErrorContains(t, err, `conflicts with cluster-wide or another partition value`)
	})

	t.Run("matching engine and partition regex is accepted", func(t *testing.T) {
		params := baseParams(map[string]any{
			"blockHostnameRegex": `d(\d+)-T\d+`,
			"topologies": map[string]any{
				"gpu-block": map[string]any{
					"plugin":             topology.TopologyBlock,
					"nodes":              []string{"n1"},
					"blockHostnameRegex": `d(\d+)-T\d+`,
				},
			},
		})
		p, err := getParameters(params)
		require.NoError(t, err)
		require.Equal(t, `d(\d+)-T\d+`, p.BlockHostnameRegex)
	})
}

// TestWithHostnameRegexDomainsPhysicalInventory verifies that
// withHostnameRegexDomains includes every selected physical Kubernetes node,
// whether or not it has a Ready slurmd pod. Live nodes contribute host entries
// keyed by their SLURM name; nodes without a Ready pod ensure the block
// remains declared with zero live hosts so its ID stays reserved.
func TestWithHostnameRegexDomainsPhysicalInventory(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "nvl72d001-T01",
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-0"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "nvl72d002-T01",
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-1"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				// No Ready pod for this physical node: still contributes an
				// empty block declaration so retained topology annotations
				// stay valid.
				Name:        "nvl72d003-T01",
				Annotations: map[string]string{topology.KeyNodeInstance: "instance-2"},
			},
		},
	}
	for _, node := range nodes {
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	for _, pod := range []*corev1.Pod{
		makeReadySlurmdPod("pod-0", "nvl72d001-T01", "nvl72d001-T01"),
		makeReadySlurmdPod("pod-1", "nvl72d002-T01", "nvl72d002-T01"),
	} {
		_, err := client.CoreV1().Pods("test-ns").Create(ctx, pod, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:   "test-ns",
			podListOpt:  &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt: &metav1.ListOptions{},
		},
	}
	clusterNodes, httpErr := eng.getClusterNodes(ctx)
	require.Nil(t, httpErr)

	got, httpErr := withHostnameRegexDomains(&topology.Graph{}, clusterNodes, `d(\d+)-T\d+`)
	require.Nil(t, httpErr)
	require.NotNil(t, got)

	// Every physical block is declared. Blocks 001 and 002 hold their Ready
	// pod's SLURM name; block 003 exists with zero hosts.
	require.Contains(t, got.Domains, "001")
	require.Contains(t, got.Domains, "002")
	require.Contains(t, got.Domains, "003")

	require.Len(t, got.Domains["001"], 1)
	require.Contains(t, got.Domains["001"], "nvl72d001-T01")
	require.Len(t, got.Domains["002"], 1)
	require.Contains(t, got.Domains["002"], "nvl72d002-T01")
	require.Len(t, got.Domains["003"], 0, "block 003 has no Ready pod and must be empty")
}

// TestWithHostnameRegexDomainsRejectsInvalidRegex verifies validation.
// Regex-level errors (syntax, capture count) fail up front; per-hostname
// data errors are logged and skipped, only failing hard when no node matches.
func TestWithHostnameRegexDomainsRejectsInvalidRegex(t *testing.T) {
	baseNodes := &corev1.NodeList{
		Items: []corev1.Node{
			{ObjectMeta: metav1.ObjectMeta{Name: "nvl72d001-T01"}},
		},
	}
	cn := &clusterNodes{nodes: baseNodes, nodeMap: map[string]string{}}

	t.Run("invalid regex", func(t *testing.T) {
		_, err := withHostnameRegexDomains(&topology.Graph{}, cn, `d(\d+`)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "invalid blockHostnameRegex")
	})
	t.Run("multiple capture groups", func(t *testing.T) {
		_, err := withHostnameRegexDomains(&topology.Graph{}, cn, `d(\d+)-T(\d+)`)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "exactly one capture group")
	})
	t.Run("non-matching hostname is skipped", func(t *testing.T) {
		_, err := withHostnameRegexDomains(&topology.Graph{}, cn, `sn(\d+)`)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "matched no nodes")
	})
	t.Run("non-decimal capture is skipped", func(t *testing.T) {
		_, err := withHostnameRegexDomains(&topology.Graph{}, cn, `nvl72d(\d+-T)\d+`)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "matched no nodes")
	})
	t.Run("partial match succeeds", func(t *testing.T) {
		mixed := &corev1.NodeList{
			Items: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "nvl72d001-T01"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "unrelated-node"}},
			},
		}
		graph, err := withHostnameRegexDomains(&topology.Graph{}, &clusterNodes{
			nodes: mixed, nodeMap: map[string]string{"nvl72d001-T01": "nvl72d001-T01"},
		}, `d(\d+)-T\d+`)
		require.Nil(t, err)
		require.NotNil(t, graph)
		require.Contains(t, graph.Domains, "001")
	})
}

// TestGenerateOutputPodScaleStability verifies the pod-scale fix end-to-end
// against a single shared client and engine across pod add/remove
// transitions, so each phase asserts against the ConfigMap left by the
// previous generation.
func TestGenerateOutputPodScaleStability(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	physical := []string{"nvl72d001-T01", "nvl72d002-T01", "nvl72d003-T01"}
	createPhysicalNodes(t, ctx, client, physical)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			BaseParams: slurm.BaseParams{
				Plugin:             topology.TopologyBlock,
				BlockSizes:         []int{1},
				BlockHostnameRegex: `d(\d+)-T\d+`,
			},
			Namespace:       "test-ns",
			ConfigMapName:   "slurm-config",
			ConfigPath:      "topology.conf",
			UseDynamicNodes: true,
			podListOpt:      &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt:     &metav1.ListOptions{},
		},
	}
	readTopology := func() string {
		return readConfigMapKey(t, ctx, client, "test-ns", "slurm-config", "topology.conf")
	}

	// Initial state: all three pods Ready.
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)
	result, httpErr := eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)
	require.Equal(t, []byte("OK\n"), result)

	initial := readTopology()
	require.Contains(t, initial, "BlockName=block001 Nodes=nvl72d001-T01")
	require.Contains(t, initial, "BlockName=block002 Nodes=nvl72d002-T01")
	require.Contains(t, initial, "BlockName=block003 Nodes=nvl72d003-T01")

	// Pod scale: remove the pod backing block 002. Block 002 stays declared,
	// blocks 001/003 keep their IDs, no renumbering occurs.
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", []string{"nvl72d001-T01", "nvl72d003-T01"})
	_, httpErr = eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)

	afterScale := readTopology()
	require.Contains(t, afterScale, "BlockName=block001 Nodes=nvl72d001-T01")
	require.Contains(t, afterScale, "BlockName=block002\n", "block002 should stay declared but empty when its pod leaves")
	require.NotContains(t, afterScale, "BlockName=block002 Nodes=", "block002 must not point to a different physical block after pod removal")
	require.Contains(t, afterScale, "BlockName=block003 Nodes=nvl72d003-T01",
		"block003 must not be renamed to block002 when block002's pod leaves")

	// Pod returns: ConfigMap must match the initial byte string exactly.
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)
	_, httpErr = eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)
	require.Equal(t, initial, readTopology(),
		"topology.conf must return to the exact initial state when the missing pod returns")
}

// TestGenerateOutputSkeletonOnlyWithRegex verifies skeleton-only mode
// preserves stable block IDs and declares every physical block with no
// Nodes= line.
func TestGenerateOutputSkeletonOnlyWithRegex(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	physical := []string{"nvl72d001-T01", "nvl72d002-T01", "nvl72d003-T01"}
	createPhysicalNodes(t, ctx, client, physical)
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			BaseParams: slurm.BaseParams{
				Plugin:             topology.TopologyBlock,
				BlockSizes:         []int{1},
				BlockHostnameRegex: `d(\d+)-T\d+`,
			},
			Namespace:        "test-ns",
			ConfigMapName:    "slurm-config",
			ConfigPath:       "topology.conf",
			ConfigUpdateMode: ConfigUpdateModeSkeletonOnly,
			UseDynamicNodes:  true,
			podListOpt:       &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt:      &metav1.ListOptions{},
		},
	}

	_, httpErr := eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)

	cm, err := client.CoreV1().ConfigMaps("test-ns").Get(ctx, "slurm-config", metav1.GetOptions{})
	require.NoError(t, err)
	got := cm.Data["topology.conf"]

	for _, id := range []string{"block001", "block002", "block003"} {
		require.Contains(t, got, "BlockName="+id+"\n",
			"skeleton-only + regex must declare %s without a Nodes= line", id)
	}
	require.NotContains(t, got, " Nodes=",
		"skeleton-only output must not contain Nodes= entries: %s", got)
}

// TestGenerateOutputRejectsSplitInStableIDMode verifies the plan's
// "one captured index identifies one emitted physical base block" contract:
// if blockSizes[0] is smaller than a physical block's host count, generation
// fails with an actionable error rather than emitting suffixed IDs.
func TestGenerateOutputRejectsSplitInStableIDMode(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	// Two physical nodes share the same captured index (both are in block 001).
	// blockSizes[0]=1 would require splitting them into two base blocks.
	physical := []string{"nvl72d001-T01", "nvl72d001-T02"}
	createPhysicalNodes(t, ctx, client, physical)
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			BaseParams: slurm.BaseParams{
				Plugin:             topology.TopologyBlock,
				BlockSizes:         []int{1, 2},
				BlockHostnameRegex: `d(\d+)-T\d+`,
			},
			Namespace:       "test-ns",
			ConfigMapName:   "slurm-config",
			ConfigPath:      "topology.conf",
			UseDynamicNodes: true,
			podListOpt:      &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt:     &metav1.ListOptions{},
		},
	}
	_, httpErr := eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "would be split")
}

// TestWithHostnameRegexDomainsPrefersSlurmNameForLiveNodes pins the
// documented hostname-source contract: live nodes are grouped by the digit
// captured from the SLURM name (the string that appears in topology.conf);
// nodes without a Ready pod fall back to the Kubernetes node name.
func TestWithHostnameRegexDomainsPrefersSlurmNameForLiveNodes(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	// K8s node "worker-42" hosts a Ready slurmd pod whose SLURM name is
	// "nvl72d001-T01". The regex captures "001" from the SLURM name and
	// "42" from the K8s name. Ready node must group into block 001.
	live := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "worker-42",
			Annotations: map[string]string{topology.KeyNodeInstance: "i-live"},
		},
	}
	_, err := client.CoreV1().Nodes().Create(ctx, live, metav1.CreateOptions{})
	require.NoError(t, err)
	// Non-Ready node without a slurmd pod. Its K8s name yields digits "99".
	nonReady := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-99",
		},
	}
	_, err = client.CoreV1().Nodes().Create(ctx, nonReady, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.CoreV1().Pods("test-ns").Create(ctx,
		makeReadySlurmdPod("pod-live", "worker-42", "nvl72d001-T01"), metav1.CreateOptions{})
	require.NoError(t, err)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:   "test-ns",
			podListOpt:  &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt: &metav1.ListOptions{},
		},
	}
	cn, httpErr := eng.getClusterNodes(ctx)
	require.Nil(t, httpErr)

	// Regex matches decimal digits after either "d" or "worker-".
	got, httpErr := withHostnameRegexDomains(&topology.Graph{}, cn, `(?:d|worker-)(\d+)`)
	require.Nil(t, httpErr)
	require.NotNil(t, got)

	// Live node routed by SLURM-name capture "001".
	require.Contains(t, got.Domains, "001")
	require.Contains(t, got.Domains["001"], "nvl72d001-T01",
		"live node must group by SLURM-name capture, not K8s-name capture")

	// Non-Ready node routed by K8s-name capture "99".
	require.Contains(t, got.Domains, "99")
	require.Empty(t, got.Domains["99"], "non-Ready node must contribute an empty declaration")

	// The live node's K8s-name capture "42" must not create a spurious block.
	require.NotContains(t, got.Domains, "42",
		"live node's K8s-name capture must not create a separate block")
}

// TestGenerateOutputPodScaleStabilityPerPartition mirrors
// TestGenerateOutputPodScaleStability but exercises the per-partition code
// path (getBlockTopologyUnit / blockNameForPartition) instead of the
// cluster-wide toBlockTopology path. Both C1 (blockInfo.id propagation with
// empty BlockSizes) and M1 (empty block declaration for zero-live-pod
// blocks) live in the per-partition path.
func TestGenerateOutputPodScaleStabilityPerPartition(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	physical := []string{"nvl72d001-T01", "nvl72d002-T01", "nvl72d003-T01"}
	createPhysicalNodes(t, ctx, client, physical)

	eng := &SlinkyEngine{
		client: client,
		params: &Params{
			Namespace:     "test-ns",
			ConfigMapName: "slurm-config",
			ConfigPath:    "topology.conf",
			Topologies: map[string]*Topology{
				"gpu-block": {
					Topology: slurm.Topology{
						Plugin:             topology.TopologyBlock,
						BlockSizes:         []int{1},
						BlockHostnameRegex: `d(\d+)-T\d+`,
						Nodes:              physical,
					},
				},
			},
			UseDynamicNodes: true,
			podListOpt:      &metav1.ListOptions{LabelSelector: "app=slinky"},
			nodeListOpt:     &metav1.ListOptions{},
		},
	}
	readTopology := func() string {
		return readConfigMapKey(t, ctx, client, "test-ns", "slurm-config", "topology.conf")
	}

	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)
	_, httpErr := eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)
	initial := readTopology()
	require.Contains(t, initial, "block: block001")
	require.Contains(t, initial, "block: block002")
	require.Contains(t, initial, "block: block003")
	require.Contains(t, initial, "nodes: nvl72d001-T01")
	require.Contains(t, initial, "nodes: nvl72d003-T01")

	// Remove block002's pod: the empty declaration must remain and no
	// block gets renumbered.
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", []string{"nvl72d001-T01", "nvl72d003-T01"})
	_, httpErr = eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)
	afterScale := readTopology()
	require.Contains(t, afterScale, "block: block001")
	require.Contains(t, afterScale, "block: block002",
		"block002 must remain declared even after its pod leaves")
	require.Contains(t, afterScale, "block: block003",
		"block003 must not be renamed to block002 when block002's pod leaves")
	require.Contains(t, afterScale, "nodes: nvl72d001-T01")
	require.Contains(t, afterScale, "nodes: nvl72d003-T01")

	// Pod returns: config must return to the exact initial state.
	reconcileReadySlurmdPods(t, ctx, client, "test-ns", physical)
	_, httpErr = eng.GenerateOutput(ctx, &topology.Graph{Domains: topology.NewDomainMap()}, nil)
	require.Nil(t, httpErr)
	require.Equal(t, initial, readTopology(),
		"per-partition topology must return to the exact initial state when the missing pod returns")
}
