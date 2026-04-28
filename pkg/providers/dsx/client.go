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

	"github.com/NVIDIA/topograph/pkg/topology"
)

// httpStatusError classifies non-2xx API responses for [Provider] to map to HTTP client errors.
type httpStatusError struct {
	code int
	msg  string
}

func (e *httpStatusError) Error() string { return e.msg }

const defaultHTTPTimeout = 120 * time.Second
const defaultRegion = "default"
const maxTopologyPages = 10000

// ctxKeyDSXPageSize carries the effective page size for one [Provider.generateInstanceTopology] call
// (request override merged with config). See [contextWithDSXPageSize].
type ctxKeyDSXPageSize struct{}

func contextWithDSXPageSize(ctx context.Context, pageSize int) context.Context {
	return context.WithValue(ctx, ctxKeyDSXPageSize{}, pageSize)
}

func effectivePageSize(ctx context.Context, p *Params) int {
	if v := ctx.Value(ctxKeyDSXPageSize{}); v != nil {
		if n, ok := v.(int); ok && n > 0 {
			return n
		}
	}
	if p.PageSize > 0 {
		return p.PageSize
	}
	return 1000
}

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
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		tr = &http.Transport{}
	}
	tr = tr.Clone()
	tr.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec // InsecureSkipVerify only when params explicitly request it (lab/dev).
		InsecureSkipVerify: insecureSkipTLS,
	}
	return &http.Client{Transport: tr, Timeout: defaultHTTPTimeout}
}

func (c *apiClient) GetTopology(ctx context.Context, vpcID string, nodeIDs []string, cis []topology.ComputeInstances) (*TopologyResponse, []topology.ComputeInstances, error) {
	pageSize := effectivePageSize(ctx, c.params)
	var merged *TopologyResponse
	token := ""
	for i := 0; ; i++ {
		if i >= maxTopologyPages {
			return nil, nil, fmt.Errorf("dsx: topology pagination exceeded %d pages", maxTopologyPages)
		}
		page, err := c.getTopologyPage(ctx, vpcID, nodeIDs, pageSize, token)
		if err != nil {
			return nil, nil, err
		}
		if merged == nil {
			merged = page
		} else {
			mergeTopologyResponses(merged, page)
		}
		token = strings.TrimSpace(page.NextPageToken)
		if token == "" {
			break
		}
	}
	if merged == nil || len(merged.Switches) == 0 {
		return nil, nil, fmt.Errorf("dsx: empty switches in response")
	}
	merged.NextPageToken = ""
	return merged, effectiveComputeInstances(merged, cis), nil
}

func (c *apiClient) getTopologyPage(ctx context.Context, vpcID string, nodeIDs []string, pageSize int, pageToken string) (*TopologyResponse, error) {
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
	return &out, nil
}

func mergeTopologyResponses(dst *TopologyResponse, src *TopologyResponse) {
	if src == nil || len(src.Switches) == 0 {
		return
	}
	if dst.Switches == nil {
		dst.Switches = make(map[string]SwitchInfo)
	}
	for name, swSrc := range src.Switches {
		swDst := dst.Switches[name]
		swDst.Switches = unionStringSlices(swDst.Switches, swSrc.Switches)
		swDst.Nodes = mergeNodeInfos(swDst.Nodes, swSrc.Nodes)
		dst.Switches[name] = swDst
	}
}

func unionStringSlices(a, b []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func mergeNodeInfos(a, b []NodeInfo) []NodeInfo {
	seen := make(map[string]struct{})
	var out []NodeInfo
	for _, n := range append(append([]NodeInfo{}, a...), b...) {
		if _, ok := seen[n.NodeID]; ok {
			continue
		}
		seen[n.NodeID] = struct{}{}
		out = append(out, n)
	}
	return out
}

// topologyRequestURL builds GET …/topology/…/nodes. Query parameters on baseURL are preserved
// so callers can append sim-only keys (e.g. filePath= for pkg/providers/dsx/sim).
func topologyRequestURL(baseURL, vpcID string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("dsx: invalid base_url: %w", err)
	}
	apiPath := "/v1/topology/nodes"
	if vpcID != "" {
		apiPath = "/v1/topology/vpcs/" + url.PathEscape(vpcID) + "/nodes"
	}
	path := apiPath
	if p := strings.TrimSuffix(base.Path, "/"); p != "" {
		path = p + apiPath
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
