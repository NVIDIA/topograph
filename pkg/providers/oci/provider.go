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

	OCICommon "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "oci"

type Provider struct {
	clientFactory ClientFactory
}

type ClientFactory func(region string) (Client, error)

type Client interface {
	TenancyOCID() string
	ListAvailabilityDomains(ctx context.Context, request identity.ListAvailabilityDomainsRequest) (response identity.ListAvailabilityDomainsResponse, err error)
	ListComputeCapacityTopologies(ctx context.Context, request core.ListComputeCapacityTopologiesRequest) (response core.ListComputeCapacityTopologiesResponse, err error)
	ListComputeCapacityTopologyComputeBareMetalHosts(ctx context.Context, request core.ListComputeCapacityTopologyComputeBareMetalHostsRequest) (response core.ListComputeCapacityTopologyComputeBareMetalHostsResponse, err error)
}

type ociClient struct {
	identity.IdentityClient
	core.ComputeClient
	tenancyOCID string
}

func (c *ociClient) TenancyOCID() string {
	return c.tenancyOCID
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	provider, err := getConfigurationProvider(config.Creds)
	if err != nil {
		return nil, err
	}

	clientFactory := func(region string) (Client, error) {
		identityClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
		if err != nil {
			return nil, fmt.Errorf("unable to create identity client. Bailing out : %v", err)
		}

		tenacyOCID, err := provider.TenancyOCID()
		if err != nil {
			return nil, fmt.Errorf("unable to get tenancy OCID from config: %s", err.Error())
		}

		computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
		if err != nil {
			return nil, fmt.Errorf("unable to get compute client: %s", err.Error())
		}

		if len(region) != 0 {
			klog.Infof("Use provided region %s", region)
			identityClient.SetRegion(region)
			computeClient.SetRegion(region)
		}

		return &ociClient{
			IdentityClient: identityClient,
			ComputeClient:  computeClient,
			tenancyOCID:    tenacyOCID,
		}, nil
	}

	return New(clientFactory), nil
}

func getConfigurationProvider(creds map[string]string) (OCICommon.ConfigurationProvider, error) {
	if len(creds) != 0 {
		var tenancyID, userID, region, fingerprint, privateKey, passphrase string
		klog.Info("Using provided credentials")
		if tenancyID = creds["tenancy_id"]; len(tenancyID) == 0 {
			return nil, fmt.Errorf("credentials error: missing tenancy_id")
		}
		if userID = creds["user_id"]; len(userID) == 0 {
			return nil, fmt.Errorf("credentials error: missing user_id")
		}
		if region = creds["region"]; len(region) == 0 {
			return nil, fmt.Errorf("credentials error: missing region")
		}
		if fingerprint = creds["fingerprint"]; len(fingerprint) == 0 {
			return nil, fmt.Errorf("credentials error: missing fingerprint")
		}
		if privateKey = creds["private_key"]; len(privateKey) == 0 {
			return nil, fmt.Errorf("credentials error: missing private_key")
		}
		passphrase = creds["passphrase"]

		return OCICommon.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, privateKey, &passphrase), nil
	}

	klog.Info("No credentials provided, trying default configuration provider")
	configProvider := OCICommon.DefaultConfigProvider()
	_, err := configProvider.AuthType()
	if err == nil {
		return configProvider, nil
	}

	klog.Infof("No default configuration provider found: %v; trying instance principal configuration provider", err)
	configProvider, err = auth.InstancePrincipalConfigurationProvider()

	if err != nil {
		return nil, fmt.Errorf("unable to authenticate API: %s", err.Error())
	}

	return configProvider, nil
}

func New(ociClientFactory ClientFactory) *Provider {
	return &Provider{
		clientFactory: ociClientFactory,
	}
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	topo, err := GenerateInstanceTopology(ctx, p.clientFactory, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, true)
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
	return node.Spec.ProviderID, nil
}
