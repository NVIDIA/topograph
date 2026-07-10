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
	"maps"
	"net/http"
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

	MergeNodeLabels(node, labels)

	_, err = eng.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})

	return err
}

func MergeNodeLabels(node *corev1.Node, labels map[string]string) {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	labels = skipAcceleratorLabelWhenGPUCliqueExists(node, labels)
	maps.Copy(node.Labels, labels)
}

func skipAcceleratorLabelWhenGPUCliqueExists(node *corev1.Node, labels map[string]string) map[string]string {
	if topologyLabelKeys.Accelerator == "" || strings.TrimSpace(node.Labels[topology.KeyNvidiaGPUClique]) == "" {
		return labels
	}

	filtered := maps.Clone(labels)
	delete(filtered, topologyLabelKeys.Accelerator)

	if topologyLabelKeys.Accelerator != topology.KeyNvidiaGPUClique {
		delete(node.Labels, topologyLabelKeys.Accelerator)
	}

	return filtered
}
