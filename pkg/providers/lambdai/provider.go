/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lambdai

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "lambdai"

	authWorkspaceID = "workspaceId"
	authToken       = "token"
	apiBaseURL      = "url"

	defaultPageSize = 200
)

type Client interface {
	WorkspaceID() string
	InstanceList(context.Context, *InstanceListRequest) (*InstanceListResponse, error)
	PageSize() int
}

type ClientFactory func(pageSize *int) (Client, error)

type baseProvider struct {
	clientFactory ClientFactory
}

// lambdaiClient is a Topology API client.
type lambdaiClient struct {
	baseURL     string
	bearerToken string
	workspaceID string
	pageSize    int
}

// InstanceTopology represents the topology of a single instance.
type InstanceTopology struct {
	ID          string      `json:"id"`
	NetworkPath []string    `json:"networkPath"`
	NVLink      *NVLinkInfo `json:"nvlink,omitempty"`
}

type InstanceListRequest struct {
	PageSize  int
	PageToken string
}

type InstanceListResponse struct {
	Items         []InstanceTopology
	NextPageToken string
}

// NVLinkInfo represents NVLink domain information.
type NVLinkInfo struct {
	DomainID string `json:"domain_id,omitempty"`
	CliqueID string `json:"clique_id,omitempty"`
}

func (c *lambdaiClient) WorkspaceID() string {
	return c.workspaceID
}

func (c *lambdaiClient) PageSize() int {
	return c.pageSize
}

func (c *lambdaiClient) InstanceList(ctx context.Context, req *InstanceListRequest) (*InstanceListResponse, error) {
	headers := map[string]string{"Authorization": "Bearer " + c.bearerToken}
	query := map[string]string{"workspace_id": c.workspaceID}
	// TODO: follow up on correct pagination
	if req.PageSize > 0 {
		query["page_size"] = strconv.Itoa(req.PageSize)
	}
	if req.PageToken != "" {
		query["page_token"] = req.PageToken
	}
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.baseURL, "/api/v1/instance-topology")

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	resp := &InstanceListResponse{Items: []InstanceTopology{}}
	if err := json.Unmarshal(body, &resp.Items); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	// TODO: follow up on correct page token
	resp.NextPageToken = ""

	return resp, nil
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	workspaceID, ok := config.Creds[authWorkspaceID]
	if !ok {
		return nil, httperr.NewError(http.StatusBadRequest, "missing workspace ID")
	}
	token, ok := config.Creds[authToken]
	if !ok {
		return nil, httperr.NewError(http.StatusBadRequest, "missing bearer token")
	}
	baseURL, ok := config.Params[apiBaseURL]
	if !ok {
		return nil, httperr.NewError(http.StatusBadRequest, "missing API URL")
	}

	clientFactory := func(pageSize *int) (Client, error) {
		return &lambdaiClient{
			workspaceID: workspaceID,
			bearerToken: token,
			baseURL:     baseURL.(string),
			pageSize:    getPageSize(pageSize),
		}, nil
	}

	return New(clientFactory), nil
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
