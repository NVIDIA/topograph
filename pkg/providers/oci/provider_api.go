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
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "oci"

	authTenancyID   = "tenancyId"
	authUserID      = "userId"
	authRegion      = "region"
	authFingerprint = "fingerprint"
	authPrivateKey  = "privateKey"
	authPassphrase  = "passphrase"
)

type apiProvider struct {
	baseProvider
	clientFactory ClientFactory
}

type ClientFactory func(region string, pageSize *int) (Client, error)

type Client interface {
	TenantID() *string
	Limit() *int
	ListAvailabilityDomains(context.Context, identity.ListAvailabilityDomainsRequest) (identity.ListAvailabilityDomainsResponse, error)
	ListComputeHosts(context.Context, core.ListComputeHostsRequest) (core.ListComputeHostsResponse, error)
}

type ociClient struct {
	identity.IdentityClient
	core.ComputeClient
	tenantID string
	limit    *int
}

func (c *ociClient) TenantID() *string {
	return &c.tenantID
}

func (c *ociClient) Limit() *int {
	return c.limit
}

func NamedLoaderAPI() (string, providers.Loader) {
	return NAME, LoaderAPI
}

func LoaderAPI(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	provider, httpErr := getConfigurationProvider(config.Creds)
	if httpErr != nil {
		return nil, httpErr
	}

	tenantID, err := provider.TenancyOCID()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("unable to get tenancy OCID from config: %v", err))
	}

	clientFactory := func(region string, pageSize *int) (Client, error) {
		identityClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
		if err != nil {
			return nil, fmt.Errorf("unable to create identity client: %v", err)
		}

		computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
		if err != nil {
			return nil, fmt.Errorf("unable to get compute client: %v", err)
		}

		if len(region) != 0 {
			klog.Infof("Use provided region %s", region)
			identityClient.SetRegion(region)
			computeClient.SetRegion(region)
		}

		return &ociClient{
			IdentityClient: identityClient,
			ComputeClient:  computeClient,
			tenantID:       tenantID,
			limit:          pageSize,
		}, nil
	}

	return NewAPI(clientFactory), nil
}

func getConfigurationProvider(creds map[string]string) (common.ConfigurationProvider, *httperr.Error) {
	if len(creds) != 0 {
		var tenancyID, userID, region, fingerprint, privateKey, passphrase string
		klog.Info("Using provided credentials")
		if tenancyID = creds[authTenancyID]; len(tenancyID) == 0 {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: missing tenancyId")
		}
		if userID = creds[authUserID]; len(userID) == 0 {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: missing userId")
		}
		if region = creds[authRegion]; len(region) == 0 {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: missing region")
		}
		if fingerprint = creds[authFingerprint]; len(fingerprint) == 0 {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: missing fingerprint")
		}
		if privateKey = creds[authPrivateKey]; len(privateKey) == 0 {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: missing privateKey")
		}
		passphrase = creds[authPassphrase]

		return common.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, privateKey, &passphrase), nil
	}

	klog.Info("No credentials provided, trying default configuration provider")
	configProvider := common.DefaultConfigProvider()
	_, err := configProvider.AuthType()
	if err == nil {
		return configProvider, nil
	}

	klog.Infof("No default configuration provider found: %v; trying instance principal configuration provider", err)
	configProvider, err = auth.InstancePrincipalConfigurationProvider()
	if err != nil {
		return nil, httperr.NewError(http.StatusUnauthorized, fmt.Sprintf("unable to authenticate API: %v", err))
	}

	return configProvider, nil
}

func NewAPI(clientFactory ClientFactory) *apiProvider {
	return &apiProvider{clientFactory: clientFactory}
}

func (p *apiProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, true), nil
}

func (p *apiProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, *httperr.Error) {
	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := p.getComputeHostInfo(ctx, pageSize, ci, topo); err != nil {
			return nil, err
		}
	}

	return topo, nil
}

func (p *apiProvider) getComputeHostInfo(ctx context.Context, pageSize *int, ci topology.ComputeInstances, topo *topology.ClusterTopology) *httperr.Error {
	if len(ci.Region) == 0 {
		return httperr.NewError(http.StatusBadRequest, "must specify region")
	}
	klog.Infof("Getting instance topology for %s region", ci.Region)

	client, err := p.clientFactory(ci.Region, pageSize)
	if err != nil {
		return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to create API client: %v", err))
	}

	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId: client.TenantID(),
	}

	start := time.Now()
	resp, err := client.ListAvailabilityDomains(ctx, req)
	reportLatency(resp.HTTPResponse(), start, "ListAvailabilityDomains")
	if err != nil {
		return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get availability domains: %v", err))
	}

	for _, ad := range resp.Items {
		err := getComputeHostSummary(ctx, client, ad.Name, topo, ci.Instances)
		if err != nil {
			return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get hosts info: %v", err))
		}
	}

	klog.V(4).Infof("Returning host info for %d nodes", topo.Len())

	return nil
}

// Engine support

// Instances2NodeMap implements slurm.instanceMapper
func (p *apiProvider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	return instanceToNodeMap(ctx, nodes)
}
