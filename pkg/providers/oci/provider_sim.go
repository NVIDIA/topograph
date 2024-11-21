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

	"github.com/agrea/ptr"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "oci-sim"
)

type SimClient struct {
	Model *models.Model
}

var httpResponce = &http.Response{
	Status:     "200 OK",
	StatusCode: 200,
}

func (c *SimClient) TenancyOCID() *string {
	val := "simulation"
	return &val
}

func (c *SimClient) Limit() *int { return nil }

func (c *SimClient) ListAvailabilityDomains(ctx context.Context, req identity.ListAvailabilityDomainsRequest) (identity.ListAvailabilityDomainsResponse, error) {
	return identity.ListAvailabilityDomainsResponse{
		RawResponse: httpResponce,
		Items: []identity.AvailabilityDomain{
			{Name: ptr.String("ad")},
		},
	}, nil
}

func (c *SimClient) ListComputeHosts(ctx context.Context, req core.ListComputeHostsRequest) (core.ListComputeHostsResponse, error) {
	resp := core.ListComputeHostsResponse{
		RawResponse: httpResponce,
		ComputeHostCollection: core.ComputeHostCollection{
			Items: make([]core.ComputeHostSummary, 0, len(c.Model.Nodes)),
		},
	}

	for _, node := range c.Model.Nodes {
		host := core.ComputeHostSummary{Id: ptr.String(node.Name)}
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
		resp.Items = append(resp.Items, host)
	}

	return resp, nil
}

func (c *SimClient) ListComputeGpuMemoryFabrics(ctx context.Context, req core.ListComputeGpuMemoryFabricsRequest) (core.ListComputeGpuMemoryFabricsResponse, error) {
	blockMap := make(map[string]string)

	for _, node := range c.Model.Nodes {
		if len(node.NVLink) != 0 && len(node.NetLayers) != 0 {
			blockMap[node.NetLayers[0]] = node.NVLink
		}
	}

	resp := core.ListComputeGpuMemoryFabricsResponse{
		RawResponse: httpResponce,
		ComputeGpuMemoryFabricCollection: core.ComputeGpuMemoryFabricCollection{
			Items: make([]core.ComputeGpuMemoryFabricSummary, 0, len(blockMap)),
		},
	}

	for block, domain := range blockMap {
		resp.Items = append(resp.Items, core.ComputeGpuMemoryFabricSummary{
			Id:                  ptr.String(domain),
			ComputeLocalBlockId: ptr.String(block),
		})
	}

	return resp, nil
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(ctx context.Context, cfg providers.Config) (providers.Provider, error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, err
	}

	csp_model, err := models.NewModelFromFile(p.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("unable to load model file for simulation, %v", err)
	}
	simClient := &SimClient{Model: csp_model}

	clientFactory := func(region string, _ *int) (Client, error) {
		return simClient, nil
	}

	return NewSim(clientFactory), nil
}

type SimProvider struct {
	baseProvider
}

func NewSim(ociClientFactory ClientFactory) *SimProvider {
	return &SimProvider{
		baseProvider: baseProvider{clientFactory: ociClientFactory},
	}
}

// Engine support

func (p *SimProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory("", nil)

	return client.(*SimClient).Model.Instances, nil
}
