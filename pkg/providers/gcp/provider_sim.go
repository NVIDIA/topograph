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
	"net/http"
	"strconv"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/agrea/ptr"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "gcp-sim"

	errNone = iota
	errClientFactory
	errProjectID
	errInstances
)

type simClient struct {
	model       *models.Model
	pageSize    *uint32
	instanceIDs []string
	apiErr      int
}

type simInstanceIter struct {
	instances []*computepb.Instance
	indx      int
	err       error
}

func (iter *simInstanceIter) Next() (*computepb.Instance, error) {
	if iter.err != nil {
		return nil, iter.err
	}

	if iter.indx >= len(iter.instances) {
		return nil, iterator.Done
	}
	ret := iter.instances[iter.indx]
	iter.indx++

	return ret, nil
}

func (c *simClient) PageSize() *uint32 {
	return c.pageSize
}

func (c *simClient) ProjectID(ctx context.Context) (string, error) {
	if c.apiErr == errProjectID {
		return "", providers.ErrAPIError
	}

	return "", nil
}

func (c *simClient) Instances(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) (InstanceIterator, string) {
	if c.apiErr == errInstances {
		return &simInstanceIter{err: providers.ErrAPIError}, ""
	}

	var indx int
	from := getPage(req.PageToken)
	iter := &simInstanceIter{instances: make([]*computepb.Instance, 0)}

	for indx = from; indx < from+int(*c.pageSize); indx++ {
		node := c.model.Nodes[c.instanceIDs[indx]]
		instanceID, err := strconv.ParseUint(node.Name, 10, 64)
		if err != nil {
			return &simInstanceIter{err: fmt.Errorf("invalid instance ID %q; must be numerical", node.Name)}, ""
		}
		instance := &computepb.Instance{
			Id:   &instanceID,
			Name: &node.Name,
			ResourceStatus: &computepb.ResourceStatus{
				PhysicalHostTopology: &computepb.ResourceStatusPhysicalHostTopology{
					Cluster:  &node.NetLayers[2],
					Block:    &node.NetLayers[1],
					Subblock: &node.NetLayers[0],
				},
			},
		}
		iter.instances = append(iter.instances, instance)
	}

	var token string
	if indx < len(c.instanceIDs) {
		token = fmt.Sprintf("%d", indx)
	}

	return iter, token
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

		limit := castPageSize(pageSize)
		if limit == nil {
			limit = ptr.Uint32(uint32(len(instanceIDs)))
		}

		return &simClient{
			model:       model,
			pageSize:    limit,
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
	client, _ := p.clientFactory(nil)

	return client.(*simClient).model.Instances, nil
}
