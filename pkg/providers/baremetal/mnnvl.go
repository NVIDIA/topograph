/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package baremetal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/ib"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	cmdClusterID = `nvidia-smi -q | grep "ClusterUUID\|CliqueId" | sort -u`
)

type Cluster struct {
	node     string
	UUID     string
	cliqueID string
}

func (c *Cluster) ID() (string, error) {
	if len(c.UUID) == 0 {
		return "", fmt.Errorf("missing ClusterUUID for node %q", c.node)
	}
	if len(c.cliqueID) == 0 {
		return "", fmt.Errorf("missing CliqueId for node %q", c.node)
	}
	return c.UUID + "." + c.cliqueID, nil
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

func populateDomainsFromPdshOutput(stdout *bytes.Buffer) (topology.DomainMap, error) {
	clusters := make(map[string]*Cluster)
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		nodeLine := scanner.Text()
		arr := strings.Split(nodeLine, ":")
		nodeName := strings.TrimSpace(arr[0])
		val := strings.TrimSpace(arr[2])
		cluster, ok := clusters[nodeName]
		if !ok {
			cluster = &Cluster{node: nodeName}
			clusters[nodeName] = cluster
		}
		switch strings.TrimSpace(arr[1]) {
		case "CliqueId":
			cluster.cliqueID = val
		case "ClusterUUID":
			cluster.UUID = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	domainMap := topology.NewDomainMap()
	for nodeName, cluster := range clusters {
		clusterID, err := cluster.ID()
		if err != nil {
			return nil, err
		}
		domainMap.AddHost(clusterID, nodeName, nodeName)
	}

	return domainMap, nil
}

func toGraph(domainMap topology.DomainMap, treeRoot *topology.Vertex) *topology.Vertex {
	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
		Metadata: make(map[string]string),
	}
	root.Vertices[topology.TopologyTree] = treeRoot
	root.Vertices[topology.TopologyBlock] = domainMap.ToBlocks()

	return root
}

func generateTopologyConfig(ctx context.Context, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	nodes := getNodeList(cis)

	output, err := exec.Pdsh(ctx, cmdClusterID, nodes)
	if err != nil {
		return nil, err
	}

	domainMap, err := populateDomainsFromPdshOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to populate NVL domains: %v", err)
	}
	// get ibnetdiscover output from all unvisited nodes
	treeRoot, err := getIbTree(ctx, nodes, cis)
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v", err)
	}

	return toGraph(domainMap, treeRoot), nil
}
