/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

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

const NAME_K8S = "infiniband-k8s"

type ProviderK8S struct {
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

func NamedLoaderK8S() (string, providers.Loader) {
	return NAME_K8S, LoaderK8S
}

func LoaderK8S(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
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

	return &ProviderK8S{
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

func (p *ProviderK8S) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	if len(cis) > 1 {
		return nil, httperr.NewError(http.StatusBadRequest, "on-prem does not support multi-region topology requests")
	}

	nodes, err := k8s.GetNodes(ctx, p.client, p.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
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
		return nil, httperr.NewError(http.StatusInternalServerError, fmt.Sprintf("getIbTree failed: %v", err))
	}

	return toGraph(domainMap, treeRoot), nil
}
