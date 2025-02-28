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
	"google.golang.org/api/iterator"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "gcp-sim"
)

type simClient struct {
	model *models.Model
	pages []*simInstanceIter
}

type simInstanceIter struct {
	instances []*computepb.Instance
	indx      int
	next      bool
	err       bool
}

func (iter *simInstanceIter) Next() (*computepb.Instance, error) {
	if iter.err {
		return nil, fmt.Errorf("iterator error")
	}

	if iter.indx >= len(iter.instances) {
		return nil, iterator.Done
	}
	ret := iter.instances[iter.indx]
	iter.indx++

	return ret, nil
}

func newSimClient(model *models.Model) (*simClient, error) {
	// divide nodes into 2 pages
	n := len(model.Nodes)
	nodeNames := make([]string, 0, n)
	for name := range model.Nodes {
		nodeNames = append(nodeNames, name)
	}
	mid := n / 2
	pages := make([]*simInstanceIter, 2)

	for i, pair := range []struct{ from, to int }{
		{from: 0, to: mid},
		{from: mid + 1, to: n - 1},
	} {
		if pair.from > pair.to {
			pages[i] = &simInstanceIter{}
		} else {
			instances := make([]*computepb.Instance, 0, pair.to-pair.from+1)
			for j := pair.from; j <= pair.to; j++ {
				node := model.Nodes[nodeNames[j]]
				physicalHost := fmt.Sprintf("/%s/%s/%s", node.NetLayers[1], node.NetLayers[0], node.Name)
				instanceID, err := strconv.ParseUint(node.Name, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid instance ID %q; must be numerical", node.Name)
				}
				instance := &computepb.Instance{
					Id:   &instanceID,
					Name: &node.Name,
					ResourceStatus: &computepb.ResourceStatus{
						PhysicalHost: &physicalHost,
					},
				}
				instances = append(instances, instance)
			}
			pages[i] = &simInstanceIter{instances: instances}
		}
	}

	pages[0].next = true

	return &simClient{
		model: model,
		pages: pages,
	}, nil
}

func (c *simClient) ProjectID(ctx context.Context) (string, error) {
	return "", nil
}

func (c *simClient) Instances(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) (InstanceIterator, string) {
	if req.PageToken == nil {
		return c.pages[0], "next"
	} else {
		return c.pages[1], ""
	}
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
		return nil, fmt.Errorf("failed to load model file for simulation: %v", err)
	}

	client, err := newSimClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create simulation client: %v", err)
	}

	clientFactory := func() (Client, error) {
		return client, nil
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

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory()

	return client.(*simClient).model.Instances, nil
}
