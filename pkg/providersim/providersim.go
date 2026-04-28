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

// Package providersim hosts auxiliary HTTP listeners for provider simulations (e.g. dsx-sim),
// managed alongside the main Topograph HTTP server via [github.com/oklog/run.Group].
package providersim

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// HandlerRegistry receives HTTP registrations before [Server.Start] runs.
type HandlerRegistry interface {
	RegisterHandler(pattern string, handler http.Handler)
}

// Server is an HTTP server for provider simulation APIs. Register handlers via
// [HandlerRegistry.RegisterHandler], then add [GetRunGroup] to an oklog run.Group.
type Server struct {
	mu sync.RWMutex

	mux *http.ServeMux
	// addr is the bind address passed to [Init].
	addr string

	httpS   *http.Server
	baseURL string
}

const (
	defaultAddr = "127.0.0.1:0"
)

var (
	defaultSrv *Server
	initOnce   sync.Once
)

// Init configures the process-wide provider-simulation HTTP server (typically "127.0.0.1:0").
// Safe to call once; subsequent calls are ignored.
func Init() {
	initOnce.Do(func() {
		defaultSrv = &Server{
			mux:  http.NewServeMux(),
			addr: defaultAddr,
		}
	})
}

// Default returns the server created by [Init]. Panics if [Init] was not called.
func Default() *Server {
	if defaultSrv == nil {
		panic("providersim: Init was not called")
	}
	return defaultSrv
}

// RegisterHandler implements [HandlerRegistry].
func (s *Server) RegisterHandler(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// BaseURL returns the listener base URL (scheme http, host:port) after [Server.Start] has bound.
// Empty until the server has listened.
func (s *Server) BaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baseURL
}

// Start binds and serves until shutdown or error. Intended for oklog/run.Group (blocking).
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	host, port := splitListenAddr(ln.Addr())
	switch {
	case host != "" && port > 0:
		s.baseURL = "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	default:
		s.baseURL = "http://" + ln.Addr().String()
	}
	s.httpS = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.mu.Unlock()

	klog.Infof("Starting provider simulation HTTP server on %s", ln.Addr().String())

	err = s.httpS.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop shuts down the server (oklog run.Group interrupt hook).
func (s *Server) Stop(reason error) {
	klog.Infof("Stopping provider simulation HTTP server: %v", reason)
	s.mu.Lock()
	srv := s.httpS
	s.mu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		klog.Errorf("provider simulation HTTP server shutdown: %v", err)
		_ = srv.Close()
	}
	klog.Infof("Stopped provider simulation HTTP server")
}

// GetRunGroup returns oklog/run.Group actors for [Server.Start] and [Server.Stop].
func GetRunGroup() (func() error, func(error)) {
	s := Default()
	return s.Start, s.Stop
}

// StartDefaultForTests runs [Init], applies register to [Default], then starts [GetRunGroup] in a
// goroutine until [Server.BaseURL] is non-empty. The returned stop function shuts down the server
// and waits for Serve to exit. Intended for TestMain in packages that load dsx-sim.
func StartDefaultForTests(register func(HandlerRegistry)) (stop func()) {
	Init()
	if register != nil {
		register(Default())
	}
	start, interrupt := GetRunGroup()
	done := make(chan struct{})
	go func() {
		_ = start()
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if Default().BaseURL() != "" {
			return func() {
				interrupt(nil)
				<-done
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	panic("providersim: StartDefaultForTests: timeout waiting for BaseURL")
}

func splitListenAddr(a net.Addr) (host string, port int) {
	full := a.String()
	if tcp, ok := a.(*net.TCPAddr); ok {
		if tcp.IP != nil {
			host = tcp.IP.String()
		}
		port = tcp.Port
		return host, port
	}
	h, p, err := net.SplitHostPort(full)
	if err != nil {
		return "", 0
	}
	n, _ := strconv.Atoi(p)
	return h, n
}
