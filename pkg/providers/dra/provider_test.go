/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"context"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetParameters(t *testing.T) {
	testCases := []struct {
		name   string
		params map[string]any
		ret    *Params
		err    string
	}{
		{
			name:   "Case 1: no params",
			params: nil,
			ret:    &Params{},
		},
		{
			name:   "Case 2: bad params",
			params: map[string]any{"nodeSelector": .1},
			err:    "could not decode configuration: 1 error(s) decoding:\n\n* 'nodeSelector' expected a map, got 'float64'",
		},
		{
			name:   "Case 3: valid input",
			params: map[string]any{"nodeSelector": map[string]string{"key": "val"}},
			ret: &Params{
				NodeSelector: map[string]string{"key": "val"},
				nodeListOpt: &metav1.ListOptions{
					LabelSelector: "key=val",
				},
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

func TestGenerateTopologyConfigUsesAnnotatedInstanceID(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "k8s-node-1",
		Labels: map[string]string{
			DomainLabel: "clique-1",
		},
		Annotations: map[string]string{
			topology.KeyNodeInstance: "instance-123",
			topology.KeyNodeRegion:   "local",
		},
	}}
	provider := &Provider{
		client: fake.NewSimpleClientset(node),
		params: &Params{},
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
