/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"fmt"
	"net/http"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "nscale-sim"

	errNone = iota
	errTopology
)

type simClient struct {
	model       *models.Model
	instanceIDs []string
	apiErr      int
}

type simProvider struct {
	baseProvider
	*providers.BaseSimProvider
}

func (c *simClient) Topology(ctx context.Context, _ string, pageSize, offset int) ([]InstanceTopology, error) {
	if c.apiErr == errTopology {
		return nil, providers.ErrAPIError
	}

	resp := make([]InstanceTopology, 0, pageSize)

	var indx int
	for indx = offset; indx < offset+pageSize && indx < len(c.instanceIDs); indx++ {
		node, exists := c.model.Nodes[c.instanceIDs[indx]]
		if !exists {
			continue
		}
		nl := node.NetLayers
		path := make([]string, len(nl))
		for i, v := range nl {
			path[len(nl)-1-i] = v
		}
		instance := InstanceTopology{
			ID:          node.ID,
			NetworkPath: path,
		}
		domainID := node.Labels[topology.KeyTopologyAccelerator]
		if domainID != "" {
			instance.BlockID = &domainID
		}

		resp = append(resp, instance)
	}

	return resp, nil
}

func (c *simClient) Instances(_ context.Context, _ string) (map[string]string, error) {
	i2n := make(map[string]string, len(c.model.Nodes))
	for _, node := range c.model.Nodes {
		i2n[node.ID] = node.ID
	}

	return i2n, nil
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(_ context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	model, err := models.NewModelFromFile(p.ModelFileName)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("failed to load model file: %v", err))
	}

	instanceIDs := make([]string, 0, len(model.Nodes))
	for _, node := range model.Nodes {
		instanceIDs = append(instanceIDs, node.ID)
	}

	return &simProvider{
		baseProvider: baseProvider{
			client: &simClient{
				model:       model,
				instanceIDs: instanceIDs,
				apiErr:      p.APIError,
			},
			params: &ProviderParams{},
		},
		BaseSimProvider: providers.NewBaseSimProvider(model, p.TrimTiers),
	}, nil
}

// Engine support

func (p *simProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}
	return p.ToGraph(NAME_SIM, topo, instances, false), nil
}
