/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package baremetal

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_IB = "baremetal.ib"

type ProviderIB struct {
	config *rest.Config
	client *kubernetes.Clientset
}

func NamedLoaderIB() (string, providers.Loader) {
	return NAME_IB, Loader
}

func LoaderIB(ctx context.Context, config providers.Config) (providers.Provider, error) {
	return NewIB()
}

func NewIB() (*ProviderIB, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &ProviderIB{
		config: config,
		client: client,
	}, nil
}

func (p *ProviderIB) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, ErrMultiRegionNotSupported
	}

	nodes, err := k8s.GetNodes(ctx, p.client)
	if err != nil {
		return nil, err
	}

	domainMap := topology.NewDomainMap()
	for _, node := range nodes.Items {
		if clusterID, ok := node.Annotations[topology.KeyNodeClusterID]; ok {
			domainMap.AddHost(clusterID, node.Name, node.Name)
		}
	}

	return toGraph(domainMap, nil), nil
}
