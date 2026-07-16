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

package k8s

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
	return getComputeInstances(nodes), nil
}

func getComputeInstances(nodes *corev1.NodeList) []topology.ComputeInstances {
	regions := make(map[string]map[string]string)
	regionNames := []string{}
	for _, node := range nodes.Items {
		instance, ok := node.Annotations[topology.KeyNodeInstance]
		if !ok {
			klog.Warningf("missing %q annotation in node %s", topology.KeyNodeInstance, node.Name)
			continue
		}
		region, ok := node.Annotations[topology.KeyNodeRegion]
		if !ok {
			klog.Warningf("missing %q annotation in node %s", topology.KeyNodeRegion, node.Name)
			continue
		}
		if _, ok = regions[region]; !ok {
			regions[region] = make(map[string]string)
			regionNames = append(regionNames, region)
		}
		regions[region][instance] = node.Name
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for _, region := range regionNames {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: regions[region]})
	}

	return cis
}

func (eng *K8sEngine) reconcileNodeLabelPlans(ctx context.Context, plans nodeLabelPlans) error {
	if len(plans) == 0 {
		return nil
	}

	nodes, err := eng.listNodesForReconciliation(ctx)
	if err != nil {
		return err
	}

	for _, nodeName := range slices.Sorted(maps.Keys(plans)) {
		node, ok := nodes[nodeName]
		if !ok {
			klog.Warningf("skipping topology labels for node %q because it was not returned by the Node List", nodeName)
			continue
		}
		if err := eng.reconcileNodeLabels(ctx, node, plans[nodeName]); err != nil {
			return fmt.Errorf("failed to reconcile topology labels on node %q: %w", nodeName, err)
		}
	}

	return nil
}

func (eng *K8sEngine) listNodesForReconciliation(ctx context.Context) (map[string]*corev1.Node, error) {
	baseOptions := metav1.ListOptions{}
	if eng.params != nil && eng.params.nodeListOpt != nil {
		baseOptions = *eng.params.nodeListOpt.DeepCopy()
	}
	baseOptions.Continue = ""

	nodes := make(map[string]*corev1.Node)
	continueToken := ""
	for {
		options := baseOptions
		options.Continue = continueToken
		page, err := eng.client.CoreV1().Nodes().List(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes for topology label reconciliation: %w", err)
		}
		for i := range page.Items {
			node := page.Items[i].DeepCopy()
			nodes[node.Name] = node
		}

		if page.Continue == "" {
			return nodes, nil
		}
		if page.Continue == continueToken {
			return nil, fmt.Errorf("node List returned repeated continue token %q", page.Continue)
		}
		continueToken = page.Continue
	}
}

func (eng *K8sEngine) reconcileNodeLabels(ctx context.Context, node *corev1.Node, plan *nodeLabelPlan) error {
	updated, changed := nodeWithReconciledLabels(node, plan)
	if !changed {
		return nil
	}

	klog.Infof("Reconciling managed topology labels on node %s", node.Name)
	if _, err := eng.client.CoreV1().Nodes().Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Warningf("skipping topology labels for node %q because it was deleted after the Node List", node.Name)
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}

		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest, getErr := eng.client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
			if getErr != nil {
				if apierrors.IsNotFound(getErr) {
					klog.Warningf("skipping topology labels for node %q because it was deleted during conflict retry", node.Name)
					return nil
				}
				return getErr
			}

			updated, changed := nodeWithReconciledLabels(latest, plan)
			if !changed {
				return nil
			}
			_, updateErr := eng.client.CoreV1().Nodes().Update(ctx, updated, metav1.UpdateOptions{})
			if apierrors.IsNotFound(updateErr) {
				klog.Warningf("skipping topology labels for node %q because it was deleted during conflict retry", node.Name)
				return nil
			}
			return updateErr
		})
	}

	return nil
}

func nodeWithReconciledLabels(node *corev1.Node, plan *nodeLabelPlan) (*corev1.Node, bool) {
	effectivePlan := effectiveNodeLabelPlan(node, plan)
	updated := node.DeepCopy()
	changed := false

	for _, key := range slices.Sorted(maps.Keys(effectivePlan.ManagedKeys)) {
		desired, desiredExists := effectivePlan.Desired[key]
		current, currentExists := node.Labels[key]
		switch {
		case desiredExists && (!currentExists || current != desired):
			if updated.Labels == nil {
				updated.Labels = make(map[string]string)
			}
			updated.Labels[key] = desired
			changed = true
		case !desiredExists && currentExists:
			delete(updated.Labels, key)
			changed = true
		}
	}

	return updated, changed
}

func effectiveNodeLabelPlan(node *corev1.Node, plan *nodeLabelPlan) nodeLabelPlan {
	effective := nodeLabelPlan{
		Desired:        maps.Clone(plan.Desired),
		ManagedKeys:    maps.Clone(plan.ManagedKeys),
		acceleratorKey: plan.acceleratorKey,
	}
	if strings.TrimSpace(node.Labels[topology.KeyNvidiaGPUClique]) == "" {
		return effective
	}

	// nvidia.com/gpu.clique belongs to GPU Operator. Protect an existing,
	// non-empty value even when a custom tier key collides with it.
	delete(effective.Desired, topology.KeyNvidiaGPUClique)
	delete(effective.ManagedKeys, topology.KeyNvidiaGPUClique)

	if plan.acceleratorKey == "" || plan.acceleratorKey == topology.KeyNvidiaGPUClique {
		return effective
	}
	if _, managed := plan.ManagedKeys[plan.acceleratorKey]; !managed {
		return effective
	}

	// A distinct Topograph accelerator key remains managed, but should be
	// absent while GPU Operator provides the authoritative clique value.
	delete(effective.Desired, plan.acceleratorKey)
	return effective
}
