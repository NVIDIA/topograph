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
	"time"

	compute_v1 "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/compute/metadata"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	v1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "gcp"

type baseProvider struct {
	clientFactory ClientFactory
}

type ClientFactory func() (Client, error)

type Client interface {
	ProjectID(ctx context.Context) (string, error)
	Zones(ctx context.Context, req *computepb.ListZonesRequest, opts ...gax.CallOption) []string
	Instances(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) ([]*computepb.Instance, string)
}

type gcpClient struct {
	zoneClient     *compute_v1.ZonesClient
	instanceClient *compute_v1.InstancesClient
}

func (c *gcpClient) ProjectID(ctx context.Context) (string, error) {
	return metadata.ProjectIDWithContext(ctx)
}

func (c *gcpClient) Zones(ctx context.Context, req *computepb.ListZonesRequest, opts ...gax.CallOption) []string {
	now := time.Now()
	iter := c.zoneClient.List(ctx, req, opts...)
	requestLatency.WithLabelValues("ListZones").Observe(time.Since(now).Seconds())

	zones := make([]string, 0)
	for {
		zone, err := iter.Next()
		if err == iterator.Done {
			break
		}
		zones = append(zones, *zone.Name)
	}

	return zones
}

func (c *gcpClient) Instances(ctx context.Context, req *computepb.ListInstancesRequest, opts ...gax.CallOption) ([]*computepb.Instance, string) {
	now := time.Now()
	iter := c.instanceClient.List(ctx, req, opts...)
	requestLatency.WithLabelValues("ListInstances").Observe(time.Since(now).Seconds())

	instances := make([]*computepb.Instance, 0)
	for {
		instance, err := iter.Next()
		if err == iterator.Done {
			break
		}
		instances = append(instances, instance)
	}

	return instances, iter.PageInfo().Token
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	clientFactory := func() (Client, error) {
		zoneClient, err := compute_v1.NewZonesRESTClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get zones client: %s", err.Error())
		}

		instanceClient, err := compute_v1.NewInstancesRESTClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get instances client: %s", err.Error())
		}

		return &gcpClient{
			zoneClient:     zoneClient,
			instanceClient: instanceClient,
		}, nil
	}

	return New(clientFactory), nil
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances)
}

type Provider struct {
	baseProvider
}

func New(clientFactory ClientFactory) *Provider {
	return &Provider{
		baseProvider: baseProvider{clientFactory: clientFactory},
	}
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
