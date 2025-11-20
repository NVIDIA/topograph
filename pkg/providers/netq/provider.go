/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "netq"

type Provider struct {
	params *ProviderParams
	cred   *Credentials
}

type ProviderParams struct {
	ApiURL string `mapstructure:"apiUrl"`
}

type Credentials struct {
	user   string
	passwd string
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	params, err := getParams(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	cred, err := getCred(config.Creds)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	return &Provider{
		params: params,
		cred:   cred,
	}, nil
}

func getCred(cred map[string]string) (*Credentials, error) {
	user, ok := cred["username"]
	if !ok {
		return nil, fmt.Errorf("username not provided")
	}

	passwd, ok := cred["password"]
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	return &Credentials{user: user, passwd: passwd}, nil
}

func getParams(params map[string]any) (*ProviderParams, error) {
	p := &ProviderParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %w", err)
	}
	if len(p.ApiURL) == 0 {
		return nil, fmt.Errorf("apiUrl not provided")
	}

	return p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	treeRoot, err := p.getNetworkTree(ctx, instances)
	if err != nil {
		return nil, err
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{
			topology.TopologyTree: treeRoot,
		},
	}

	if domains, err := p.getNvlDomains(ctx); err != nil {
		klog.Warningf("Failed to get NVL domains: %v", err)
	} else {
		root.Vertices[topology.TopologyBlock] = domains.ToBlocks()
	}

	return root, nil
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetInstancesRegions implements slurm.instanceMapper
func (p *Provider) GetInstancesRegions(ctx context.Context, nodes []string) (map[string]string, error) {
	res := make(map[string]string)
	for _, node := range nodes {
		res[node] = "local"
	}
	return res, nil
}
