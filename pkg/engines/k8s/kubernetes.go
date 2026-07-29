/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (eng *K8sEngine) ResolveComputeInstances(ctx context.Context, instances []topology.ComputeInstances, _ any) ([]topology.ComputeInstances, *httperr.Error) {
	listOptions := eng.params.nodeListOpt
	if len(instances) != 0 {
		// Cache every Node so output generation can distinguish a nonexistent
		// requested node from one intentionally excluded by nodeSelector.
		listOptions = nil
	}

	nodes, err := k8s.GetNodes(ctx, eng.client, listOptions)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	eng.cacheNodes(nodes)
	if len(instances) != 0 {
		return instances, nil
	}
	return k8s.GetComputeInstances(nodes), nil
}

func (eng *K8sEngine) AddNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	if err := eng.loadNodes(ctx); err != nil {
		return err
	}

	node, ok := eng.cachedNodeMap[nodeName]
	if !ok {
		return fmt.Errorf("node %q was not found in Kubernetes", nodeName)
	}
	if !eng.matchesNodeSelector(node) {
		klog.Warningf("Skipping topology labels for node %q because it does not match the engine nodeSelector", nodeName)
		return nil
	}

	desiredLabels := mergeNodeLabels(node.Labels, labels, eng.params.labelKeys)
	if maps.Equal(node.Labels, desiredLabels) {
		return nil
	}

	patchData, err := nodeLabelPatch(node.Labels, desiredLabels)
	if err != nil {
		return fmt.Errorf("failed to create label patch for node %q: %w", nodeName, err)
	}

	klog.Infof("Updating topology labels on node %s: %v", nodeName, labels)
	_, err = eng.client.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.StrategicMergePatchType,
		patchData,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch topology labels on node %q: %w", nodeName, err)
	}

	node.Labels = desiredLabels
	return nil
}

func (eng *K8sEngine) loadNodes(ctx context.Context) error {
	if eng.cachedNodes != nil {
		return nil
	}

	// Without a preceding ResolveComputeInstances call, load every Node so a
	// cache miss still means the Node does not exist rather than merely being
	// excluded by nodeSelector.
	nodes, err := k8s.GetNodes(ctx, eng.client, nil)
	if err != nil {
		return err
	}
	eng.cacheNodes(nodes)
	return nil
}

func (eng *K8sEngine) matchesNodeSelector(node *corev1.Node) bool {
	if len(eng.params.NodeSelector) == 0 {
		return true
	}
	selector := labels.SelectorFromSet(eng.params.NodeSelector)
	return selector.Matches(labels.Set(node.Labels))
}

func (eng *K8sEngine) cacheNodes(nodes *corev1.NodeList) {
	eng.cachedNodes = nodes
	eng.cachedNodeMap = make(map[string]*corev1.Node, len(nodes.Items))
	for i := range nodes.Items {
		node := &nodes.Items[i]
		eng.cachedNodeMap[node.Name] = node
	}
}

func nodeLabelPatch(current, desired map[string]string) ([]byte, error) {
	changes := make(map[string]any)
	for key := range current {
		if _, ok := desired[key]; !ok {
			changes[key] = nil
		}
	}
	for key, value := range desired {
		if currentValue, ok := current[key]; !ok || currentValue != value {
			changes[key] = value
		}
	}

	return json.Marshal(map[string]any{
		"metadata": map[string]any{
			"labels": changes,
		},
	})
}

func mergeNodeLabels(current, labels map[string]string, keys *TopologyLabelKeys) map[string]string {
	desired := maps.Clone(current)
	if desired == nil {
		desired = make(map[string]string)
	}

	labels = skipAcceleratorLabelWhenGPUCliqueExists(desired, labels, keys)
	removeManagedTopologyLabels(desired, keys)
	maps.Copy(desired, labels)
	return desired
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

func skipAcceleratorLabelWhenGPUCliqueExists(nodeLabels, labels map[string]string, keys *TopologyLabelKeys) map[string]string {
	acceleratorLabel := keys.AcceleratorKey()
	if strings.TrimSpace(nodeLabels[topology.KeyNvidiaGPUClique]) == "" {
		return labels
	}

	filtered := maps.Clone(labels)
	delete(filtered, acceleratorLabel)

	if acceleratorLabel != topology.KeyNvidiaGPUClique {
		delete(nodeLabels, acceleratorLabel)
	}

	return filtered
}
