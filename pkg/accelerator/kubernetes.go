/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package accelerator

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	internalK8s "github.com/NVIDIA/topograph/internal/k8s"
)

// NewKubernetesNodeDiscoverer returns the node-local discoverer used by the
// node-data-broker. Sources that read existing Kubernetes metadata require no
// node-local collection and therefore return an empty discoverer.
func NewKubernetesNodeDiscoverer(section Section, client kubernetes.Interface, restConfig *rest.Config) (Discoverer, error) {
	config, err := ParseConfig(section)
	if err != nil {
		return nil, err
	}

	switch config.Source {
	case SourceNvidiaSMI:
		if client == nil {
			return nil, fmt.Errorf("k8s client is required for nvidia-smi discovery")
		}
		if restConfig == nil {
			return nil, fmt.Errorf("k8s REST config is required for nvidia-smi discovery")
		}
		return NewNvidiaSMIDiscoverer(config, &kubernetesNvidiaSMIRunner{
			client:    client,
			config:    restConfig,
			namespace: config.NvidiaSMI.GPUOperatorNamespace,
			daemonSet: config.NvidiaSMI.DevicePluginDaemonSet,
		})
	case SourceKubernetesLabel, SourceNone:
		return NewNoneDiscoverer(), nil
	default:
		return nil, fmt.Errorf("unsupported accelerator source %q", config.Source)
	}
}

type kubernetesNvidiaSMIRunner struct {
	client    kubernetes.Interface
	config    *rest.Config
	namespace string
	daemonSet string
}

func (r *kubernetesNvidiaSMIRunner) Run(ctx context.Context, command string, targets []Target) (map[string]string, error) {
	outputs := make(map[string]string)
	for _, target := range targets {
		pods, err := internalK8s.GetDaemonSetPods(ctx, r.client, r.daemonSet, r.namespace, target.HostName)
		if err != nil {
			return nil, err
		}

		switch len(pods.Items) {
		case 0:
			klog.Infof("no %s on %s node", r.daemonSet, target.HostName)
		case 1:
			output, err := internalK8s.ExecInPod(
				ctx,
				r.client,
				r.config,
				pods.Items[0].Name,
				r.namespace,
				strings.Fields(command),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to query NVL partition ID: %w", err)
			}
			outputs[target.HostName] = output.String()
		default:
			return nil, fmt.Errorf("expected 1 %s pod, got %d", r.daemonSet, len(pods.Items))
		}
	}

	return outputs, nil
}
