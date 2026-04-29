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
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(EmbeddedResponsesFS())
}

func TestVPCPathIgnoredUsesFilePath(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/vpcs/ignored-vpc/nodes?" + QueryParamFilePath + "=" + url.QueryEscape("small-tree.json")
	res, err := ts.Client().Get(u)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)

	var m map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
	require.Contains(t, m, "switches")
}

func TestBearerUsesFilePathIgnoresToken(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/nodes?" + QueryParamFilePath + "=" + url.QueryEscape("small-tree.json")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer opaque-token-not-used")

	res, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestBearerJWTIgnoredUsesFilePath(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	pl, err := json.Marshal(map[string]string{"vpc_id": "wrong"})
	require.NoError(t, err)
	tok := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(pl) + ".x"

	u := ts.URL + "/v1/topology/nodes?" + QueryParamFilePath + "=" + url.QueryEscape("medium.json")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)

	res, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"sw3"`)
}

func TestBearerMissingAuth(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/nodes?" + QueryParamFilePath + "=small-tree.json"
	res, err := ts.Client().Get(u)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestMissingFilePathVPCRoute(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	res, err := ts.Client().Get(ts.URL + "/v1/topology/vpcs/x/nodes")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestAbsoluteFilePathReadsDiskOnly(t *testing.T) {
	dir := t.TempDir()
	custom := `{"disk":true}` + "\n"
	p := filepath.Join(dir, "custom.json")
	require.NoError(t, os.WriteFile(p, []byte(custom), 0o644))
	abs, err := filepath.Abs(p)
	require.NoError(t, err)

	srv := NewServer(EmbeddedResponsesFS())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/vpcs/x/nodes?" + QueryParamFilePath + "=" + url.QueryEscape(abs)
	res, err := ts.Client().Get(u)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(custom), strings.TrimSpace(string(body)))
}

func TestEmbedWhenRelativePathUsesBasename(t *testing.T) {
	ts := httptest.NewServer(testServer(t).Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/vpcs/x/nodes?" + QueryParamFilePath + "=" + url.QueryEscape("does/not/exist/small-tree.json")
	res, err := ts.Client().Get(u)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
	require.Contains(t, m, "switches")
}

func TestAbsoluteMissingNoEmbedFallback(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	abs, err := filepath.Abs(missing)
	require.NoError(t, err)

	srv := NewServer(EmbeddedResponsesFS())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	u := ts.URL + "/v1/topology/vpcs/x/nodes?" + QueryParamFilePath + "=" + url.QueryEscape(abs)
	res, err := ts.Client().Get(u)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}
