/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"context"
	"testing"

	"github.com/NVIDIA/topograph/pkg/accelerator"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewAcceleratorDiscoverer(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		labelKey string
		err      string
	}{
		{name: "legacy default", labelKey: defaultDomainLabel},
		{
			name: "configured label",
			params: map[string]any{"accelerator": map[string]any{
				"source":          accelerator.SourceKubernetesLabel,
				"kubernetesLabel": map[string]any{"key": "example.com/domain"},
			}},
			labelKey: "example.com/domain",
		},
		{
			name:   "unsupported source",
			params: map[string]any{"accelerator": map[string]any{"source": accelerator.SourceNone}},
			err:    `dra provider supports only accelerator source "kubernetes-label"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			discoverer, labelKey, err := newAcceleratorDiscoverer(test.params)
			if test.err != "" {
				require.EqualError(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.labelKey, labelKey)
			assignments, err := discoverer.Discover(context.Background(), []accelerator.Target{{
				InstanceID: "instance-1",
				Labels:     map[string]string{test.labelKey: "domain-1"},
			}})
			require.NoError(t, err)
			require.Equal(t, accelerator.Assignments{
				"instance-1": {DomainID: "domain-1"},
			}, assignments)
		})
	}
}

func TestGenerateTopologyConfigUsesAnnotatedInstanceID(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "k8s-node-1",
		Labels: map[string]string{
			defaultDomainLabel: "clique-1",
		},
		Annotations: map[string]string{
			topology.KeyNodeInstance: "instance-123",
			topology.KeyNodeRegion:   "local",
		},
	}}
	provider := &Provider{
		client:           fake.NewSimpleClientset(node),
		accelerator:      mustAcceleratorDiscoverer(t, defaultDomainLabel),
		acceleratorLabel: defaultDomainLabel,
	}
	instances := []topology.ComputeInstances{{
		Region: "local",
		Instances: map[string]string{
			"instance-123": "scheduler-node-1",
		},
	}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, instances)

	require.Nil(t, httpErr)
	expectedDomains := topology.NewDomainMap()
	expectedDomains.AddHost("clique-1", "instance-123", "scheduler-node-1")
	require.Equal(t, expectedDomains, graph.Domains)
}

func TestGenerateTopologyConfigRejectsMissingDomainLabels(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "k8s-node-1",
		Annotations: map[string]string{
			topology.KeyNodeInstance: "instance-123",
			topology.KeyNodeRegion:   "local",
		},
	}}
	provider := &Provider{
		client:           fake.NewSimpleClientset(node),
		accelerator:      mustAcceleratorDiscoverer(t, defaultDomainLabel),
		acceleratorLabel: defaultDomainLabel,
	}
	instances := []topology.ComputeInstances{{
		Region: "local",
		Instances: map[string]string{
			"instance-123": "scheduler-node-1",
		},
	}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, instances)

	require.Nil(t, graph)
	require.NotNil(t, httpErr)
	require.Equal(t, 502, httpErr.Code())
	require.ErrorContains(t, httpErr, `no matching nodes found; check label "nvidia.com/gpu.clique"`)
}

func mustAcceleratorDiscoverer(t *testing.T, labelKey string) accelerator.Discoverer {
	t.Helper()
	discoverer, err := accelerator.NewKubernetesDiscoverer(accelerator.KubernetesLabelSection(labelKey))
	require.NoError(t, err)
	return discoverer
}
