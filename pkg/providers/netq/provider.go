/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"context"
	"fmt"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "netq"

type ProviderParams struct {
	NetqLoginUrl string `mapstructure:"netqLoginUrl"`
	NetqApiUrl   string `mapstructure:"netqApiUrl"`
}

type Provider struct {
	params *ProviderParams
	cred   *Credentials
}

type Credentials struct {
	user   string
	passwd string
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	params, err := GetParams(config.Params)
	if err != nil {
		return nil, err
	}

	cred, err := GetCred(config.Creds)
	if err != nil {
		return nil, err
	}

	return &Provider{
		params: params,
		cred:   cred,
	}, nil
}

func GetCred(cred map[string]string) (*Credentials, error) {
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

func GetParams(params map[string]any) (*ProviderParams, error) {
	p := &ProviderParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %w", err)
	}
	if len(p.NetqLoginUrl) == 0 {
		return nil, fmt.Errorf("netqLoginUrl not provided")
	}
	if len(p.NetqApiUrl) == 0 {
		return nil, fmt.Errorf("netqApiUrl not provided")
	}

	return p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	return p.generateTopologyConfig(ctx, instances)
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion(_ context.Context) (string, error) {
	return "local", nil
}
