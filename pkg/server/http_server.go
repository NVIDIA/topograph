/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/config"
	"github.com/NVIDIA/topograph/pkg/utils"
)

type HttpServer struct {
	ctx   context.Context
	cfg   *config.Config
	srv   *http.Server
	async *asyncController
}

type TopologyRequest struct {
	provider string
	engine   string
	params   map[string]string
	payload  *common.Payload
}

var srv *HttpServer

func InitHttpServer(ctx context.Context, cfg *config.Config) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/generate", generate)
	mux.HandleFunc("/v1/topology", getresult)
	mux.HandleFunc("/healthz", healthz)
	mux.Handle("/metrics", promhttp.Handler())

	srv = &HttpServer{
		ctx: ctx,
		cfg: cfg,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTP.Port),
			Handler: mux,
		},
		async: &asyncController{
			queue: utils.NewTrailingDelayQueue(processRequest, cfg.RequestAggregationDelay),
		},
	}
}

func GetRunGroup() (func() error, func(error)) {
	return srv.Start, srv.Stop
}

func (s *HttpServer) Start() error {
	if s.cfg.HTTP.SSL {
		klog.Infof("Starting HTTPS server on port %d", s.cfg.HTTP.Port)
		return s.srv.ListenAndServeTLS(s.cfg.SSL.Cert, s.cfg.SSL.Key)
	}
	klog.Infof("Starting HTTP server on port %d", s.cfg.HTTP.Port)
	return s.srv.ListenAndServe()
}

func (s *HttpServer) Stop(err error) {
	klog.Infof("Stopping HTTP server: %v", err)
	if err := s.srv.Shutdown(s.ctx); err != nil {
		klog.Errorf("Error during HTTP server shutdown: %v", err)
	}
	klog.Infof("Stopped HTTP server")
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func generate(w http.ResponseWriter, r *http.Request) {
	tr := readRequest(w, r)
	if tr == nil {
		return
	}

	uid := srv.async.queue.Submit(tr)

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(uid))
}

func readRequest(w http.ResponseWriter, r *http.Request) *TopologyRequest {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read request body", http.StatusInternalServerError)
		return nil
	}
	defer func() { _ = r.Body.Close() }()

	tr := &TopologyRequest{}

	tr.payload, err = common.GetPayload(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	tr.provider, tr.engine, tr.params, err = parseQuery(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	klog.InfoS("Topology request", "provider", tr.provider, "engine", tr.engine, "params", tr.params, "payload", tr.payload.String())

	if err = validate(tr); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	return tr
}

func parseQuery(vals url.Values) (string, string, map[string]string, error) {
	params := make(map[string]string)
	var provider, engine string

	for key, arr := range vals {
		switch key {
		case common.KeyProvider:
			provider = arr[0]
		case common.KeyEngine:
			engine = arr[0]
		default:
			params[key] = arr[0]
		}
	}

	if len(provider) == 0 {
		return "", "", nil, fmt.Errorf("missing provider URL query parameter")
	}
	if len(engine) == 0 {
		return "", "", nil, fmt.Errorf("missing engine URL query parameter")
	}

	return provider, engine, params, nil
}

func validate(tr *TopologyRequest) error {
	switch tr.provider {
	case common.ProviderAWS, common.ProviderOCI, common.ProviderGCP, common.ProviderCW, common.ProviderTest:
		//nop
	default:
		return fmt.Errorf("unsupported provider %s", tr.provider)
	}

	switch tr.engine {
	case common.EngineK8S:
		for _, key := range []string{common.KeyTopoConfigPath, common.KeyTopoConfigmapName, common.KeyTopoConfigmapNamespace} {
			if _, ok := tr.params[key]; !ok {
				return fmt.Errorf("missing %q URL query parameter", key)
			}
		}
	}

	return nil
}

func getresult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
		return
	}

	uid := r.URL.Query().Get(common.KeyUID)
	if len(uid) == 0 {
		http.Error(w, "must specify request uid", http.StatusBadRequest)
		return
	}

	res := srv.async.queue.Get(uid)
	if len(res.Message) != 0 {
		http.Error(w, res.Message, res.Status)
	} else {
		w.WriteHeader(res.Status)
		_, _ = w.Write(res.Ret.([]byte))
	}
}
