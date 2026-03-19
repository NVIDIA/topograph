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
			ID:          node.Name,
			NetworkPath: path,
		}
		if len(node.Attributes.NVLink) != 0 {
			instance.BlockID = &node.Attributes.NVLink
		}

		resp = append(resp, instance)
	}

	return resp, nil
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
		instanceIDs = append(instanceIDs, node.Name)
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
	}, nil
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	return p.client.(*simClient).model.Instances, nil
}
