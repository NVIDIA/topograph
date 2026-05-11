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

	urlTopologyPath  = "/v1/topology"
	urlInstancesPath = "/v2/instances"
)

type baseProvider struct {
	params *ProviderParams
	creds  *Credentials
	client Client
}

type ProviderParams struct {
	RadarApiUrl    string `mapstructure:"radarApiUrl"`
	InstanceAPIUrl string `mapstructure:"instanceApiUrl"`
	TrimTiers      int    `mapstructure:"trimTiers"`
}

type Credentials struct {
	Org    string `mapstructure:"org"`
	Token  string `mapstructure:"token"`
	Region string `mapstructure:"region"`
}

type Client interface {
	Topology(context.Context, string, int, int) ([]InstanceTopology, error)
	Instances(context.Context, string) (map[string]string, error)
}

// nscaleClient is a topology and instance API client.
type nscaleClient struct {
	radarAPIURL    string
	instanceAPIURL string
	org            string
	token          string
}

// InstanceTopology represents the topology of a single instance.
type InstanceTopology struct {
	ID          string   `json:"instance_id"`
	NetworkPath []string `json:"network_node_path"`
	BlockID     *string  `json:"block_id,omitempty"`
}

// TopologyResult represents the topology of a single instance.
type TopologyResult struct {
	Instances []InstanceTopology `json:"results"`
}

type instance struct {
	Metadata instanceMetadata `json:"metadata"`
}

type instanceMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.radarAPIURL, urlTopologyPath)

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	resp := TopologyResult{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return resp.Instances, nil
}

func (c *nscaleClient) Instances(ctx context.Context, region string) (map[string]string, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + c.token,
	}
	query := map[string]string{
		"organizationID": c.org,
		"regionID":       region,
	}
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.instanceAPIURL, urlInstancesPath)

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	instances := []instance{}
	if err := json.Unmarshal(body, &instances); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	i2n := make(map[string]string, len(instances))
	for _, instance := range instances {
		if instance.Metadata.ID == "" || instance.Metadata.Name == "" {
			continue
		}
		i2n[instance.Metadata.ID] = instance.Metadata.Name
	}

	return i2n, nil
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
				radarAPIURL:    params.RadarApiUrl,
				instanceAPIURL: params.InstanceAPIUrl,
				org:            creds.Org,
				token:          creds.Token,
			},
			params: params,
			creds:  creds,
		},
	}, nil
}

func getParams(params map[string]any) (*ProviderParams, error) {
	p := &ProviderParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %v", err)
	}
	if len(p.RadarApiUrl) == 0 {
		return nil, fmt.Errorf("missing 'radarApiUrl'")
	}
	if len(p.InstanceAPIUrl) == 0 {
		return nil, fmt.Errorf("missing 'instanceApiUrl'")
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
	if len(p.creds.Region) == 0 {
		return nil, fmt.Errorf("missing 'region'")
	}

	instances, err := p.client.Instances(ctx, p.creds.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	if len(nodes) == 0 {
		return instances, nil
	}

	nodeSet := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeSet[node] = struct{}{}
	}

	i2n := make(map[string]string, len(instances))
	for instanceID, node := range instances {
		if _, ok := nodeSet[node]; ok {
			i2n[instanceID] = node
		}
	}

	return i2n, nil
}

// GetInstancesRegions implements slurm.instanceMapper
func (p *Provider) GetInstancesRegions(ctx context.Context, nodes []string) (map[string]string, error) {
	if len(p.creds.Region) == 0 {
		return nil, fmt.Errorf("missing 'region'")
	}

	res := make(map[string]string)
	for _, node := range nodes {
		res[node] = p.creds.Region
	}

	return res, nil
}
