/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type Cluster struct {
	node     string
	UUID     string
	cliqueID string
}

type IBNetDiscoverBM struct{}

func (c *Cluster) ID() (string, error) {
	if len(c.UUID) == 0 {
		return "", fmt.Errorf("missing ClusterUUID for node %q", c.node)
	}
	if len(c.cliqueID) == 0 {
		return "", fmt.Errorf("missing CliqueId for node %q", c.node)
	}
	return c.UUID + "." + c.cliqueID, nil
}

func (h *IBNetDiscoverBM) Run(ctx context.Context, node string) (*bytes.Buffer, error) {
	return exec.Pdsh(ctx, "sudo ibnetdiscover", []string{node}, "-N")
}

func populateDomainsFromPdshOutput(stdout *bytes.Buffer) (topology.DomainMap, error) {
	clusters := make(map[string]*Cluster)
	invalid := make(map[string]bool)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		nodeLine := scanner.Text()
		arr := strings.Split(nodeLine, ":")
		nodeName := strings.TrimSpace(arr[0])
		idName := strings.TrimSpace(arr[1])
		val := strings.TrimSpace(arr[2])
		cluster, ok := clusters[nodeName]
		if !ok {
			cluster = &Cluster{node: nodeName}
			clusters[nodeName] = cluster
		}
		switch idName {
		case "CliqueId":
			setID(nodeName, idName, &cluster.cliqueID, val, invalid)
		case "ClusterUUID":
			setID(nodeName, idName, &cluster.UUID, val, invalid)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// delete invalid nodes
	for nodeName := range invalid {
		delete(clusters, nodeName)
	}

	domainMap := topology.NewDomainMap()
	for nodeName, cluster := range clusters {
		clusterID, err := cluster.ID()
		if err != nil {
			return nil, err
		}
		domainMap.AddHost(clusterID, nodeName, nodeName)
	}

	klog.V(4).Info(domainMap.String())

	return domainMap, nil
}

func setID(nodename, idname string, id *string, val string, invalid map[string]bool) {
	if len(*id) == 0 {
		*id = val
	} else {
		klog.Warningf("Ambiguous %s %q, %q for node %q", idname, *id, val, nodename)
		invalid[nodename] = true
	}
}
