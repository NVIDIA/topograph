package baremetal

import (
	"bufio"
	"context"
	"fmt"
	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/utils"
	"strconv"
	"strings"
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
func toSlurm(domainMap map[string]domain) *common.Vertex {
	root := &common.Vertex{
		Vertices: make(map[string]*common.Vertex),
		Metadata: make(map[string]string),
	}
	blockSize := -1
	for domainName, domain := range domainMap {
		tree := &common.Vertex{
			ID:       domainName,
			Vertices: make(map[string]*common.Vertex),
		}
		for node, _ := range domain.nodeMap {
			tree.Vertices[node] = &common.Vertex{Name: node, ID: node}
			if blockSize == -1 {
				blockSize = len(domain.nodeMap)
			} else {
				fmt.Printf("blockSize different between NVL domains")
			}
		}
		root.Vertices[domainName] = tree
	}
	// add root metadata
	root.Metadata["engine"] = "slurm"
	root.Metadata["plugin"] = "topology/block"
	root.Metadata["blocksize"] = strconv.Itoa(blockSize)
	return root
}

func generateTopologyConfig(ctx context.Context, cis []common.ComputeInstances) (*common.Vertex, error) {
	domainMap := make(map[string]domain) // domainID: domain
	nodes := getNodeList(cis)
	err := getClusterOutput(ctx, domainMap, nodes, "nvidia-smi -q | grep ClusterUUID")
	if err != nil {
		return nil, fmt.Errorf("getClusterOutput failed: %v\n", err)
	}
	return toSlurm(domainMap), nil
}
