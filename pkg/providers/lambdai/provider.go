/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lambdai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/mitchellh/mapstructure"

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
	trimTiers     int
}

type credentialsConfig struct {
	WorkspaceID string `mapstructure:"workspaceId"`
	Token       string `mapstructure:"token"`
}

type paramsConfig struct {
	BaseURL string `mapstructure:"url"`
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
	creds, err := decodeCredentials(config.Creds)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, "credentials error: "+err.Error())
	}
	params, err := decodeParams(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, "parameters error: "+err.Error())
	}
	trimTiers, err := providers.GetTrimTiers(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, "parameters error: "+err.Error())
	}

	clientFactory := func(pageSize *int) (Client, error) {
		return &lambdaiClient{
			workspaceID: creds.WorkspaceID,
			bearerToken: creds.Token,
			baseURL:     params.BaseURL,
			pageSize:    getPageSize(pageSize),
		}, nil
	}

	return New(clientFactory, trimTiers), nil
}

func decodeCredentials(creds map[string]any) (*credentialsConfig, error) {
	c := &credentialsConfig{}
	if err := mapstructure.Decode(creds, c); err != nil {
		return nil, err
	}

	for _, key := range []string{authWorkspaceID, authToken} {
		if v, ok := creds[key]; !ok || v == nil {
			return nil, fmt.Errorf("missing '%s'", key)
		}
	}

	return c, nil
}

func decodeParams(params map[string]any) (*paramsConfig, error) {
	p := &paramsConfig{}
	if err := mapstructure.Decode(params, p); err != nil {
		return nil, err
	}

	if p.BaseURL == "" {
		return nil, fmt.Errorf("missing '%s'", apiBaseURL)
	}

	return p, nil
}

func getPageSize(sz *int) int {
	if sz == nil {
		return defaultPageSize
	}
	return *sz
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, p.trimTiers, false), nil
}

type Provider struct {
	baseProvider
}

func New(clientFactory ClientFactory, trimTiers int) *Provider {
	return &Provider{
		baseProvider: baseProvider{
			clientFactory: clientFactory,
			trimTiers:     trimTiers,
		},
	}
}
