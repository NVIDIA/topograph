/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/nebius/gosdk"
	"github.com/nebius/gosdk/auth"
	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	services "github.com/nebius/gosdk/services/nebius/compute/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/version"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "nebius"

	authProjectID        = "projectId"
	authServiceAccountID = "serviceAccountId"
	authPublicKeyID      = "publicKeyId"
	authPrivateKey       = "privateKey"
	authTokenPath        = "/mnt/cloud-metadata/token"
	authTokenEnvVar      = "IAM_TOKEN"

	defaultPageSize int = 200
	userAgentProduct string = "nvidia-topograph"
)

type Client interface {
	ProjectID() string
	GetComputeInstanceList(context.Context, *compute.ListInstancesRequest) (*compute.ListInstancesResponse, error)
	PageSize() int64
}

type ClientFactory func(pageSize *int) (Client, error)

type baseProvider struct {
	clientFactory ClientFactory
}

type nebiusClient struct {
	instanceService services.InstanceService
	projectID       string
	pageSize        int
}

func (c *nebiusClient) ProjectID() string {
	return c.projectID
}

func (c *nebiusClient) PageSize() int64 {
	return int64(c.pageSize)
}

func (c *nebiusClient) GetComputeInstanceList(ctx context.Context, req *compute.ListInstancesRequest) (*compute.ListInstancesResponse, error) {
	return c.instanceService.List(ctx, req)
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	sdk, httpErr := getSDK(ctx, config.Creds)
	if httpErr != nil {
		return nil, httpErr
	}

	// if project ID is not passed in credentials, get it from file
	projectID, err := providers.StringFromMap(authProjectID, config.Creds, false)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, "credentials error: "+err.Error())
	}
	if len(projectID) == 0 {
		klog.Info("Project ID is not in credentials; getting from file")
		if projectID, err = getParentID(); err != nil {
			return nil, httperr.NewError(http.StatusInternalServerError, fmt.Sprintf("failed to get project ID: %v", err))
		}
	}

	klog.Infof("Project ID %s", projectID)

	instanceService := sdk.Services().Compute().V1().Instance()
	clientFactory := func(pageSize *int) (Client, error) {
		return &nebiusClient{
			instanceService: instanceService,
			projectID:       projectID,
			pageSize:        getPageSize(pageSize),
		}, nil
	}

	return New(clientFactory), nil
}

func getAuthOption(creds map[string]any) (gosdk.Option, *httperr.Error) {
	if len(creds) != 0 {
		klog.Info("Authentication with provided credentials")

		serviceAccountID, err := providers.StringFromMap(authServiceAccountID, creds, true)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: "+err.Error())
		}
		publicKeyID, err := providers.StringFromMap(authPublicKeyID, creds, true)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: "+err.Error())
		}
		privateKey, err := providers.StringFromMap(authPrivateKey, creds, true)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadRequest, "credentials error: "+err.Error())
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
			return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
		}
		return gosdk.WithCredentials(gosdk.IAMToken(token)), nil
	}

	return nil, httperr.NewError(http.StatusBadRequest, "missing authentication credentials")
}

func getSDK(ctx context.Context, creds map[string]any) (*gosdk.SDK, *httperr.Error) {
	opt, httpErr := getAuthOption(creds)
	if httpErr != nil {
		return nil, httpErr
	}

	sdk, err := gosdk.New(ctx, opt, gosdk.WithUserAgentPrefix(getUserAgentPrefix(version.Version)))
	if err != nil {
		return nil, httperr.NewError(http.StatusUnauthorized, fmt.Sprintf("failed to create gosdk: %v", err))
	}

	return sdk, nil
}

func getUserAgentPrefix(versionValue string) string {
	if strings.TrimSpace(versionValue) == "" {
		return userAgentProduct
	}
	return userAgentProduct + "/" + versionValue
}

func getPageSize(sz *int) int {
	if sz == nil {
		return defaultPageSize
	}
	return *sz
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, false), nil
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

// GetInstancesRegions implements slurm.instanceMapper
func (p *Provider) GetInstancesRegions(ctx context.Context, nodes []string) (map[string]string, error) {
	return getRegions(ctx, nodes)
}
