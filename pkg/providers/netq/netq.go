/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/topology"
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
	CompoundedLinks []CompoundedLinks `json:"compounded_links"`
	Id              string            `json:"id"`
}

type CompoundedLinks struct {
	Id string `json:"id"`
}

type AuthOutput struct {
	AccessToken string `json:"access_token"`
}

func (p *Provider) generateTopologyConfig(ctx context.Context, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	contentType := "Content-Type: application/json"
	accept := "accept: application/json"

	creds := fmt.Sprintf("{\"username\":\"%s\" , \"password\":\"%s\"}", p.cred.user, p.cred.passwd)
	args := []string{p.params.NetqLoginUrl, "-H", accept, "-H", contentType, "-d", creds}
	stdout, err := exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return nil, err
	}

	//get access code from stdout and call API URL
	var authOutput AuthOutput
	outputBytes := stdout.Bytes()
	if len(outputBytes) == 0 {
		return nil, fmt.Errorf("failed to login to Netq server")
	}

	if err := json.Unmarshal(outputBytes, &authOutput); err != nil {
		return nil, fmt.Errorf("failed to parse access token: %v", err)
	}

	addArgs := "{\"filters\": [], \"subgroupNestingDepth\":2}"
	args = []string{p.params.NetqApiUrl, "-H", "authorization: Bearer " + authOutput.AccessToken, "-H", contentType, "-d", addArgs}
	stdout, err = exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return nil, err
	}

	var netqResponse []NetqResponse
	err = json.Unmarshal(stdout.Bytes(), &netqResponse)
	if err != nil {
		return nil, fmt.Errorf("netq output read failed: %v", err)
	}

	nodes := topology.GetNodeList(cis)
	return parseNetq(netqResponse, nodes)
}

func getReqNodeMap(nodes []string) map[string]bool {
	reqNodeMap := make(map[string]bool)
	for _, nodeName := range nodes {
		reqNodeMap[nodeName] = true
	}
	return reqNodeMap
}

func sortVertices(root *topology.Vertex) []string {
	// sort the IDs
	keys := make([]string, 0, len(root.Vertices))
	for key := range root.Vertices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func invalidateExtraNodes(curVertex *topology.Vertex, reqNodeMap map[string]bool, nameMap map[string]string) bool {
	linkExists := false

	if len(curVertex.Vertices) == 0 {
		_, linkExists = reqNodeMap[nameMap[curVertex.ID]]
	} else {
		keys := sortVertices(curVertex)
		for _, key := range keys {
			w := curVertex.Vertices[key]
			childLink := invalidateExtraNodes(w, reqNodeMap, nameMap)
			if !childLink {
				delete(curVertex.Vertices, key)
			}
			linkExists = linkExists || childLink
		}
	}
	return linkExists
}

// parseNetq parses Netq topology output
func parseNetq(netqResponse []NetqResponse, inputNodes []string) (*topology.Vertex, error) {
	nodeMap := make(map[string]*topology.Vertex)    // nodeId : Vertex
	tierMap := make(map[string]int)                 // nodeId : tier
	inverseTierMap := make(map[int]map[string]bool) // tier : nodeId
	nameMap := make(map[string]string)              // nodeId : nodeName
	for _, nodelist := range netqResponse[0].Nodes {
		for _, cnode := range nodelist.Cnode {
			nodeMap[cnode.Id] = &topology.Vertex{
				ID:       cnode.Id,
				Name:     cnode.Name,
				Vertices: make(map[string]*topology.Vertex),
			}
			tierMap[cnode.Id] = cnode.Tier
			nameMap[cnode.Id] = cnode.Name
		}
	}

	highestTier := -1
	for _, link := range netqResponse[0].Links {
		node_id := strings.Split(link.Id, "-*-")

		// ignore mgmt connections on eth0
		if len(link.CompoundedLinks) == 1 {
			nodes := strings.Split(link.CompoundedLinks[0].Id, "-*-")
			src := strings.Split(nodes[0], ":")
			target := strings.Split(nodes[1], ":")
			if src[1] == "eth0" || target[1] == "eth0" {
				continue
			}
		}
		if tierMap[node_id[0]] > tierMap[node_id[1]] {
			treenode := nodeMap[node_id[0]]
			treenode.Vertices[node_id[1]] = nodeMap[node_id[1]]
			if highestTier < tierMap[node_id[0]] {
				highestTier = tierMap[node_id[0]]
			}
			if _, exists := inverseTierMap[tierMap[node_id[0]]]; !exists {
				inverseTierMap[tierMap[node_id[0]]] = make(map[string]bool)
			}
			inverseTierMap[tierMap[node_id[0]]][node_id[0]] = true
		} else {
			treenode := nodeMap[node_id[1]]
			treenode.Vertices[node_id[0]] = nodeMap[node_id[0]]
			if highestTier < tierMap[node_id[1]] {
				highestTier = tierMap[node_id[1]]
			}
			if _, exists := inverseTierMap[tierMap[node_id[1]]]; !exists {
				inverseTierMap[tierMap[node_id[1]]] = make(map[string]bool)
			}
			inverseTierMap[tierMap[node_id[1]]][node_id[1]] = true
		}
	}

	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}

	for nodeId := range inverseTierMap[highestTier] {
		treeRoot.Vertices[nodeId] = nodeMap[nodeId]
	}

	if len(inputNodes) > 0 {
		reqNodeMap := getReqNodeMap(inputNodes)
		invalidateExtraNodes(treeRoot, reqNodeMap, nameMap)
	}

	var graphVertices []*topology.Vertex
	for _, node := range treeRoot.Vertices {
		graphVertices = append(graphVertices, node)
	}

	// Ethernet Spectrum-X may have CLOS network and may require merging of switches to a tree format
	merger := topology.NewMerger(graphVertices)
	merger.Merge()
	top := merger.TopTier()

	treeRoot = &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}

	for _, node := range top {
		treeRoot.Vertices[node.ID] = node
	}

	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	root.Vertices[topology.TopologyTree] = treeRoot

	return root, nil
}
