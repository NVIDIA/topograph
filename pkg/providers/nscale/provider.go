/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME = "nscale"

	urlTopologyPath = "/v1/topology"
)

type baseProvider struct {
	params *ProviderParams
	client Client
}

type ProviderParams struct {
	BaseURL   string `mapstructure:"baseUrl"`
	Region    string `mapstructure:"region"`
	TrimTiers int    `mapstructure:"trimTiers"`
}

type Credentials struct {
	Org   string `mapstructure:"org"`
	Token string `mapstructure:"token"`
}

type Client interface {
	Topology(context.Context, string, int, int) ([]InstanceTopology, error)
}

// nscaleClient is a Topology API client.
type nscaleClient struct {
	baseURL string
	org     string
	token   string
}

// InstanceTopology represents the topology of a single instance.
type InstanceTopology struct {
	ID          string   `json:"instance_id"`
	NetworkPath []string `json:"network_node_path"`
	BlockID     *string  `json:"block_id,omitempty"`
}

func (c *nscaleClient) Topology(ctx context.Context, region string, pageSize, offset int) ([]InstanceTopology, error) {
	headers := map[string]string{
		"Authorization":  "Bearer " + c.token,
		"X-Organization": c.org,
		"X-Region":       region,
	}
	query := map[string]string{
		"limit":  strconv.Itoa(pageSize),
		"offset": strconv.Itoa(offset),
	}
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.baseURL, urlTopologyPath)

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	resp := []InstanceTopology{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return resp, nil
}

type Provider struct {
	baseProvider
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	params, err := getParams(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	creds, err := getCreds(config.Creds)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	return &Provider{
		baseProvider: baseProvider{
			client: &nscaleClient{
				baseURL: params.BaseURL,
				org:     creds.Org,
				token:   creds.Token,
			},
			params: params,
		},
	}, nil
}

func getParams(params map[string]any) (*ProviderParams, error) {
	p := &ProviderParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %v", err)
	}
	if len(p.BaseURL) == 0 {
		return nil, fmt.Errorf("missing 'baseUrl'")
	}

	return p, nil
}

func getCreds(creds map[string]any) (*Credentials, error) {
	c := &Credentials{}
	if err := config.Decode(creds, c); err != nil {
		return nil, fmt.Errorf("failed to decode creds: %v", err)
	}
	if len(c.Org) == 0 {
		return nil, fmt.Errorf("missing 'org'")
	}
	if len(c.Token) == 0 {
		return nil, fmt.Errorf("missing 'token'")
	}

	return c, nil
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	return topo.ToThreeTierGraph(NAME, instances, p.params.TrimTiers, false), nil
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetInstancesRegions implements slurm.instanceMapper
func (p *Provider) GetInstancesRegions(ctx context.Context, nodes []string) (map[string]string, error) {
	res := make(map[string]string)
	for _, node := range nodes {
		res[node] = p.params.Region
	}

	return res, nil
}
