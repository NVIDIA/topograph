package baremetal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NETQLOGINURL = "https://api.air.netq.nvidia.com/netq/auth/v1/login"
	NETQAPIURL   = "https://air.netq.nvidia.com/api/netq/telemetry/v1/object/topologygraph/fetch-topology?timestamp=0"
)

type NetqResponse struct {
	Links []Links `json:"links"`
	Nodes []Nodes `json:"nodes"`
}

type Nodes struct {
	Cnode []CNode `json:"compounded_nodes"`
}

type CNode struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	NodeType string `json:"node_type"`
	Tier     int    `json:"tier"`
}

type Links struct {
	Id string `json:"id"`
}

type AuthOutput struct {
	AccessToken string `json:"access_token"`
}

func generateTopologyConfigForEth(ctx context.Context, cred Credentials) (*topology.Vertex, error) {
	contentType := "Content-Type: application/json"
	accept := "accept: application/json"

	creds := fmt.Sprintf("{\"username\":\"%s\" , \"password\":\"%s\"}", cred.Uname, cred.Pwd)
	args := []string{NETQLOGINURL, "-H", accept, "-H", contentType, "-d", creds}
	stdout, err := exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return nil, err
	}

	//get access code from stdout and call API URL
	var authOutput AuthOutput
	outputBytes := stdout.Bytes()
	if len(outputBytes) == 0 {
		return nil, fmt.Errorf("failed to login to Netq server\n")
	}

	if err := json.Unmarshal(outputBytes, &authOutput); err != nil {
		return nil, fmt.Errorf("failed to parse access token: %v\n", err)
	}

	addArgs := "{\"filters\": [], \"subgroupNestingDepth\":2}"
	args = []string{NETQAPIURL, "-H", "authorization: Bearer " + authOutput.AccessToken, "-H", contentType, "-d", addArgs}
	stdout, err = exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return nil, err
	}

	var netqResponse []NetqResponse
	err = json.Unmarshal(stdout.Bytes(), &netqResponse)
	if err != nil {
		return nil, fmt.Errorf("netq output read failed: %v", err)
	}
	return parseNetq(netqResponse)
}

// parseNetq parses Netq topology output
func parseNetq(netqResponse []NetqResponse) (*topology.Vertex, error) {
	nodeMap := make(map[string]*topology.Vertex)    // nodeId : Vertex
	tierMap := make(map[string]int)                 // nodeId : tier
	inverseTierMap := make(map[int]map[string]bool) // tier : nodename
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
		ID:       "root",
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}

	for nodeId := range inverseTierMap[highestTier] {
		treeRoot.Vertices[nodeId] = nodeMap[nodeId]
	}

	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	root.Vertices[topology.TopologyTree] = treeRoot

	return root, nil
}
