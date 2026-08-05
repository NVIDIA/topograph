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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (eng *K8sEngine) ResolveComputeInstances(ctx context.Context, instances []topology.ComputeInstances, _ any) ([]topology.ComputeInstances, *httperr.Error) {
	nodes, err := k8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	if len(instances) != 0 {
		eng.cacheRequestedNodes(instances, nodes)
		return instances, nil
	}
	eng.cacheNodes(nodes)
	return k8s.GetComputeInstances(nodes), nil
}

func (eng *K8sEngine) AddNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	if err := eng.loadNodes(ctx); err != nil {
		return err
	}

	node, expected := eng.cachedNodeMap[nodeName]
	if !expected {
		klog.Warningf("Skipping topology labels for node %q because it is not part of the resolved compute instances", nodeName)
		return nil
	}
	if node == nil {
		var err error
		node, err = eng.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("node %q was not found in Kubernetes", nodeName)
		}
		if err != nil {
			return fmt.Errorf("failed to get node %q: %w", nodeName, err)
		}
		eng.cachedNodeMap[nodeName] = node
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
	if eng.cachedNodeMap != nil {
		return nil
	}

	nodes, err := k8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
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
	eng.cachedNodeMap = make(map[string]*corev1.Node, len(nodes.Items))
	for i := range nodes.Items {
		node := &nodes.Items[i]
		eng.cachedNodeMap[node.Name] = node
	}
}

func (eng *K8sEngine) cacheRequestedNodes(instances []topology.ComputeInstances, nodes *corev1.NodeList) {
	eng.cachedNodeMap = make(map[string]*corev1.Node)
	for _, region := range instances {
		for _, nodeName := range region.Instances {
			eng.cachedNodeMap[nodeName] = nil
		}
	}

	for i := range nodes.Items {
		node := &nodes.Items[i]
		if _, requested := eng.cachedNodeMap[node.Name]; requested {
			eng.cachedNodeMap[node.Name] = node
		}
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

	labels = skipXclrLabelsWhenGPUCliqueExists(desired, labels, keys)
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
	if key == topology.KeyTopologyXclrDomain || key == topology.KeyTopologyXclrSubDomain {
		return true
	}
	configuredKeys := append(append([]string(nil), keys.Fabric...), keys.XclrDomain, keys.XclrSubDomain)
	for _, configured := range configuredKeys {
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

func skipXclrLabelsWhenGPUCliqueExists(currentLabels, labels map[string]string, keys *TopologyLabelKeys) map[string]string {
	xclrDomainLabel := keys.XclrDomainKey()
	xclrSubDomainLabel := keys.XclrSubDomainKey()
	if strings.TrimSpace(currentLabels[topology.KeyNvidiaGPUClique]) == "" {
		return labels
	}

	filtered := maps.Clone(labels)
	delete(filtered, xclrDomainLabel)
	delete(filtered, xclrSubDomainLabel)

	return filtered
}
