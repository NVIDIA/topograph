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
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
)

var ErrEnvironmentUnsupported = errors.New("environment must implement k8sNodeInfo")

func (eng *K8sEngine) GetComputeInstances(ctx context.Context, environment engines.Environment) ([]topology.ComputeInstances, error) {
	k8sNodeInfo, ok := environment.(k8sNodeInfo)
	if !ok {
		return nil, ErrEnvironmentUnsupported
	}

	// TODO: replace with variables
	daemonSetName := "node-data-broker"
	namespace := "default"
	ds, err := eng.client.AppsV1().DaemonSets(namespace).Get(ctx, daemonSetName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonset %s: %v", daemonSetName, err)
	}

	selector := labels.Set(ds.Spec.Selector.MatchLabels).String()
	pods, err := eng.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pods for daemonset %s: %v", daemonSetName, err)
	}

	return getComputeInstances(ctx, k8sNodeInfo, daemonSetName, pods, eng.execPod), nil
}

func getComputeInstances(ctx context.Context, nodeInfo k8sNodeInfo, daemonSetName string, pods *v1.PodList, execPod func(context.Context, *v1.Pod, []string) (string, error)) []topology.ComputeInstances {
	ch := make(chan any)
	var wg sync.WaitGroup

	for i := range pods.Items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pod := &pods.Items[i]
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "DaemonSet" && owner.Name == daemonSetName {
					nodeName := pod.Spec.NodeName
					region, err := execPod(ctx, pod, nodeInfo.NodeRegionCommand())
					if err != nil {
						ch <- fmt.Errorf("failed to get region for node %s: %v", nodeName, err)
						return
					}
					region = nodeInfo.ProcessNodeRegionOutput(region)

					instance, err := execPod(ctx, pod, nodeInfo.NodeInstanceCommand())
					if err != nil {
						ch <- fmt.Errorf("failed to get instance ID for node %s: %v", nodeName, err)
						return
					}
					instance = nodeInfo.ProcessNodeInstanceOutput(instance)
					ch <- [3]string{nodeName, instance, region}
					return
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	regions := make(map[string]map[string]string)
	for obj := range ch {
		switch v := obj.(type) {
		case error:
			klog.Error(v.Error())
		case [3]string:
			nodeName, instance, region := v[0], v[1], v[2]
			_, ok := regions[region]
			if !ok {
				regions[region] = make(map[string]string)
			}
			klog.V(4).InfoS("Updating cluster inventory", "node", nodeName, "instance", instance, "region", region)
			regions[region][instance] = nodeName
		}
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for region, nodes := range regions {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: nodes})
	}

	return cis
}

func (eng *K8sEngine) execPod(ctx context.Context, pod *v1.Pod, cmd []string) (string, error) {
	execOpts := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	req := eng.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(execOpts, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(eng.config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to execute command %v in pod %s: %v", cmd, pod.Name, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", fmt.Errorf("failed to execute command %v in pod %s: %s: %v", cmd, pod.Name, stderr.String(), err)
	}

	return stdout.String(), nil
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
	res, err := eng.client.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if err == nil {
		verb = "update"
		res, err = eng.client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	} else if apierrors.IsNotFound(err) {
		verb = "create"
		res, err = eng.client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
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
	node, err := eng.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	for k, v := range labels {
		node.Labels[k] = v
	}

	_, err = eng.client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})

	return err
}
