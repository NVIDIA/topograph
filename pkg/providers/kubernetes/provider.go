/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgo "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/dsx-ai-factory/topograph/internal/config"
	"github.com/dsx-ai-factory/topograph/internal/httperr"
	"github.com/dsx-ai-factory/topograph/internal/k8s"
	"github.com/dsx-ai-factory/topograph/pkg/accelerator"
	"github.com/dsx-ai-factory/topograph/pkg/providers"
	"github.com/dsx-ai-factory/topograph/pkg/topology"
)

const NAME = "kubernetes"

type Params struct {
	TopologyLabels []string          `mapstructure:"topologyLabels"`
	NodeSelector   map[string]string `mapstructure:"nodeSelector"`
}

// ParseParams validates the shared provider and observer configuration.
func ParseParams(values map[string]any) (*Params, error) {
	switch v := values["topologyLabels"].(type) {
	case []string, nil:
	case []any:
		for _, key := range v {
			if _, ok := key.(string); !ok {
				return nil, fmt.Errorf("topologyLabels must be a list of strings")
			}
		}
	default:
		return nil, fmt.Errorf("topologyLabels must be a list of strings")
	}
	p := &Params{}
	if err := config.Decode(values, p); err != nil {
		return nil, err
	}
	if len(p.TopologyLabels) == 0 {
		return nil, fmt.Errorf("topologyLabels must contain at least one label key, closest tier first")
	}
	seen := make(map[string]bool)
	for _, key := range p.TopologyLabels {
		if err := k8s.ValidateLabelKey("topologyLabels", key); err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate topologyLabels key %q", key)
		}
		seen[key] = true
	}
	for key, value := range p.NodeSelector {
		if err := k8s.ValidateLabelKey("nodeSelector", key); err != nil {
			return nil, err
		}
		if errs := validation.IsValidLabelValue(value); len(errs) != 0 {
			return nil, fmt.Errorf("invalid nodeSelector value for %q: %s", key, strings.Join(errs, "; "))
		}
	}
	return p, nil
}

type Provider struct {
	client clientgo.Interface
	params *Params
}

func NamedLoader() (string, providers.Loader) { return NAME, Loader }

func Loader(_ context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	params, err := ParseParams(cfg.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	if err := k8s.ConfigureClientRateLimits(clientConfig); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	client, err := clientgo.NewForConfig(clientConfig)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	return &Provider{client: client, params: params}, nil
}

// New constructs a provider using an existing Kubernetes client.
func New(client clientgo.Interface, values map[string]any) (*Provider, error) {
	params, err := ParseParams(values)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, params: params}, nil
}

// IsNodeEligible excludes nodes that cannot participate in the current topology.
// Cordoning alone does not remove existing workloads.
func IsNodeEligible(node *corev1.Node) bool {
	if node.DeletionTimestamp != nil {
		return false
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	nodes, err := k8s.GetNodes(ctx, p.client, &metav1.ListOptions{LabelSelector: labels.Set(p.params.NodeSelector).String()})
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	requested := make(map[string]string)
	for _, ci := range cis {
		for id, name := range ci.Instances {
			requested[id] = name
		}
	}
	eligible := make(map[string]string)
	topo := topology.NewClusterTopology()
	for _, node := range nodes.Items {
		id := strings.TrimSpace(node.Annotations[topology.KeyNodeInstance])
		if id == "" {
			id = node.Name
		}
		name, wanted := requested[id]
		if !wanted || !IsNodeEligible(&node) {
			continue
		}
		values := make([]string, len(p.params.TopologyLabels))
		for i, key := range p.params.TopologyLabels {
			value := node.Labels[key]
			if value == "" || len(validation.IsValidLabelValue(value)) != 0 {
				return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("node %q has missing or invalid topology label %q: %q", node.Name, key, value))
			}
			values[i] = value
		}
		// Scope repeated switch labels to their ancestors. Label values cannot contain '/'.
		tiers := make([]topology.FabricTier, len(values))
		for i := range values {
			tiers[i].ID = strings.Join(values[i:], "/")
		}
		topo.Append(&topology.InstanceTopology{InstanceID: id, FabricTiers: tiers})
		eligible[id] = name
	}
	// Do not let ToGraph add filtered nodes back as nodes without topology.
	return topo.ToGraph(NAME, []topology.ComputeInstances{{Instances: eligible}}, 0, true), nil
}

func GetNodeAnnotations(_ context.Context, hostName string) (map[string]string, error) {
	return accelerator.BaseKubernetesNodeAnnotations(hostName), nil
}
