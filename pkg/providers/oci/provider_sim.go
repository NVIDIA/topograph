/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package oci

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/agrea/ptr"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "oci-sim"

	errNoce = iota
	errClientFactory
	errListAvailabilityDomains
	errListComputeHosts
)

type simClient struct {
	model       *models.Model
	pageSize    *int
	instanceIDs []string
	apiErr      int
}

var httpResponce = &http.Response{
	Status:     "200 OK",
	StatusCode: 200,
}

func (c *simClient) TenantID() *string {
	return ptr.String("simulation")
}

func (c *simClient) Limit() *int { return c.pageSize }

func (c *simClient) ListAvailabilityDomains(ctx context.Context, req identity.ListAvailabilityDomainsRequest) (identity.ListAvailabilityDomainsResponse, error) {
	if c.apiErr == errListAvailabilityDomains {
		return identity.ListAvailabilityDomainsResponse{}, providers.APIError
	}

	return identity.ListAvailabilityDomainsResponse{
		RawResponse: httpResponce,
		Items: []identity.AvailabilityDomain{
			{Name: ptr.String("ad")},
		},
	}, nil
}

func (c *simClient) ListComputeHosts(ctx context.Context, req core.ListComputeHostsRequest) (core.ListComputeHostsResponse, error) {
	if c.apiErr == errListComputeHosts {
		return core.ListComputeHostsResponse{}, providers.APIError
	}

	resp := core.ListComputeHostsResponse{
		RawResponse: httpResponce,
		ComputeHostCollection: core.ComputeHostCollection{
			Items: make([]core.ComputeHostSummary, 0, len(c.model.Nodes)),
		},
	}

	var indx int
	from := getPage(req.Page)
	for indx = from; indx < from+*c.pageSize; indx++ {
		node := c.model.Nodes[c.instanceIDs[indx]]
		host := core.ComputeHostSummary{InstanceId: ptr.String(node.Name)}
		for i := 0; i < len(node.NetLayers) && i < 3; i++ {
			switch i {
			case 0:
				host.LocalBlockId = ptr.String(node.NetLayers[i])
			case 1:
				host.NetworkBlockId = ptr.String(node.NetLayers[i])
			case 2:
				host.HpcIslandId = ptr.String(node.NetLayers[i])
			}
		}
		if len(node.NVLink) != 0 {
			host.GpuMemoryFabricId = &node.NVLink
		}
		resp.Items = append(resp.Items, host)
	}

	if indx < len(c.instanceIDs) {
		resp.OpcNextPage = ptr.String(fmt.Sprintf("%d", indx))
	}

	return resp, nil
}

func getPage(page *string) int {
	if page == nil {
		return 0
	}

	val, _ := strconv.ParseInt(*page, 10, 32)
	return int(val)
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

	instanceIDs := make([]string, 0, len(model.Nodes))
	for _, node := range model.Nodes {
		instanceIDs = append(instanceIDs, node.Name)
	}

	clientFactory := func(region string, pageSize *int) (Client, error) {
		if p.APIError == errClientFactory {
			return nil, providers.APIError
		}
		if pageSize == nil {
			pageSize = ptr.Int(len(instanceIDs))
		}

		return &simClient{
			model:       model,
			pageSize:    pageSize,
			instanceIDs: instanceIDs,
			apiErr:      p.APIError,
		}, nil
	}

	return NewSim(clientFactory), nil
}

type simProvider struct {
	apiProvider
}

func NewSim(factory ClientFactory) *simProvider {
	return &simProvider{
		apiProvider: apiProvider{clientFactory: factory},
	}
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory("", nil)

	return client.(*simClient).model.Instances, nil
}
