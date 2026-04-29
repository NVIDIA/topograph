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
)

// Server serves DSX topology simulation APIs by returning raw JSON from a filesystem path
// (when [QueryParamFilePath] is absolute) or from embedded `responses/<stem>.json`.
// The path segment `vpcs/{vpcID}` and any Authorization claims are ignored for file selection.
type Server struct {
	embed fs.FS
}

// NewServer returns a server; pass [EmbeddedResponsesFS] for non-absolute filePath resolution.
// embedFS may be nil to disable embedded responses (tests).
func NewServer(embedFS fs.FS) *Server {
	return &Server{embed: embedFS}
}

// Handler returns an http.Handler with the DSX sim routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/topology/nodes", http.HandlerFunc(s.handleNodesBearer))
	mux.Handle("GET /v1/topology/vpcs/{vpcID}/nodes", http.HandlerFunc(s.handleNodesVPC))
	return mux
}

func (s *Server) handleNodesVPC(w http.ResponseWriter, r *http.Request) {
	fp, err := filePathFromRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.serveResponse(w, fp)
}

func (s *Server) handleNodesBearer(w http.ResponseWriter, r *http.Request) {
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
	b, err := readResponseBytes(s.embed, filePath)
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
