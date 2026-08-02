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

package lldp

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func providersConfig(params map[string]any) providers.Config {
	return providers.Config{Params: params}
}

func TestGetK8SParameters(t *testing.T) {
	params, err := getK8SParameters(map[string]any{
		"interfaces":   []string{"eno1"},
		"nodeSelector": map[string]string{"fabric": "ethernet"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"eno1"}, params.Interfaces)
	require.Equal(t, "fabric=ethernet", params.nodeListOpt.LabelSelector)
}

func TestGetNodeAnnotations(t *testing.T) {
	runner := func(_ context.Context, executable string, args []string, _ map[string]string) (*bytes.Buffer, error) {
		require.Equal(t, "lldpctl", executable)
		require.Equal(t, []string{"-f", "json"}, args)
		return bytes.NewBufferString(singleNeighborJSON), nil
	}

	annotations, err := getNodeAnnotations(context.Background(), "node-1", map[string]string{"interfaces": "eno1"}, runner)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		topology.KeyNodeInstance:  "node-1",
		topology.KeyNodeRegion:    "local",
		topology.KeyLLDPChassisID: "mac:00:11:22:33:44:55",
	}, annotations)
}

func TestGetNodeAnnotationsClearsMissingNeighbor(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(`{"lldp":{}}`), nil
	}

	annotations, err := getNodeAnnotations(context.Background(), "node-1", nil, runner)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		topology.KeyNodeInstance:  "node-1",
		topology.KeyNodeRegion:    "local",
		topology.KeyLLDPChassisID: "",
	}, annotations)
}

func TestGetNodeAnnotationsRejectsAmbiguity(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(multipleNeighborsJSON), nil
	}

	annotations, err := getNodeAnnotations(context.Background(), "node-1", nil, runner)
	require.ErrorContains(t, err, "multiple LLDP switches found")
	require.Nil(t, annotations)
}

func TestProviderK8SGenerateTopologyConfig(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "k8s-node-1",
		Labels: map[string]string{"fabric": "ethernet"},
		Annotations: map[string]string{
			topology.KeyNodeInstance:  "instance-1",
			topology.KeyNodeRegion:    "local",
			topology.KeyLLDPChassisID: "mac:00:11:22:33:44:55",
		},
	}}
	provider := &ProviderK8S{
		client: fake.NewSimpleClientset(node),
		params: &K8SParams{nodeListOpt: &metav1.ListOptions{LabelSelector: "fabric=ethernet"}},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"instance-1": "k8s-node-1"}}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-001122334455")
	require.Contains(t, graph.Tiers.Vertices["lldp-001122334455"].Vertices, "instance-1")
}

func TestProviderK8SGeneratesNoTopologyWithoutChassisAnnotation(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "k8s-node-1",
		Annotations: map[string]string{
			topology.KeyNodeInstance: "instance-1",
			topology.KeyNodeRegion:   "local",
		},
	}}
	provider := &ProviderK8S{
		client: fake.NewSimpleClientset(node),
		params: &K8SParams{},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"instance-1": "k8s-node-1"}}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, topology.NoTopology)
	require.Contains(t, graph.Tiers.Vertices[topology.NoTopology].Vertices, "instance-1")
}
