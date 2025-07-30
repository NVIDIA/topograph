/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"
	"os"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	services "github.com/nebius/gosdk/services/nebius/compute/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "nebius"

	authServiceAccountID = "service-account-id"
	authPublicKeyID      = "public-key-id"
	authPrivateKey       = "private-key"
	authTokenPath        = "/mnt/cloud-metadata/token"
	authTokenEnvVar      = "IAM_TOKEN"

	defaultPageSize int = 200
)

type Client interface {
	ProjectID() (string, error)
	GetComputeInstanceList(context.Context, *compute.ListInstancesRequest) (*compute.ListInstancesResponse, error)
	PageSize() int64
}

type ClientFactory func(pageSize *int) (Client, error)

type baseProvider struct {
	clientFactory ClientFactory
}

type nebiusClient struct {
	instanceService services.InstanceService
	pageSize        int
}

func (c *nebiusClient) ProjectID() (string, error) {
	return getParentID()
}

func (c *nebiusClient) GetComputeInstanceList(ctx context.Context, req *compute.ListInstancesRequest) (*compute.ListInstancesResponse, error) {
	return c.instanceService.List(ctx, req)
}

func (c *nebiusClient) PageSize() int64 {
	return int64(c.pageSize)
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	sdk, err := getSDK(ctx, config.Creds)
	if err != nil {
		return nil, err
	}

	instanceService := sdk.Services().Compute().V1().Instance()
	clientFactory := func(pageSize *int) (Client, error) {
		return &nebiusClient{
			instanceService: instanceService,
			pageSize:        getPageSize(pageSize),
		}, nil
	}

	return New(clientFactory), nil
}

func getAuthOption(creds map[string]string) (gosdk.Option, error) {
	if len(creds) != 0 {
		klog.Info("Authentication with provided credentials")

		var serviceAccountID, publicKeyID, privateKey string
		if serviceAccountID = creds[authServiceAccountID]; len(serviceAccountID) == 0 {
			return nil, fmt.Errorf("credentials error: missing %s", authServiceAccountID)
		}
		if publicKeyID = creds[authPublicKeyID]; len(publicKeyID) == 0 {
			return nil, fmt.Errorf("credentials error: missing %s", authPublicKeyID)
		}
		if privateKey = creds[authPrivateKey]; len(privateKey) == 0 {
			return nil, fmt.Errorf("credentials error: missing %s", authPrivateKey)
		}

		return gosdk.WithCredentials(
			gosdk.ServiceAccountReader(
				auth.NewPrivateKeyParser([]byte(privateKey), publicKeyID, serviceAccountID))), nil
	}

	if token := os.Getenv(authTokenEnvVar); len(token) != 0 {
		klog.Info("Authentication with provided IAM token")
		return gosdk.WithCredentials(gosdk.IAMToken(token)), nil
	}

	if _, err := os.Stat(authTokenPath); err == nil || !os.IsNotExist(err) {
		klog.Infof("Authentication with %s", authTokenPath)
		token, err := providers.ReadFile(authTokenPath)
		if err != nil {
			return nil, err
		}
		return gosdk.WithCredentials(gosdk.IAMToken(token)), nil
	}

	return nil, fmt.Errorf("missing authentication credentials")
}

func getSDK(ctx context.Context, creds map[string]string) (*gosdk.SDK, error) {
	opt, err := getAuthOption(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to create gosdk: %v", err)
	}

	sdk, err := gosdk.New(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to create gosdk: %v", err)
	}

	return sdk, nil
}

func getPageSize(sz *int) int {
	if sz == nil {
		return defaultPageSize
	}
	return *sz
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, false)
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
func (p *Provider) GetComputeInstancesRegion(_ context.Context) (string, error) {
	return getRegion()
}
