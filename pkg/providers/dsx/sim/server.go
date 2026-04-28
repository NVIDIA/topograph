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
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// Server serves DSX topology simulation APIs by returning JSON from a filesystem path
// (when [QueryParamFilePath] is absolute and constrained by [Server.AbsResponseRoot]) or by converting a YAML model from tests/models
// (`filePath` basename stem.json / stem.yaml / stem) into DSX topology JSON.
// The path segment `vpcs/{vpcID}` is ignored for file selection; Authorization must be a non-empty
// Bearer token (value unused), matching the real API client.
type Server struct {
	// AbsResponseRoot limits absolute filePath query values to paths under this directory.
	// When empty, absolute filePath is rejected (see [readResponseBytesAbsolute]). [NewServer] defaults this from the environment or [os.TempDir].
	AbsResponseRoot string
}

// NewServer returns a server that resolves relative filePath values against embedded
// tests/models/<stem>.yaml (see [readResponseBytes]). Absolute paths are confined under
// [EnvAbsResponseRoot] when set, otherwise under the process temp directory ([os.TempDir]).
func NewServer() *Server {
	r := strings.TrimSpace(os.Getenv(EnvAbsResponseRoot))
	if r == "" {
		r = os.TempDir()
	}
	return &Server{AbsResponseRoot: r}
}

// Handler returns an http.Handler with the DSX sim routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/topology/nodes", http.HandlerFunc(s.handleTopologyNodes))
	mux.Handle("GET /v1/topology/vpcs/{vpcID}/nodes", http.HandlerFunc(s.handleTopologyNodes))
	return mux
}

func (s *Server) handleTopologyNodes(w http.ResponseWriter, r *http.Request) {
	if err := requireBearerScheme(r); err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}
	fp, err := filePathFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.serveResponse(w, fp)
}

func (s *Server) serveResponse(w http.ResponseWriter, filePath string) {
	b, err := readResponseBytes(s.AbsResponseRoot, filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
