/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"context"
	"errors"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "dra"

type Provider struct {
	config *rest.Config
	client *kubernetes.Clientset
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Provider{
		config: cfg,
		client: client,
	}, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, errors.New("DRA provider does not support multi-region topology requests")
	}

	nodes, err := k8s.GetNodes(ctx, p.client)
	if err != nil {
		return nil, err
	}

	domainMap := topology.NewDomainMap()
	for _, node := range nodes.Items {
		if clusterID, ok := node.Labels["nvidia.com/gpu.clique"]; ok {
			domainMap.AddHost(clusterID, node.Name, node.Name)
		}
	}

	return toGraph(domainMap), nil
}

func toGraph(domainMap topology.DomainMap) *topology.Vertex {
	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}
	root.Vertices[topology.TopologyBlock] = domainMap.ToBlocks()

	return root
}

func GetNodeAnnotations(ctx context.Context, hostname string) (map[string]string, error) {
	annotations := map[string]string{
		topology.KeyNodeInstance: hostname,
		topology.KeyNodeRegion:   "local",
	}

	return annotations, nil
}
