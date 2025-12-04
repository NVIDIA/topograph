/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dra

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "dra"

	DomainLabel = "nvidia.com/gpu.clique"
)

type Provider struct {
	config *rest.Config
	client *kubernetes.Clientset
	params *Params
}

type Params struct {
	// NodeSelector (optional) specifies nodes participating in the topology
	NodeSelector map[string]string `mapstructure:"nodeSelector"`

	// derived fields
	nodeListOpt *metav1.ListOptions
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	p, err := getParameters(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

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
		params: p,
	}, nil
}

func getParameters(params map[string]any) (*Params, error) {
	p := &Params{}
	if err := config.Decode(params, p); err != nil {
		return nil, err
	}

	if len(p.NodeSelector) != 0 {
		p.nodeListOpt = &metav1.ListOptions{
			LabelSelector: labels.Set(p.NodeSelector).String(),
		}
	}

	return p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	regIndices := make(map[string]int) // map[region : index]
	for i, ci := range instances {
		regIndices[ci.Region] = i
	}

	nodes, err := k8s.GetNodes(ctx, p.client, p.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	domainMap := topology.NewDomainMap()
	for _, node := range nodes.Items {
		clusterID, ok := node.Labels[DomainLabel]
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

	if len(domainMap) == 0 {
		return nil, httperr.NewError(http.StatusBadGateway,
			fmt.Sprintf("no matching nodes found; check label %q and annotations %q and %q",
				DomainLabel, topology.KeyNodeRegion, topology.KeyNodeInstance))
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
