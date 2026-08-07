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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
)

const pathNodes = "/v1/topology/nodes"

type httpClient struct {
	baseURL string
	token   string
}

// NewHTTPClient returns a Client that calls the DSX Topology API.
// If token is empty the Authorization header is omitted and the Envoy sidecar
// is expected to supply SVID-based authentication transparently.
func NewHTTPClient(baseURL, token string) *httpClient {
	return &httpClient{baseURL: baseURL, token: token}
}

func (c *httpClient) GetTopology(ctx context.Context, vpcID string, nodeIDs []string, pageSize int, pageToken string) (*TopologyResponse, error) {
	path := pathNodes
	if vpcID != "" {
		path = "/v1/topology/vpcs/" + vpcID + "/nodes"
	}

	headers := map[string]string{}
	if c.token != "" {
		headers["Authorization"] = "Bearer " + c.token
	}

	query := map[string]string{}
	if len(nodeIDs) > 0 {
		query["node_ids"] = strings.Join(nodeIDs, ",")
	}
	if pageSize > 0 {
		query["page_size"] = strconv.Itoa(pageSize)
	}
	if pageToken != "" {
		query["page_token"] = pageToken
	}

	f := httpreq.GetRequestFunc(ctx, http.MethodGet, headers, query, nil, c.baseURL, path)
	body, httpErr := httpreq.DoRequestWithRetries(f, false)
	if httpErr != nil {
		return nil, httpErr
	}

	var resp TopologyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return &resp, nil
}
