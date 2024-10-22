package baremetal

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/ib"
	"github.com/NVIDIA/topograph/pkg/utils"
)

// domain contains map of each domainID(clusterUUID) -> list of nodeNames in that domain
// Each domain will be a separate NVL Domain
type domain struct {
	nodeMap map[string]bool // nodeName: true
}

// getNodeList retrieves all the nodenames on the cluster
func getNodeList(cis []common.ComputeInstances) []string {
	nodes := []string{}
	for _, ci := range cis {
		for _, node := range ci.Instances {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// Check if domainID exists in the map
func domainIDExists(id string, domainMap map[string]domain) bool {
	if _, exists := domainMap[id]; exists {
		return true
	}
	return false
}

func getIbTree(ctx context.Context, nodes []string) (*common.Vertex, error) {
	nodeVisited := make(map[string]bool)
	treeRoot := &common.Vertex{
		Vertices: make(map[string]*common.Vertex),
	}
	ibPrefix := "IB"
	ibCount := 0
	for _, node := range nodes {
		if _, exists := nodeVisited[node]; !exists {
			args := []string{"-N", "-R", "ssh", "-w", node, "sudo ibnetdiscover"}
			stdout, err := utils.Exec(ctx, "pdsh", args, nil)
			if err != nil {
				return nil, fmt.Errorf("Exec error while pdsh IB command\n")
			}
			if strings.Contains(stdout.String(), "Topology file:") {
				_, hca, _ := ib.ParseIbnetdiscoverFile(stdout.Bytes())
				for _, nodeName := range hca {
					nodeVisited[nodeName] = true
				}
				ibRoot, err := ib.GenerateTopologyConfig(stdout.Bytes())
				if err != nil {
					return nil, fmt.Errorf("IB GenerateTopologyConfig failed: %v\n", err)
				}
				ibCount++
				ibKey := ibPrefix + strconv.Itoa(ibCount)
				treeRoot.Vertices[ibKey] = ibRoot
			}
		}
	}
	return treeRoot, nil
}

// getClusterOutput reads output from nodeInfo and populates the structs
func getClusterOutput(ctx context.Context, domainMap map[string]domain, nodes []string, cmd string) error {
	args := []string{"-R", "ssh", "-w", strings.Join(nodes, ","), cmd}
	stdout, err := utils.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return fmt.Errorf("Exec error while pdsh\n")
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		nodeLine := scanner.Text()
		arr := strings.Split(nodeLine, ":")
		nodeName := arr[0]
		clusterUUID := strings.TrimSpace(arr[2])
		if !domainIDExists(clusterUUID, domainMap) {
			domainMap[clusterUUID] = domain{
				nodeMap: make(map[string]bool),
			}
		}
		nodeMap := domainMap[clusterUUID].nodeMap
		nodeMap[nodeName] = true
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Scanner error while reading pdsh output\n")
	}
	return nil
}
func toGraph(domainMap map[string]domain, treeRoot *common.Vertex) *common.Vertex {
	root := &common.Vertex{
		Vertices: make(map[string]*common.Vertex),
		Metadata: make(map[string]string),
	}
	blockRoot := &common.Vertex{
		Vertices: make(map[string]*common.Vertex),
	}
	root.Vertices[common.ValTopologyTree] = treeRoot
	for domainName, domain := range domainMap {
		tree := &common.Vertex{
			ID:       domainName,
			Vertices: make(map[string]*common.Vertex),
		}
		for node := range domain.nodeMap {
			tree.Vertices[node] = &common.Vertex{Name: node, ID: node}
		}
		blockRoot.Vertices[domainName] = tree
	}
	// add root metadata
	root.Metadata[common.KeyEngine] = common.EngineSLURM
	root.Metadata[common.KeyPlugin] = common.ValTopologyBlock
	root.Vertices[common.ValTopologyBlock] = blockRoot
	return root
}

func generateTopologyConfig(ctx context.Context, cis []common.ComputeInstances) (*common.Vertex, error) {
	domainMap := make(map[string]domain) // domainID: domain
	nodes := getNodeList(cis)
	err := getClusterOutput(ctx, domainMap, nodes, "nvidia-smi -q | grep ClusterUUID")
	if err != nil {
		return nil, fmt.Errorf("getClusterOutput failed: %v\n", err)
	}
	// get ibnetdiscover output from all unvisited nodes
	treeRoot, err := getIbTree(ctx, nodes)
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v\n", err)
	}
	return toGraph(domainMap, treeRoot), nil
}
