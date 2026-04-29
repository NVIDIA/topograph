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

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/providers/dsx/sim"

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

	// Local HTTP server serves embedded responses/<stem>.json via filePath= (same routes as the real DSX apiClient).
	// It is not stopped when the provider is constructed; it lives for the process lifetime (typical for dsx-sim).
	ls, err := sim.ListenAndServe("127.0.0.1:0")
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, "dsx-sim: "+err.Error())
	}

	stem := stemFromModelFileName(p.ModelFileName)
	base, err := url.Parse(ls.GetURL())
	if err != nil {
		_ = ls.Close()
		return nil, httperr.NewError(http.StatusInternalServerError, "dsx-sim: invalid listener URL: "+err.Error())
	}
	q := base.Query()
	q.Set(sim.QueryParamFilePath, stem+".json")
	base.RawQuery = q.Encode()

	klog.InfoS("DSX sim provider", "baseURL", base.String(), "response", stem+".json")

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

	return &Provider{
		clientFactory: factory,
		params:        params,
		trimTiers:     trimTiers,
		providerName:  NAME_SIM,
	}, nil
}

// errTopologyClient implements [Client] and always fails GetTopology with [providers.ErrAPIError]
// (simulation parameter api_error mapping to topology fetch failure).
type errTopologyClient struct{}

func (errTopologyClient) GetTopology(context.Context, string, []string, int, string) (*TopologyResponse, error) {
	return nil, providers.ErrAPIError
}
