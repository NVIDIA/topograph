/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_K8S = "infiniband-k8s"

type ProviderK8S struct {
	config *rest.Config
	client *kubernetes.Clientset
}

func NamedLoaderK8S() (string, providers.Loader) {
	return NAME_K8S, LoaderK8S
}

func LoaderK8S(ctx context.Context, _ providers.Config) (providers.Provider, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &ProviderK8S{
		config: cfg,
		client: client,
	}, nil
}

func (p *ProviderK8S) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(cis) > 1 {
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

	ibnetdiscover := NewIBNetDiscoverK8S(p.config, p.client)
	treeRoot, err := getIbTree(ctx, cis, ibnetdiscover)
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v", err)
	}

	return toGraph(domainMap, treeRoot), nil
}
