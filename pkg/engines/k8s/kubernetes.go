/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"context"
	"maps"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (eng *K8sEngine) GetComputeInstances(ctx context.Context, _ any) ([]topology.ComputeInstances, *httperr.Error) {
	nodes, err := k8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	return k8s.GetComputeInstances(nodes), nil
}

func (eng *K8sEngine) AddNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	klog.Infof("Applying labels on node %s : %v", nodeName, labels)
	node, err := eng.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	MergeNodeLabels(node, labels, eng.params.labelKeys)

	_, err = eng.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})

	return err
}

func MergeNodeLabels(node *corev1.Node, labels map[string]string, keys *TopologyLabelKeys) {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	labels = skipAcceleratorLabelWhenGPUCliqueExists(node, labels, keys)
	removeManagedTopologyLabels(node.Labels, keys)
	maps.Copy(node.Labels, labels)
}

func removeManagedTopologyLabels(labels map[string]string, keys *TopologyLabelKeys) {
	for key := range labels {
		if key == topology.KeyNvidiaGPUClique {
			continue
		}
		if isManagedLevelLabel(key, keys) {
			delete(labels, key)
		}
	}
}

func isManagedLevelLabel(key string, keys *TopologyLabelKeys) bool {
	if key == topology.KeyTopologyAccelerator {
		return true
	}
	for _, configured := range append(append([]string(nil), keys.Fabric...), keys.Accelerator) {
		if configured != "" && key == configured {
			return true
		}
	}
	for _, prefix := range []string{topology.KeyFabricTierPrefix} {
		if strings.HasPrefix(key, prefix) {
			level, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
			if err == nil && level >= 0 {
				return true
			}
		}
	}
	return false
}

func skipAcceleratorLabelWhenGPUCliqueExists(node *corev1.Node, labels map[string]string, keys *TopologyLabelKeys) map[string]string {
	acceleratorLabel := keys.AcceleratorKey()
	if strings.TrimSpace(node.Labels[topology.KeyNvidiaGPUClique]) == "" {
		return labels
	}

	filtered := maps.Clone(labels)
	delete(filtered, acceleratorLabel)

	if acceleratorLabel != topology.KeyNvidiaGPUClique {
		delete(node.Labels, acceleratorLabel)
	}

	return filtered
}
