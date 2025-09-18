/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
				{
					Region:    "r1",
					Instances: map[string]string{"i1": "node1"},
				},
			},
		},
		{
			name:  "Case 2: missing region",
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
			nodes: &corev1.NodeList{Items: []corev1.Node{node1, node2, node3}},
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
			cis, err := getComputeInstances(tc.nodes)
			require.NoError(t, err)
			require.Equal(t, tc.cis, cis)
		})
	}
}

func TestMergeNodeLabels(t *testing.T) {
	testCases := []struct {
		name string
		node *corev1.Node
		in   map[string]string
		out  map[string]string
	}{
		{
			name: "Case 1: no labels",
			node: &corev1.Node{},
			out:  map[string]string{},
		},
		{
			name: "Case 2: copy",
			node: &corev1.Node{},
			in:   map[string]string{"a": "1", "b": "2"},
			out:  map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "Case 3: merge",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"a": "1", "b": "2", "c": "x"},
					Annotations: map[string]string{"a": "1", "b": "2", "c": "x"},
				},
			},
			in:  map[string]string{"c": "3", "d": "4"},
			out: map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			MergeNodeLabels(tc.node, tc.in)
			require.Equal(t, tc.out, tc.node.Labels)
		})
	}
}
