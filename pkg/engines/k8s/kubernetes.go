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
	"errors"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
)

var ErrEnvironmentUnsupported = errors.New("environment must implement k8sNodeInfo")

func (eng *K8sEngine) GetComputeInstances(ctx context.Context, _ engines.Environment) ([]topology.ComputeInstances, error) {
	nodes, err := eng.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list node in the cluster: %v", err)
	}
	return getComputeInstances(nodes)
}

func getComputeInstances(nodes *corev1.NodeList) ([]topology.ComputeInstances, error) {
	regions := make(map[string]map[string]string)
	for _, node := range nodes.Items {
		instance, ok := node.Annotations[topology.KeyNodeInstance]
		if !ok {
			return nil, fmt.Errorf("missing %q annotation in node %s", topology.KeyNodeInstance, node.Name)
		}
		region, ok := node.Annotations[topology.KeyNodeRegion]
		if !ok {
			return nil, fmt.Errorf("missing %q annotation in node %s", topology.KeyNodeRegion, node.Name)
		}
		if _, ok = regions[region]; !ok {
			regions[region] = make(map[string]string)
		}
		regions[region][instance] = node.Name
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for region, nodes := range regions {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: nodes})
	}

	return cis, nil
}

/*
	func (eng *K8sEngine) getNodeDataBrokerPods(ctx context.Context) (*corev1.PodList, error) {
		dataBrokerName := os.Getenv("NODE_DATA_BROKER_NAME")
		dataBrokerNamespace := os.Getenv("NODE_DATA_BROKER_NAMESPACE")

		ds, err := eng.client.AppsV1().DaemonSets(dataBrokerNamespace).Get(ctx, dataBrokerName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		selector := labels.Set(ds.Spec.Selector.MatchLabels).String()
		return eng.client.CoreV1().Pods(dataBrokerNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
	}

	func (eng *K8sEngine) execPod(ctx context.Context, pod *corev1.Pod, cmd []string) (string, error) {
		execOpts := &corev1.PodExecOptions{
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
*/
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

func MergeNodeAnnotations(node *corev1.Node, annotations map[string]string) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	maps.Copy(node.Annotations, annotations)
}
