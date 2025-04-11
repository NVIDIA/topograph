/*
 * Copyright (c) 2024-2025, NVIDIA CORPORATION.  All rights reserved.
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

package node_data_broker

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
)

type Server struct {
	ctx  context.Context
	port int
	srv  *http.Server
}

func NewServer(ctx context.Context, port int) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/test1", test1)
	mux.HandleFunc("/test2", test2)

	return &Server{
		ctx:  ctx,
		port: port,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}
}

func (s *Server) Start() error {
	klog.Infof("Starting NodeDataBroker server on port %d", s.port)
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(err error) {
	klog.Infof("Stopping NodeDataBroker server: %v", err)
	if err := s.srv.Shutdown(s.ctx); err != nil {
		klog.Errorf("Error during HTTP server shutdown: %v", err)
	}
	klog.Infof("Stopped HTTP server")
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func test1(w http.ResponseWriter, r *http.Request) {
	klog.Info("service test1")
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusBadRequest)
		return
	}
}

func test2(w http.ResponseWriter, r *http.Request) {
	klog.Info("service test2")
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusBadRequest)
		return
	}
}
