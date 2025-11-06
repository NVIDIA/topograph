/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	LoginURL    = "auth/v1/login"
	OpIdURL     = "auth/v1/select/opid"
	TopologyURL = "telemetry/v1/object/topologygraph/fetch-topology"
)

type NetqResponse struct {
	Links []Links `json:"links"`
	Nodes []Nodes `json:"nodes"`
}

type Nodes struct {
	Cnode []CNode `json:"compounded_nodes"`
}

type CNode struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Tier int    `json:"tier"`
}

type Links struct {
	Id string `json:"id"`
}

type AuthOutput struct {
	AccessToken string `json:"access_token"`
}

func (p *Provider) generateTopologyConfig(ctx context.Context, cis []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	// 1. login to NetQ server
	payload := strings.NewReader(fmt.Sprintf(`{"username":%q, "password":%q}`, p.cred.user, p.cred.passwd))
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	u, httpErr := getURL(p.params.ApiURL, nil, LoginURL)
	if httpErr != nil {
		return nil, httpErr
	}
	klog.V(4).Infof("Fetching %s", u)
	f := getRequestFunc(ctx, "POST", u, headers, payload)
	resp, data, err := httpreq.DoRequest(f, true)
	if err != nil {
		code := http.StatusInternalServerError
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, httperr.NewError(code, err.Error())
	}

	if len(data) == 0 {
		return nil, httperr.NewError(http.StatusUnauthorized, "failed to login to NetQ server")
	}

	//get access token
	var authOutput AuthOutput
	if err := json.Unmarshal(data, &authOutput); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to parse access token: %v", err))
	}

	// 2. set OpID
	headers = map[string]string{
		"Authorization": "Bearer " + authOutput.AccessToken,
	}
	u, httpErr = getURL(p.params.ApiURL, nil, OpIdURL, p.params.OpID)
	if httpErr != nil {
		return nil, httpErr
	}
	klog.V(4).Infof("Fetching %s", u)
	f = getRequestFunc(ctx, "GET", u, headers, nil)
	resp, data, err = httpreq.DoRequest(f, true)
	if err != nil {
		code := http.StatusInternalServerError
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, httperr.NewError(code, err.Error())
	}

	if len(data) == 0 {
		return nil, httperr.NewError(http.StatusBadGateway, "failed to set NetQ OpID")
	}

	//get access token
	if err := json.Unmarshal(data, &authOutput); err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to parse access token: %v", err))
	}

	// 3. get Topology
	payload = strings.NewReader(`{"filters": [], "subgroupNestingDepth":2}`)
	headers = map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + authOutput.AccessToken,
	}
	query := map[string]string{"timestamp": "0"}
	u, httpErr = getURL(p.params.ApiURL, query, TopologyURL)
	if httpErr != nil {
		return nil, httpErr
	}
	klog.V(4).Infof("Fetching %s", u)
	f = getRequestFunc(ctx, "POST", u, headers, payload)
	resp, data, err = httpreq.DoRequest(f, true)
	if err != nil {
		code := http.StatusInternalServerError
		if resp != nil {
			code = resp.StatusCode
		}
		return nil, httperr.NewError(code, err.Error())
	}

	var netqResponse []NetqResponse
	err = json.Unmarshal(data, &netqResponse)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("netq output read failed: %v", err))
	}

	return parseNetq(netqResponse, topology.GetNodeNameMap(cis))
}

func getRequestFunc(ctx context.Context, method, url string, headers map[string]string, payload io.Reader) httpreq.RequestFunc {
	return func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, url, payload)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %v", err)
		}
		for key, val := range headers {
			req.Header.Add(key, val)
		}
		return req, nil
	}
}

func getURL(baseURL string, query map[string]string, paths ...string) (string, *httperr.Error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", httperr.NewError(http.StatusBadRequest, err.Error())
	}

	u.Path = path.Join(append([]string{u.Path}, paths...)...)

	if len(query) != 0 {
		q := u.Query()
		for key, val := range query {
			q.Set(key, val)
		}
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

// parseNetq parses Netq topology output
func parseNetq(resp []NetqResponse, inputNodes map[string]bool) (*topology.Vertex, *httperr.Error) {
	if len(resp) != 1 {
		return nil, httperr.NewError(http.StatusBadGateway, "invalid NetQ response: multiple entries")
	}

	layer := make(map[string]*topology.Vertex)   // current layer starting from leaves (nodeId : Vertex)
	nodeMap := make(map[string]*topology.Vertex) // nodeId : Vertex
	tierMap := make(map[string]int)              // nodeId : tier
	nameMap := make(map[string]string)           // nodeId : nodeName

	// split nodes between leaves and switches
	for _, nodelist := range resp[0].Nodes {
		for _, cnode := range nodelist.Cnode {
			v := &topology.Vertex{
				ID:   cnode.Id,
				Name: cnode.Name,
			}
			if cnode.Tier == -1 { // leaf
				if inputNodes[cnode.Name] {
					layer[cnode.Id] = v
				}
			} else { // switch
				nodeMap[cnode.Id] = v
			}
			tierMap[cnode.Id] = cnode.Tier
			nameMap[cnode.Id] = cnode.Name
		}
	}

	// create map of link IDs [lower node : upper nodes]
	linksUp := make(map[string]map[string]bool)
	for _, link := range resp[0].Links {
		nodeIDs := strings.Split(link.Id, "-*-")
		if len(nodeIDs) != 2 {
			klog.Warningf("invalid link ID %q", link.Id)
			continue
		}
		nodeLow, nodeHigh := nodeIDs[0], nodeIDs[1]

		if tierMap[nodeLow] == tierMap[nodeHigh] {
			klog.Warningf("invalid link ID %q: nodes belong to the same tier %d", link.Id, tierMap[nodeLow])
			continue
		}

		if tierMap[nodeLow] > tierMap[nodeHigh] {
			nodeLow, nodeHigh = nodeHigh, nodeLow
		}

		up, ok := linksUp[nodeLow]
		if !ok {
			up = make(map[string]bool)
			linksUp[nodeLow] = up
		}
		up[nodeHigh] = true
	}

	for {
		count := len(nodeMap)
		nextLayer := make(map[string]*topology.Vertex)
		for id, w := range layer {
			for up := range linksUp[id] {
				v, ok := nextLayer[up]
				if !ok {
					v = nodeMap[up]
					v.Vertices = make(map[string]*topology.Vertex)
					nextLayer[up] = v
					delete(nodeMap, up)
				}

				if v != nil {
					v.Vertices[id] = w
				} else {
					klog.Warningf("node ID %q not found", up)
				}
			}
		}

		if count == len(nodeMap) {
			break
		}
		layer = nextLayer
	}

	var top []*topology.Vertex
	for _, node := range layer {
		top = append(top, node)
	}

	// Ethernet Spectrum-X may have CLOS network and may require merging of switches to a tree format
	merger := topology.NewMerger(top)
	merger.Merge()
	top = merger.TopTier()

	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}

	for _, node := range top {
		treeRoot.Vertices[node.ID] = node
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyTree: treeRoot},
	}

	return root, nil
}
