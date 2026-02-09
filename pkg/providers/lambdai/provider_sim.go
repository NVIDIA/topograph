/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lambdai

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "lambdai-sim"

	errNone = iota
	errClientFactory
	errInstanceList
)

type simClient struct {
	model       *models.Model
	pageSize    int
	instanceIDs []string
	apiErr      int
}

func (c *simClient) WorkspaceID() string {
	return "simulation"
}

func (c *simClient) PageSize() int {
	return c.pageSize
}

func (c *simClient) InstanceList(ctx context.Context, req *InstanceListRequest) (*InstanceListResponse, error) {
	if c.apiErr == errInstanceList {
		return &InstanceListResponse{}, providers.ErrAPIError
	}

	resp := InstanceListResponse{
		Items: make([]InstanceTopology, 0, len(c.model.Nodes)),
	}

	var indx int
	from := getPage(req.PageToken)
	for indx = from; indx < from+c.pageSize && indx < len(c.instanceIDs); indx++ {
		node, exists := c.model.Nodes[c.instanceIDs[indx]]
		if !exists {
			continue
		}
		instance := InstanceTopology{
			ID:          node.Name,
			NetworkPath: node.NetLayers,
			//TODO: check whether the below mapping is correct
			NVLink: &NVLinkInfo{
				DomainID: node.NVLink,
				CliqueID: "simulation",
			},
		}

		resp.Items = append(resp.Items, instance)
	}

	if indx < len(c.instanceIDs) {
		resp.NextPageToken = fmt.Sprintf("%d", indx)
	}

	return &resp, nil
}

func getPage(page string) int {
	if len(page) == 0 {
		return 0
	}

	val, _ := strconv.ParseInt(page, 10, 32)
	return int(val)
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

	clientFactory := func(pageSize *int) (Client, error) {
		if p.APIError == errClientFactory {
			return nil, providers.ErrAPIError
		}

		pSize := len(instanceIDs)
		if pageSize != nil {
			pSize = *pageSize
		}

		return &simClient{
			model:       model,
			pageSize:    pSize,
			instanceIDs: instanceIDs,
			apiErr:      p.APIError,
		}, nil
	}

	return NewSim(clientFactory), nil
}

type simProvider struct {
	baseProvider
}

func NewSim(clientFactory ClientFactory) *simProvider {
	return &simProvider{
		baseProvider: baseProvider{clientFactory: clientFactory},
	}
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	client, err := p.clientFactory(nil)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, "failed to create client")
	}

	return client.(*simClient).model.Instances, nil
}
