/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package baremetal

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func GetClusterID(ctx context.Context, client *kubernetes.Clientset, config *rest.Config, hostname string) (string, error) {
	// TODO: parametrize gpu-operator namespace/name
	pods, err := k8s.GetDaemonSetPods(ctx, client, "nvidia-device-plugin-daemonset", "gpu-operator", hostname)
	if err != nil {
		return "", err
	}

	var output string
	switch len(pods.Items) {
	case 0:
		klog.Infof("no nvidia-device-plugin-daemonset in %s node", hostname)
		return "", nil
	case 1:
		cmd := []string{"sh", "-c", `nvidia-smi -q | grep "ClusterUUID\|CliqueId" | sort -u`}
		output, err = k8s.ExecInPod(ctx, client, config, pods.Items[0].Name, "gpu-operator", cmd)
		if err != nil {
			return "", err
		}
		return parseClusterID(output)
	default:
		return "", fmt.Errorf("expected 1 nvidia-device-plugin-daemonset pod, got %d", len(pods.Items))
	}
}

func parseClusterID(txt string) (string, error) {
	klog.V(4).Infof("ClusterID output: %q", txt)
	var cliqueId, clusterUUID string
	scanner := bufio.NewScanner(strings.NewReader(txt))
	for scanner.Scan() {
		line := scanner.Text()
		arr := strings.Split(line, ":")
		switch strings.TrimSpace(arr[0]) {
		case "CliqueId":
			cliqueId = strings.TrimSpace(arr[1])
		case "ClusterUUID":
			clusterUUID = strings.TrimSpace(arr[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan %q: %v", txt, err)
	}

	if len(clusterUUID) == 0 {
		return "", fmt.Errorf("missing ClusterUUID")
	}

	if len(cliqueId) == 0 {
		return "", fmt.Errorf("missing CliqueId")
	}

	klog.V(4).InfoS("Cluster ID", "clusterUUID", clusterUUID, "cliqueId", cliqueId)
	return clusterUUID + "." + cliqueId, nil
}

func GetNodeAnnotations(ctx context.Context, client *kubernetes.Clientset, config *rest.Config, hostname string) (map[string]string, error) {
	annotations := map[string]string{
		topology.KeyNodeInstance: hostname,
		topology.KeyNodeRegion:   "local",
	}

	if clusterID, err := GetClusterID(ctx, client, config, hostname); err != nil {
		klog.Warningf("No clusterID for node %s: %v", hostname, err)
	} else {
		annotations[topology.KeyNodeClusterID] = clusterID
	}

	return annotations, nil
}
