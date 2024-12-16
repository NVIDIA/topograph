package baremetal

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/ib"
	"github.com/NVIDIA/topograph/pkg/topology"
)

// domain contains map of each domainID(clusterUUID) -> list of nodeNames in that domain
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

// Check if domainID exists in the map
func domainIDExists(id string, domainMap map[string]domain) bool {
	if _, exists := domainMap[id]; exists {
		return true
	}

	return false
}

func getIbTree(ctx context.Context, _ []string) (*topology.Vertex, error) {
	nodeVisited := make(map[string]bool)
	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	ibPrefix := "IB"
	ibCount := 0
	partitionNodeMap := make(map[string][]string)
	partitionVisitedMap := make(map[string]bool)

	args := []string{"-h"}
	stdout, err := exec.Exec(ctx, "sinfo", args, nil)
	if err != nil {
		return nil, fmt.Errorf("exec error in sinfo: %v", err)
	}

	// scan each line containing slurm partition and the nodes in it
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		nodeLine := scanner.Text()
		arr := strings.Fields(nodeLine)
		if arr[3] == "0" {
			continue
		}
		partitionName := strings.TrimSpace(arr[0])
		nodeList := strings.TrimSpace(arr[5])
		nodesArr, err := deCompressNodeNames(nodeList)
		if err != nil {
			return nil, fmt.Errorf("deCompressNodeNames failed : %v", err)
		}
		// map of slurm partition name  -> node names
		partitionNodeMap[partitionName] = append(partitionNodeMap[partitionName], nodesArr...)
	}
	for pName, nodes := range partitionNodeMap {
		// for each partition in slurm, find the IB tree it belongs to
		if _, exists := partitionVisitedMap[pName]; !exists {
			for _, node := range nodes {
				if _, exists := nodeVisited[node]; !exists {
					args := []string{"-N", "-R", "ssh", "-w", node, "sudo ibnetdiscover"}
					stdout, err := exec.Exec(ctx, "pdsh", args, nil)
					if err != nil {
						return nil, fmt.Errorf("exec error while pdsh IB command: %v", err)
					}
					if strings.Contains(stdout.String(), "Topology file:") {
						_, hca, _ := ib.ParseIbnetdiscoverFile(stdout.Bytes())
						for _, nodeName := range hca {
							nodeVisited[nodeName] = true
						}
						partitionVisitedMap[pName] = true
						ibRoot, err := ib.GenerateTopologyConfig(stdout.Bytes())
						if err != nil {
							return nil, fmt.Errorf("IB GenerateTopologyConfig failed: %v", err)
						}
						ibCount++
						ibKey := ibPrefix + strconv.Itoa(ibCount)
						treeRoot.Vertices[ibKey] = ibRoot
						break
					} else {
						fmt.Printf("Missing ibnetdiscover output\n")
					}
				} else {
					partitionVisitedMap[pName] = true
				}
			}
		}
	}
	return treeRoot, nil
}

// deCompressNodeNames returns array of node names
func deCompressNodeNames(nodeList string) ([]string, error) {
	nodeArr := []string{}
	// split entries by comma
	// example : nodename-1-[001-004,007,91-99,100],nodename-2-89
	arr := strings.Split(nodeList, ",")
	prefix := ""
	var nodeName string

	// example : nodename-1-[001-004 , 007, 91-99 , 100], nodename-2-89
	for _, entry := range arr {
		// example : nodename-1-[001-004
		if strings.Contains(entry, "[") {
			// example : 100]
			entryWithoutSuffix := strings.TrimSuffix(entry, "]")
			tuple := strings.Split(entryWithoutSuffix, "[")
			prefix = tuple[0]
			// example : nodename-1-[001-004
			if strings.Contains(tuple[1], "-") {
				nr := strings.Split(tuple[1], "-")
				w := len(nr[0])
				start, err := strconv.Atoi(nr[0])
				if err != nil {
					return nil, fmt.Errorf("Atoi err for range start: %v", err)
				}
				end, err := strconv.Atoi(nr[1])
				if err != nil {
					return nil, fmt.Errorf("Atoi err for range end: %v", err)
				}
				for i := start; i <= end; i++ {
					suffixNum := fmt.Sprintf(fmt.Sprintf("%%0%dd", w), i)
					nodeName = prefix + suffixNum
					nodeArr = append(nodeArr, nodeName)
				}
				// avoid another nodename append at the end
				continue
			} else {
				// example : nodename-1-[001
				nv := tuple[1]
				nodeName = prefix + nv
			}
		} else { // no [ means, this could be whole nodename or suffix
			// example: 100], nodename-2-89, 90
			if len(prefix) > 0 { //prefix exists, so must be a suffix.
				if strings.HasSuffix(entry, "]") { //if suffix has ], reset prefix
					nv := strings.Split(entry, "]")
					nodeName = prefix + nv[0]
					prefix = ""
				} else if strings.Contains(entry, "-") { // suffix containing range of nodes
					// example: 100-102]
					nr := strings.Split(entry, "-")
					w := len(nr[0])
					start, err := strconv.Atoi(nr[0])
					if err != nil {
						return nil, fmt.Errorf("Atoi err for range start when prefix is set: %v", err)
					}
					end, err := strconv.Atoi(nr[1])
					if err != nil {
						return nil, fmt.Errorf("Atoi err for range end when prefix is set: %v", err)
					}
					for i := start; i <= end; i++ {
						suffixNum := fmt.Sprintf(fmt.Sprintf("%%0%dd", w), i)
						nodeName = prefix + suffixNum
						nodeArr = append(nodeArr, nodeName)
					}
					// avoid another nodename append at the end
					continue
				} else {
					//example: 90
					nodeName = prefix + entry
				}
			} else { // no prefix yet, must be whole nodename
				//example: nodename-2-89
				nodeName = entry
			}
		}
		nodeArr = append(nodeArr, nodeName)
	}
	return nodeArr, nil
}

// getClusterOutput reads output from nodeInfo and populates the structs
func getClusterOutput(ctx context.Context, domainMap map[string]domain, nodes []string, cmd string) error {
	args := []string{"-R", "ssh", "-w", strings.Join(nodes, ","), cmd}
	stdout, err := exec.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return fmt.Errorf("exec error while pdsh: %v", err)
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
		return fmt.Errorf("scanner error while reading pdsh output: %v", err)
	}

	return nil
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
	domainMap := make(map[string]domain) // domainID: domain
	nodes := getNodeList(cis)
	err := getClusterOutput(ctx, domainMap, nodes, "nvidia-smi -q | grep ClusterUUID")
	if err != nil {
		return nil, fmt.Errorf("getClusterOutput failed: %v", err)
	}
	// get ibnetdiscover output from all unvisited nodes
	treeRoot, err := getIbTree(ctx, nodes)
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v", err)
	}

	return toGraph(domainMap, treeRoot), nil
}
