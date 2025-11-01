/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"context"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/httperr"
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

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return &Provider{
		config: cfg,
		client: client,
	}, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	regIndices := make(map[string]int) // map[region : index]
	for i, ci := range instances {
		regIndices[ci.Region] = i
	}

	nodes, err := k8s.GetNodes(ctx, p.client)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	domainMap := topology.NewDomainMap()
	for _, node := range nodes.Items {
		clusterID, ok := node.Labels["nvidia.com/gpu.clique"]
		if !ok {
			continue
		}

		region := node.Annotations[topology.KeyNodeRegion]
		indx, ok := regIndices[region]
		if !ok {
			continue
		}

		i2n := instances[indx].Instances
		if host, ok := i2n[node.Name]; ok {
			domainMap.AddHost(clusterID, node.Name, host)
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
