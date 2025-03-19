/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"context"
	"fmt"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testNodeInfo struct{}

func (h *testNodeInfo) NodeRegionCommand() []string                  { return []string{"region"} }
func (h *testNodeInfo) ProcessNodeRegionOutput(data string) string   { return data }
func (h *testNodeInfo) NodeInstanceCommand() []string                { return []string{"instance"} }
func (h *testNodeInfo) ProcessNodeInstanceOutput(data string) string { return "instance-" + data }

func execPod(ctx context.Context, pod *v1.Pod, cmd []string) (string, error) {
	switch cmd[0] {
	case "region":
		return "region", nil
	case "instance":
		return pod.Spec.NodeName, nil
	default:
		return "", fmt.Errorf("error")
	}
}

func TestGetComputeInstances(t *testing.T) {
	var nodeInfo testNodeInfo
	ownerReferences := []metav1.OwnerReference{
		{
			Kind: "DaemonSet",
			Name: "ds",
		},
	}

	podList := &v1.PodList{
		Items: []v1.Pod{
			// no OwnerReferences
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod2",
					Namespace:       "default",
					OwnerReferences: ownerReferences,
				},
				Spec: v1.PodSpec{
					NodeName: "node2",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "pod3",
					Namespace:       "default",
					OwnerReferences: ownerReferences,
				},
				Spec: v1.PodSpec{
					NodeName: "node3",
				},
			},
		},
	}
	cis := getComputeInstances(context.TODO(), &nodeInfo, "ds", podList, execPod)

	expected := []topology.ComputeInstances{
		{
			Region: "region",
			Instances: map[string]string{
				"instance-node2": "node2",
				"instance-node3": "node3",
			},
		},
	}

	require.Equal(t, expected, cis)
}
