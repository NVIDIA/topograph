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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// httpStatusError classifies non-2xx API responses for [Provider] to map to HTTP client errors.
type httpStatusError struct {
	code int
	msg  string
}

func (e *httpStatusError) Error() string { return e.msg }

const defaultHTTPTimeout = 120 * time.Second

// apiClient implements [Client] using HTTPS GET DSX API endpoints.
type apiClient struct {
	http   *http.Client
	params *Params
}

func newAPIClient(p *Params) *apiClient {
	return &apiClient{
		http:   newHTTPTransportClient(p.InsecureSkipTLS),
		params: p,
	}
}

func newHTTPTransportClient(insecureSkipTLS bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec // InsecureSkipVerify only when params explicitly request it (lab/dev).
		InsecureSkipVerify: insecureSkipTLS,
	}
	return &http.Client{Transport: tr, Timeout: defaultHTTPTimeout}
}

func (c *apiClient) GetTopology(ctx context.Context, vpcID string, nodeIDs []string, pageSize int, pageToken string) (*TopologyResponse, error) {
	v := vpcID
	if v == "" {
		v = c.params.VpcID
	}
	u, err := topologyRequestURL(c.params.BaseURL, v)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(pageSize))
	}
	for _, id := range nodeIDs {
		q.Add("node_ids", id)
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.params.BearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{
			code: resp.StatusCode,
			msg:  fmt.Sprintf("dsx: api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	var out TopologyResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("dsx: decode topology: %w", err)
	}
	if len(out.Switches) == 0 {
		return nil, fmt.Errorf("dsx: empty switches in response")
	}
	return &out, nil
}

// topologyRequestURL builds GET …/topology/…/nodes. Query parameters on baseURL are preserved
// so callers can append sim-only keys (e.g. filePath= for pkg/providers/dsx/sim).
func topologyRequestURL(baseURL, vpcID string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("dsx: invalid base_url: %w", err)
	}
	path := "/v1/topology/nodes"
	if vpcID != "" {
		path = "/v1/topology/vpcs/" + url.PathEscape(vpcID) + "/nodes"
	}
	out := &url.URL{
		Scheme:   base.Scheme,
		Opaque:   base.Opaque,
		User:     base.User,
		Host:     base.Host,
		Path:     path,
		RawQuery: base.RawQuery,
		Fragment: base.Fragment,
	}
	return out, nil
}

// Simulation API fault injection for [LoaderSim] (mapstructure "api_error").
const (
	simAPIErrNone = iota
	simAPIErrClientFactory
	simAPIErrGetTopology
)

func stemFromModelFileName(modelFileName string) string {
	base := filepath.Base(modelFileName)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
