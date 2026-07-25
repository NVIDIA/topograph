/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nfd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	internalk8s "github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/engines"
	k8sengine "github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME            = "nfd"
	envNFDNamespace = "NFD_NAMESPACE"
)

type NfdEngine struct {
	client        kubernetes.Interface
	dynamicClient dynamic.Interface
	params        *Params
	namespace     string
	cachedNodes   *corev1.NodeList
}

type Params struct {
	// NodeSelector (optional) specifies nodes participating in the topology.
	NodeSelector map[string]string `mapstructure:"nodeSelector"`
	// Cleanup deletes stale Topograph-managed NFD objects. Defaults to true.
	Cleanup bool `mapstructure:"cleanup"`

	// derived fields
	nodeListOpt *metav1.ListOptions
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(_ context.Context, params engines.Config) (engines.Engine, *httperr.Error) {
	p, err := getParameters(params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	namespace, err := getNFDNamespace()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return &NfdEngine{
		client:        client,
		dynamicClient: dynamicClient,
		params:        p,
		namespace:     namespace,
	}, nil
}

func getParameters(params engines.Config) (*Params, error) {
	p := &Params{Cleanup: true}
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

func getNFDNamespace() (string, error) {
	if namespace := strings.TrimSpace(os.Getenv(envNFDNamespace)); namespace != "" {
		return namespace, nil
	}
	return "", fmt.Errorf("%s environment variable is required", envNFDNamespace)
}

func (eng *NfdEngine) GenerateOutput(ctx context.Context, graph *topology.Graph, _ map[string]any) ([]byte, *httperr.Error) {
	nodes := eng.cachedNodes
	if nodes == nil {
		var err error
		nodes, err = internalk8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadGateway, err.Error())
		}
	}

	nodeLabels, err := k8sengine.NewTopologyLabeler(
		k8sengine.NewTopologyLabelKeys(nil, ""),
	).BuildNodeLabels(graph)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	nodeFeatures, nodeFeatureGroups, err := buildNFDObjects(nodeLabels, gpuCliqueValues(nodes))
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	if eng.params.Cleanup && len(nodeFeatures) == 0 && len(nodeFeatureGroups) == 0 {
		return nil, httperr.NewError(http.StatusBadGateway,
			"generated no NFD topology objects; keeping the existing topology")
	}

	if err := eng.applyObjects(ctx, nodeFeatures, nodeFeatureGroups); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	if eng.params.Cleanup {
		if err := eng.cleanupObjects(ctx, nodeFeatures, nodeFeatureGroups); err != nil {
			return nil, httperr.NewError(http.StatusBadGateway, err.Error())
		}
	}

	return fmt.Appendf(nil, "OK nodeFeatures=%d nodeFeatureGroups=%d\n", len(nodeFeatures), len(nodeFeatureGroups)), nil
}

func gpuCliqueValues(nodes *corev1.NodeList) map[string]string {
	out := make(map[string]string)
	if nodes == nil {
		return out
	}

	for _, node := range nodes.Items {
		if value := strings.TrimSpace(node.Labels[topology.KeyNvidiaGPUClique]); value != "" {
			out[node.Name] = value
		}
	}

	return out
}
