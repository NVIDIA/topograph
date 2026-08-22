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

	urlTopologyPath         = "/v1/topology"
	urlPlacementsPath       = "/api/v2/placements"
	urlPlacementServersPath = "/api/v2/placements/%s/servers"
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
	ListPlacements(ctx context.Context, org, region string) ([]string, error)
	PlacementServers(context.Context, string) (map[string]string, error)
}

// nscaleClient is a Radar topology, Placements, and Placement Servers API client.
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

type placement struct {
	Metadata placementMetadata `json:"metadata"`
}

type placementMetadata struct {
	ID string `json:"id"`
}

type placementServer struct {
	Metadata placementServerMetadata `json:"metadata"`
}

type placementServerMetadata struct {
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

func (c *nscaleClient) ListPlacements(ctx context.Context, org, region string) ([]string, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + c.token,
	}
	query := map[string]string{
		"organizationID": org,
		"regionID":       region,
	}
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.instanceAPIURL, urlPlacementsPath)

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	placements := []placement{}
	if err := json.Unmarshal(body, &placements); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	ids := make([]string, 0, len(placements))
	for _, p := range placements {
		if p.Metadata.ID == "" {
			continue
		}
		ids = append(ids, p.Metadata.ID)
	}

	return ids, nil
}

func (c *nscaleClient) PlacementServers(ctx context.Context, placementID string) (map[string]string, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + c.token,
	}
	path := fmt.Sprintf(urlPlacementServersPath, placementID)
	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, nil, nil, c.instanceAPIURL, path)

	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	servers := []placementServer{}
	if err := json.Unmarshal(body, &servers); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	i2n := make(map[string]string, len(servers))
	for _, s := range servers {
		if s.Metadata.ID == "" || s.Metadata.Name == "" {
			continue
		}
		i2n[s.Metadata.ID] = s.Metadata.Name
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

	return topo.ToGraph(NAME, instances, p.params.TrimTiers, false), nil
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	if len(p.creds.Region) == 0 {
		return nil, fmt.Errorf("missing 'region'")
	}

	placementIDs, err := p.client.ListPlacements(ctx, p.creds.Org, p.creds.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to list placements: %w", err)
	}

	instances := make(map[string]string)
	for _, placementID := range placementIDs {
		servers, err := p.client.PlacementServers(ctx, placementID)
		if err != nil {
			return nil, fmt.Errorf("failed to get placement servers for placement %s: %w", placementID, err)
		}
		for id, node := range servers {
			instances[id] = node
		}
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
