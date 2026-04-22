/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dsx

import (
	"context"
	"fmt"
	"net/http"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "dsx-sim"

	errNone = iota
	errClientFactory
	errAPIError
)

type simClient struct {
	model  *models.Model
	apiErr int
}

func (client *simClient) GetTopology(ctx context.Context, _ string, nodeIDs []string, pageSize int, pageToken string) (*TopologyResponse, error) {
	if client.apiErr == errAPIError {
		return nil, providers.ErrAPIError
	}

	// For simulation, generate topology from model
	response := &TopologyResponse{
		Switches: make(map[string]SwitchInfo),
	}

	want := make(map[string]struct{})
	for _, nodeID := range nodeIDs {
		want[nodeID] = struct{}{}
	}

	// Build capacity block map from model
	capacityBlockMap := make(map[string]*models.CapacityBlock)
	for _, cb := range client.model.CapacityBlocks {
		capacityBlockMap[cb.Name] = cb
	}

	//Iterate over the switches from the model and add them to the switch map
	for _, sw := range client.model.Switches {
		swInfo := SwitchInfo{
			Switches: make([]string, 0),
			Nodes:    make([]NodeInfo, 0),
		}

		if len(sw.CapacityBlocks) > 0 {
			//If it is a leaf switch, add the nodes to the switch info
			for _, cbName := range sw.CapacityBlocks {
				if cb, exists := capacityBlockMap[cbName]; exists {
					nodes := cluset.Expand(cb.Nodes)
					for _, nodeName := range nodes {
						if _, exists := want[nodeName]; !exists {
							continue
						}
						swInfo.Nodes = append(swInfo.Nodes, NodeInfo{NodeID: nodeName, AcceleratedNetworkID: cb.NVLink})
					}
				}
			}
		} else {
			//If it is not a leaf switch, add the child switches to the switch info
			swInfo.Switches = append(swInfo.Switches, sw.Switches...)
		}
		response.Switches[sw.Name] = swInfo
	}

	//Return the response
	return response, nil
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(ctx context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	model, err := models.NewModelFromFile(p.ModelFileName)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("failed to load model file: %v", err))
	}

	sim := &simClient{
		model:  model,
		apiErr: p.APIError,
	}

	clientFactory := func() (Client, error) {
		if p.APIError == errClientFactory {
			return nil, providers.ErrAPIError
		}
		return sim, nil
	}

	return NewSim(clientFactory, p.TrimTiers), nil
}

type simProvider struct {
	baseProvider
}

func NewSim(clientFactory ClientFactory, trimTiers int) *simProvider {
	return &simProvider{
		baseProvider: baseProvider{
			clientFactory: clientFactory,
			trimTiers:     trimTiers,
		},
	}
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get client: %v", err))
	}
	return client.(*simClient).model.Instances, nil
}
