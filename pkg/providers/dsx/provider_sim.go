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
	"net/http"
	"net/url"
	"strings"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/providers/dsx/sim"
	"github.com/NVIDIA/topograph/pkg/providersim"
	"github.com/NVIDIA/topograph/pkg/topology"

	"k8s.io/klog/v2"
)

const NAME_SIM = "dsx-sim"

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(_ context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	trimTiers, terr := providers.GetTrimTiers(cfg.Params)
	if terr != nil {
		return nil, httperr.NewError(http.StatusBadRequest, "parameters error: "+terr.Error())
	}
	if p.TrimTiers != 0 {
		trimTiers = p.TrimTiers
	}

	sim.RegisterHTTP(NAME_SIM, providersim.Default())

	// Local HTTP server is started by cmd/topograph via [providersim]; URL comes from the shared listener.
	baseURL := providersim.Default().BaseURL()
	if baseURL == "" {
		return nil, httperr.NewError(http.StatusInternalServerError, "dsx-sim: provider simulation HTTP server is not listening yet")
	}

	stem := stemFromModelFileName(p.ModelFileName)
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, "dsx-sim: invalid listener URL: "+err.Error())
	}
	prefix := strings.Trim(NAME_SIM, "/")
	switch {
	case base.Path == "" || base.Path == "/":
		base.Path = "/" + prefix
	default:
		base.Path = strings.TrimSuffix(base.Path, "/") + "/" + prefix
	}
	q := base.Query()
	q.Set(sim.QueryParamFilePath, stem+".yaml")
	base.RawQuery = q.Encode()

	klog.InfoS("DSX sim provider", "baseURL", base.String(), "model", stem+".yaml")

	params := &Params{
		BaseURL:     base.String(),
		BearerToken: "sim",
	}

	factory := func() (Client, error) {
		switch p.APIError {
		case simAPIErrClientFactory:
			return nil, providers.ErrAPIError
		case simAPIErrGetTopology:
			return errTopologyClient{}, nil
		default:
			return newAPIClient(params), nil
		}
	}

	return &simProvider{
		Provider: &Provider{
			clientFactory: factory,
			params:        params,
			trimTiers:     trimTiers,
			providerName:  NAME_SIM,
		},
	}, nil
}

// simProvider wraps [Provider] so dsx-sim can expose [simProvider.GetComputeInstances] for empty-node API requests without implementing it on the real DSX provider.
type simProvider struct {
	*Provider
}

// GetComputeInstances returns all instances from an unconstrained topology response (same path as [Provider.GenerateTopologyConfig] with no instances).
func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	_, cisEff, herr := p.generateInstanceTopology(ctx, nil, nil)
	if herr != nil {
		return nil, herr
	}
	return cisEff, nil
}

// errTopologyClient implements [Client] and always fails GetTopology with [providers.ErrAPIError]
// (simulation parameter api_error mapping to topology fetch failure).
type errTopologyClient struct{}

func (errTopologyClient) GetTopology(context.Context, string, []string, []topology.ComputeInstances) (*TopologyResponse, []topology.ComputeInstances, error) {
	return nil, nil, providers.ErrAPIError
}
