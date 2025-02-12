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

	compute_v1 "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	gax "github.com/googleapis/gax-go/v2"
	v1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "gcp"

type Provider struct {
	clientFactory ClientFactory
}

type ClientFactory func() (*Client, error)

type Client struct {
	Zones     ZonesClient
	Instances InstancesClient
}

type ZonesClient interface {
	List(ctx context.Context, req *computepb.ListZonesRequest, opts ...gax.CallOption) *compute_v1.ZoneIterator
}

type InstancesClient interface {
	List(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) *compute_v1.InstanceIterator
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	clientFactory := func() (*Client, error) {
		zonesClient, err := compute_v1.NewZonesRESTClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get zones client: %s", err.Error())
		}

		instancesClient, err := compute_v1.NewInstancesRESTClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get instances client: %s", err.Error())
		}

		return &Client{
			Zones:     zonesClient,
			Instances: instancesClient,
		}, nil
	}

	return New(clientFactory)
}

func New(clientFactory ClientFactory) (*Provider, error) {
	return &Provider{
		clientFactory: clientFactory,
	}, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	cfg, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return cfg.toGraph()
}

// Engine support

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	return instanceToNodeMap(ctx, nodes)
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion(ctx context.Context) (string, error) {
	return getRegion(ctx)
}

// GetNodeRegion implements k8s.k8sNodeInfo
func (p *Provider) GetNodeRegion(node *v1.Node) (string, error) {
	return node.Labels["topology.kubernetes.io/region"], nil
}

// GetNodeInstance implements k8s.k8sNodeInfo
func (p *Provider) GetNodeInstance(node *v1.Node) (string, error) {
	return node.Annotations["container.googleapis.com/instance_id"], nil
}
