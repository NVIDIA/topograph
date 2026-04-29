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

package sim

import (
	"errors"
	"net"
	"net/http"
	"strconv"
)

// ListenServer is returned by [ListenAndServe]. It reports the bound listening address
// and blocks on [ListenServer.Wait] until the server exits.
type ListenServer struct {
	// Addr is the listener address in host:port form (see [net.Listener.Addr]).
	Addr string
	// Host is the bound IP address string for TCP listeners (may be empty for some configs).
	Host string
	// Port is the TCP port the server bound to.
	Port int

	httpS *http.Server
	errCh chan error
}

// Wait blocks until the HTTP server stops, then returns the result of [http.Server.Serve]
// (usually [http.ErrServerClosed] after [ListenServer.Close]).
func (l *ListenServer) Wait() error {
	if l == nil {
		return nil
	}
	err := <-l.errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close shuts down the server (see [http.Server.Close]).
func (l *ListenServer) Close() error {
	if l == nil || l.httpS == nil {
		return nil
	}
	return l.httpS.Close()
}

// GetURL returns the HTTP base URL for this listener, always including an explicit host:port
// (scheme http). Uses [ListenServer.Host] and [ListenServer.Port] when set; otherwise [ListenServer.Addr].
func (l *ListenServer) GetURL() string {
	if l == nil {
		return ""
	}
	var hostPort string
	switch {
	case l.Host != "" && l.Port > 0:
		hostPort = net.JoinHostPort(l.Host, strconv.Itoa(l.Port))
	case l.Addr != "":
		hostPort = l.Addr
	default:
		return ""
	}
	// Build without url.URL.String() so the port is never elided for default http(s) ports.
	return "http://" + hostPort
}

// ListenAndServe binds addr, starts serving [EmbeddedResponsesFS] handlers, and returns a
// [ListenServer] describing the effective listening address and port. Call [ListenServer.Wait]
// to block until the process should exit, or [ListenServer.Close] to stop serving.
func ListenAndServe(addr string) (*ListenServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	ls := newListenServer(ln)
	ls.httpS = &http.Server{
		Handler: NewServer(EmbeddedResponsesFS()).Handler(),
	}
	go func() {
		ls.errCh <- ls.httpS.Serve(ln)
	}()

	return ls, nil
}

func newListenServer(ln net.Listener) *ListenServer {
	host, port, addr := splitListenAddr(ln.Addr())
	return &ListenServer{
		Addr:  addr,
		Host:  host,
		Port:  port,
		errCh: make(chan error, 1),
	}
}

func splitListenAddr(a net.Addr) (host string, port int, full string) {
	full = a.String()
	if tcp, ok := a.(*net.TCPAddr); ok {
		if tcp.IP != nil {
			host = tcp.IP.String()
		}
		port = tcp.Port
		return host, port, full
	}
	h, p, err := net.SplitHostPort(full)
	if err != nil {
		return "", 0, full
	}
	n, _ := strconv.Atoi(p)
	return h, n, full
}
