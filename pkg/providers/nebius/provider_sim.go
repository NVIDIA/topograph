/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"

	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "nebius-sim"

	errNone = iota
	errInstances
	errTopologyPath
)

type simClient struct {
	model  *models.Model
	apiErr int
}

func (c *simClient) GetComputeInstance(ctx context.Context, req *compute.GetInstanceRequest) (*compute.Instance, error) {
	if c.apiErr == errInstances {
		return nil, fmt.Errorf("error")
	}

	instance := &compute.Instance{Status: &compute.InstanceStatus{}}

	if node, ok := c.model.Nodes[req.Id]; ok {
		var path []string
		if c.apiErr == errTopologyPath {
			path = []string{}
		} else {
			path = []string{node.NetLayers[2], node.NetLayers[1], node.NetLayers[0]}
		}
		instance.Status.GpuClusterTopology = &compute.InstanceStatus_InfinibandTopologyPath{
			InfinibandTopologyPath: &compute.InstanceStatusInfinibandTopologyPath{
				Path: path,
			},
		}
	}

	return instance, nil
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(ctx context.Context, cfg providers.Config) (providers.Provider, error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, err
	}

	model, err := models.NewModelFromFile(p.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model file: %v", err)
	}

	clientFactory := func() (Client, error) {

		return &simClient{
			model:  model,
			apiErr: p.APIError,
		}, nil
	}

	return NewSim(clientFactory), nil
}

type simProvider struct {
	baseProvider
}

func NewSim(factory ClientFactory) *simProvider {
	return &simProvider{
		baseProvider: baseProvider{clientFactory: factory},
	}
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory()

	return client.(*simClient).model.Instances, nil
}
