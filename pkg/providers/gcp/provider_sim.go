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

package gcp

import (
	"context"
	"fmt"
	"strconv"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	gax "github.com/googleapis/gax-go/v2"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "gcp-sim"
)

type SimClient struct {
	Model *models.Model
}

func (c *SimClient) ProjectID(ctx context.Context) (string, error) {
	return "", nil
}

func (c *SimClient) Zones(ctx context.Context, req *computepb.ListZonesRequest, opts ...gax.CallOption) []string {
	return []string{"zone"}
}

func (c *SimClient) Instances(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) ([]*computepb.Instance, string) {
	instances := make([]*computepb.Instance, 0, len(c.Model.Nodes))

	for _, node := range c.Model.Nodes {
		physicalHost := fmt.Sprintf("/%s/%s/%s", node.NetLayers[1], node.NetLayers[0], node.Name)
		instanceID, _ := strconv.ParseUint(node.Name, 10, 64)
		instance := &computepb.Instance{
			Id:   &instanceID,
			Name: &node.Name,
			ResourceStatus: &computepb.ResourceStatus{
				PhysicalHost: &physicalHost,
			},
		}
		instances = append(instances, instance)
	}

	return instances, ""
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
	simClient := &SimClient{
		Model: csp_model,
	}

	clientFactory := func() (Client, error) {
		return simClient, nil
	}

	return NewSim(clientFactory), nil
}

type SimProvider struct {
	baseProvider
}

func NewSim(clientFactory ClientFactory) *SimProvider {
	return &SimProvider{
		baseProvider: baseProvider{clientFactory: clientFactory},
	}
}

// Engine support

func (p *SimProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory()

	return client.(*SimClient).Model.Instances, nil
}
