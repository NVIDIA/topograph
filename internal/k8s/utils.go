/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package k8s

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func GetNodes(ctx context.Context, client *kubernetes.Clientset) (*corev1.NodeList, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list node in the cluster: %v", err)
	}

	return nodes, nil
}

func GetPodsByLabels(ctx context.Context, client *kubernetes.Clientset, namespace string, l map[string]string) (*corev1.PodList, error) {
	opt := metav1.ListOptions{LabelSelector: labels.SelectorFromSet(l).String()}
	return client.CoreV1().Pods(namespace).List(ctx, opt)
}

func IsPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func GetDaemonSetPods(ctx context.Context, client *kubernetes.Clientset, name, namespace, nodename string) (*corev1.PodList, error) {
	ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	opt := metav1.ListOptions{
		LabelSelector: labels.Set(ds.Spec.Selector.MatchLabels).String(),
	}
	if len(nodename) != 0 {
		opt.FieldSelector = "spec.nodeName=" + nodename
	}

	return client.CoreV1().Pods(namespace).List(ctx, opt)
}

func ExecInPod(ctx context.Context, client *kubernetes.Clientset, config *rest.Config, name, namespace string, cmd []string) (*bytes.Buffer, error) {
	execOpts := &corev1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec").
		VersionedParams(execOpts, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to execute command %v in pod %s/%s: %v", cmd, namespace, name, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to execute command %v in pod %s/%s: %s: %v", cmd, namespace, name, stderr.String(), err)
	}

	return &stdout, nil
}
