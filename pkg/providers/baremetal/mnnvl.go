package baremetal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/ib"
	"github.com/NVIDIA/topograph/pkg/topology"
	"k8s.io/klog/v2"
)

// domain contains map of each domainID(clusterUUID) -> map of nodeNames in that domain
// Each domain will be a separate NVL Domain
type domain struct {
	nodeMap map[string]bool // nodeName: true
}

// getNodeList retrieves all the nodenames on the cluster
func getNodeList(cis []topology.ComputeInstances) []string {
	nodes := []string{}
	for _, ci := range cis {
		for _, node := range ci.Instances {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

func getIbTree(ctx context.Context, nodeList []string, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	nodeVisited := make(map[string]bool)
	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	ibPrefix := "IB"
	ibCount := 0

	for _, node := range nodeList {
		if _, exists := nodeVisited[node]; !exists {
			args := []string{"-N", "-R", "ssh", "-w", node, "sudo ibnetdiscover"}
			stdout, err := exec.Exec(ctx, "pdsh", args, nil)
			if err != nil {
				return nil, fmt.Errorf("exec error while pdsh IB command: %v", err)
			}
			if strings.Contains(stdout.String(), "Topology file:") {
				// mark the visited nodes
				_, hca, _ := ib.ParseIbnetdiscoverFile(stdout.Bytes())
				for _, nodeName := range hca {
					nodeVisited[nodeName] = true
				}
				ibRoot, err := ib.GenerateTopologyConfig(stdout.Bytes(), cis)
				if err != nil {
					return nil, fmt.Errorf("IB GenerateTopologyConfig failed: %v", err)
				}
				ibCount++
				ibKey := ibPrefix + strconv.Itoa(ibCount)
				treeRoot.Vertices[ibKey] = ibRoot
			} else {
				klog.V(2).Infof("Missing ibnetdiscover output\n")
			}
		}
	}
	return treeRoot, nil
}

func populateDomains(stdout *bytes.Buffer) (map[string]domain, error) {
	domainMap := make(map[string]domain) // domainID: domain
	scanner := bufio.NewScanner(stdout)
	cliqueId := ""
	clusterUUID := ""
	domainName := ""
	for scanner.Scan() {
		nodeLine := scanner.Text()
		arr := strings.Split(nodeLine, ":")
		nodeName := strings.TrimSpace(arr[0])
		itemName := strings.TrimSpace(arr[1])
		if itemName == "CliqueId" {
			cliqueId = strings.TrimSpace(arr[2])
			continue
		}
		clusterUUID = strings.TrimSpace(arr[2])
		domainName = clusterUUID + cliqueId
		if _, exists := domainMap[domainName]; !exists {
			domainMap[domainName] = domain{
				nodeMap: make(map[string]bool),
			}
		}
		nodeMap := domainMap[domainName].nodeMap
		nodeMap[nodeName] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error while reading pdsh output: %v", err)
	}
	return domainMap, nil
}

// getClusterOutput reads output from nodeInfo and populates the structs
func getClusterOutput(ctx context.Context, nodes []string, cmd string) (map[string]domain, error) {
	args := []string{"-R", "ssh", "-w", strings.Join(nodes, ","), cmd}
	stdout, err := exec.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return nil, fmt.Errorf("exec error while pdsh: %v", err)
	}
	return populateDomains(stdout)
}

func toGraph(domainMap map[string]domain, treeRoot *topology.Vertex) *topology.Vertex {
	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}
	blockRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	root.Vertices[topology.TopologyTree] = treeRoot
	for domainName, domain := range domainMap {
		tree := &topology.Vertex{
			ID:       domainName,
			Vertices: make(map[string]*topology.Vertex),
		}
		for node := range domain.nodeMap {
			tree.Vertices[node] = &topology.Vertex{Name: node, ID: node}
		}
		blockRoot.Vertices[domainName] = tree
	}
	root.Vertices[topology.TopologyBlock] = blockRoot
	return root
}

func generateTopologyConfig(ctx context.Context, cis []topology.ComputeInstances) (*topology.Vertex, error) {

	nodes := getNodeList(cis)
	domainMap, err := getClusterOutput(ctx, nodes, `nvidia-smi -q | grep "ClusterUUID\|CliqueId"`)
	if err != nil {
		return nil, fmt.Errorf("getClusterOutput failed: %v", err)
	}
	// get ibnetdiscover output from all unvisited nodes
	treeRoot, err := getIbTree(ctx, nodes, cis)
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v", err)
	}

	return toGraph(domainMap, treeRoot), nil
}
