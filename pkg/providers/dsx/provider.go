/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package dsx

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

// NAME is the provider registry key for DSX topology.
const NAME = "dsx"

type Provider struct {
	clientFactory ClientFactory
	params        *Params
	trimTiers     int
	providerName  string
}

type Params struct {
	BaseURL         string `mapstructure:"baseUrl"`
	BearerToken     string `mapstructure:"bearerToken"`
	VpcID           string `mapstructure:"vpcId"`
	PageSize        int    `mapstructure:"pageSize"`
	InsecureSkipTLS bool   `mapstructure:"insecureSkipTLS"`
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(_ context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	p, err := loadParams(cfg)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	trimTiers, terr := providers.GetTrimTiers(cfg.Params)
	if terr != nil {
		return nil, httperr.NewError(http.StatusBadRequest, terr.Error())
	}
	if err := validateParams(p); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	factory := func() (Client, error) {
		return newAPIClient(p), nil
	}
	klog.InfoS("DSX provider", "baseURL", p.BaseURL, "vpcID", p.VpcID)
	return &Provider{clientFactory: factory, params: p, trimTiers: trimTiers, providerName: NAME}, nil
}

func loadParams(cfg providers.Config) (*Params, error) {
	p := &Params{}
	if err := config.Decode(cfg.Params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %w", err)
	}

	return p, nil
}

func validateParams(p *Params) error {
	if len(p.BaseURL) == 0 {
		return fmt.Errorf("baseUrl not provided")
	}
	if len(p.BearerToken) == 0 {
		return fmt.Errorf("bearerToken not provided")
	}
	if len(p.VpcID) == 0 {
		return fmt.Errorf("vpcId not provided")
	}
	return nil
}

// GetComputeInstances implements optional engine support when a request does not include node lists
// (e.g. Slurm block topology). It reuses the same [Provider.generateInstanceTopology] path as [Provider.GenerateTopologyConfig].
func (p *Provider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	_, cisEff, herr := p.generateInstanceTopology(ctx, nil, nil)
	if herr != nil {
		return nil, herr
	}
	return cisEff, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	cluster, cisEff, herr := p.generateInstanceTopology(ctx, pageSize, cis)
	if herr != nil {
		return nil, herr
	}
	return cluster.ToThreeTierGraph(p.providerName, cisEff, p.trimTiers, false), nil
}
