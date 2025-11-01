/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "nebius-sim"

	errNone = iota
	errClientFactory
	errInstances
	errTopologyPath
	errNetworkIntf
)

type simClient struct {
	model       *models.Model
	pageSize    int
	instanceIDs []string
	apiErr      int
}

func (c *simClient) ProjectID() string {
	return ""
}

func (c *simClient) PageSize() int64 {
	return int64(c.pageSize)
}

func (c *simClient) GetComputeInstanceList(ctx context.Context, req *compute.ListInstancesRequest) (*compute.ListInstancesResponse, error) {
	if c.apiErr == errInstances {
		return nil, providers.ErrAPIError
	}

	resp := &compute.ListInstancesResponse{
		Items: []*compute.Instance{},
	}

	var indx int
	from := getStartIndex(req.PageToken)
	for indx = from; indx < len(c.instanceIDs) && indx < from+c.pageSize; indx++ {
		node := c.model.Nodes[c.instanceIDs[indx]]
		instance := &compute.Instance{Status: &compute.InstanceStatus{}}

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

		if c.apiErr != errNetworkIntf {
			instance.Status.NetworkInterfaces = []*compute.NetworkInterfaceStatus{
				{
					Name:       "eth0",
					MacAddress: node.Name,
				},
			}
		}

		resp.Items = append(resp.Items, instance)
	}

	if indx < len(c.instanceIDs) {
		resp.NextPageToken = fmt.Sprintf("%d", indx)
	}

	return resp, nil
}

func getStartIndex(token string) int {
	if len(token) == 0 {
		return 0
	}
	val, _ := strconv.ParseInt(token, 10, 32)
	return int(val)
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(ctx context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	model, err := models.NewModelFromFile(p.ModelPath)
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

		return &simClient{
			model:       model,
			pageSize:    getPageSize(pageSize),
			instanceIDs: instanceIDs,
			apiErr:      p.APIError,
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

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	client, _ := p.clientFactory(nil)

	return client.(*simClient).model.Instances, nil
}
