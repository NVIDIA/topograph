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
	std_errors "errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
)

var ErrEnvironmentUnsupported = std_errors.New("environment must implement k8sNodeInfo")

func (eng *K8sEngine) GetComputeInstances(ctx context.Context, environment engines.Environment) ([]topology.ComputeInstances, error) {
	k8sNodeInfo, ok := environment.(k8sNodeInfo)
	if !ok {
		return nil, ErrEnvironmentUnsupported
	}

	nodeList, err := eng.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to list node in the cluster: %v", err)
	}

	regions := make(map[string]map[string]string)
	for _, n := range nodeList.Items {
		region, err := k8sNodeInfo.GetNodeRegion(&n)
		if err != nil {
			return nil, err
		}
		instance, err := k8sNodeInfo.GetNodeInstance(&n)
		if err != nil {
			return nil, err
		}

		_, ok := regions[region]
		if !ok {
			regions[region] = make(map[string]string)
		}
		regions[region][instance] = n.Name
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for region, nodes := range regions {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: nodes})
	}

	return cis, nil
}

func (eng *K8sEngine) UpdateTopologyConfigmap(ctx context.Context, name, namespace string, data map[string]string) error {
	klog.Infof("Updating topology config %s/%s", namespace, name)

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	verb := "get"
	res, err := eng.kubeClient.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if err == nil {
		verb = "update"
		res, err = eng.kubeClient.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	} else if errors.IsNotFound(err) {
		verb = "create"
		res, err = eng.kubeClient.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	}

	if err != nil {
		return fmt.Errorf("failed to %s configmap %s/%s: %v",
			verb, cm.Namespace, cm.Name, err)
	}

	klog.V(4).Infof("Successfully %sd configmap %s/%s", verb, res.Namespace, res.Name)

	return nil
}

func (eng *K8sEngine) AddNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	klog.Infof("Applying labels on node %s : %v", nodeName, labels)
	node, err := eng.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	for k, v := range labels {
		node.Labels[k] = v
	}

	_, err = eng.kubeClient.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})

	return err
}
