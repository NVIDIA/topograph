package baremetal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Id       string `json:"id"`
	Name     string `json:"name"`
	NodeType string `json:"node_type"`
	Tier     int    `json:"tier"`
}

type Links struct {
	Id string `json:"id"`
}

// parseNetq parses Netq topology output
func parseNetq() (*topology.Vertex, error) {
	filePath, _ := filepath.Abs("pkg/providers/baremetal/air-topology.json")
	plan, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("netq output file failed: %v", err)
	}
	var netqResponse []NetqResponse
	err = json.Unmarshal(plan, &netqResponse)
	if err != nil {
		return nil, fmt.Errorf("netq output read failed: %v", err)
	}

	nodeMap := make(map[string]*topology.Vertex)
	tierMap := make(map[string]int)
	inverseTierMap := make(map[int]map[string]bool)
	nameMap := make(map[string]string)
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

	for nodeId, _ := range inverseTierMap[highestTier] {
		treeRoot.Vertices[nodeId] = nodeMap[nodeId]
	}

	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	root.Vertices[topology.TopologyTree] = treeRoot

	return root, nil
}
