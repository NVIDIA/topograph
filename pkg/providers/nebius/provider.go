/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"
	"os"

	"github.com/nebius/gosdk"
	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	services "github.com/nebius/gosdk/services/nebius/compute/v1"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "nebius"

type Client interface {
	GetComputeInstance(context.Context, *compute.GetInstanceRequest) (*compute.Instance, error)
}

type ClientFactory func() (Client, error)

type baseProvider struct {
	clientFactory ClientFactory
}

type nebiusClient struct {
	instanceService services.InstanceService
}

func (c *nebiusClient) GetComputeInstance(ctx context.Context, req *compute.GetInstanceRequest) (*compute.Instance, error) {
	return c.instanceService.Get(ctx, req)
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	sdk, err := getSDK(ctx, config.Creds)
	if err != nil {
		return nil, err
	}

	instanceService := sdk.Services().Compute().V1().Instance()

	clientFactory := func() (Client, error) {
		return &nebiusClient{
			instanceService: instanceService,
		}, nil
	}

	return New(clientFactory), nil
}

func getSDK(ctx context.Context, creds map[string]string) (*gosdk.SDK, error) {
	// TODO: use credentials from payload or config to properly authenticate SDK.
	sdk, err := gosdk.New(
		ctx,
		gosdk.WithCredentials(
			gosdk.IAMToken(os.Getenv("IAM_TOKEN")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gosdk: %v", err)
	}

	return sdk, nil
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	topo, err := p.generateInstanceTopology(ctx, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, false)
}

type Provider struct {
	baseProvider
}

func New(clientFactory ClientFactory) *Provider {
	return &Provider{
		baseProvider: baseProvider{clientFactory: clientFactory},
	}
}

// Engine support

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	// TODO: implement function that returns map[instance id : hostname] for SLURM cluster given array of hostnames
	return nil, fmt.Errorf("not implemented")
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion(ctx context.Context) (string, error) {
	// TODO: implement function that returns region name for the current host in SLURM cluster
	return "", fmt.Errorf("not implemented")
}
