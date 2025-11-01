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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (eng *K8sEngine) GetComputeInstances(ctx context.Context, _ engines.Environment) ([]topology.ComputeInstances, *httperr.Error) {
	nodes, err := k8s.GetNodes(ctx, eng.client)
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
	maps.Copy(node.Labels, labels)
}
