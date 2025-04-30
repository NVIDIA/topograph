/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package oci

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/exec"
)

const (
	IMDSURL         = "http://169.254.169.254/opc/v2"
	IMDSInstanceURL = IMDSURL + "/instance"
	IMDSTopologyURL = IMDSURL + "/host/rdmaTopologyData"
	IMDSHeader      = `-H "Authorization: Bearer Oracle" -L`
)

type topologyData struct {
	GpuMemoryFabric string `json:"customerGpuMemoryFabric"`
	HPCIslandId     string `json:"customerHPCIslandId"`
	LocalBlock      string `json:"customerLocalBlock"`
	NetworkBlock    string `json:"customerNetworkBlock"`
}

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	args := []string{"-w", strings.Join(cluset.Compact(nodes), ","), fmt.Sprintf("echo $(curl -s  %s %s/id)", IMDSHeader, IMDSInstanceURL)}

	stdout, err := exec.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return nil, err
	}

	return parseInstanceOutput(stdout)
}

func parseInstanceOutput(buff *bytes.Buffer) (map[string]string, error) {
	i2n := map[string]string{}
	scanner := bufio.NewScanner(buff)
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), ": ")
		if len(arr) == 2 {
			node, instance := arr[0], arr[1]
			klog.V(4).Info("Node name: ", node, "Instance ID: ", instance)
			i2n[instance] = node
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return i2n, nil
}

func getHostTopology(ctx context.Context, nodes []string) (map[string]*topologyData, error) {
	args := []string{"-w", strings.Join(cluset.Compact(nodes), ","), fmt.Sprintf("echo $(curl -s  %s %s)", IMDSHeader, IMDSTopologyURL)}

	stdout, err := exec.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return nil, err
	}

	return parseTopologyOutput(stdout)
}

func parseTopologyOutput(buff *bytes.Buffer) (map[string]*topologyData, error) {
	topoMap := map[string]*topologyData{}
	scanner := bufio.NewScanner(buff)
	for scanner.Scan() {
		str := scanner.Text()
		indx := strings.Index(str, ": ")
		if indx != -1 {
			node, data := str[:indx], str[indx+2:]
			klog.V(4).Info("Node name: ", node, " Topology: ", data)
			nodeTopology := &topologyData{}
			err := json.Unmarshal([]byte(data), nodeTopology)
			if err != nil {
				klog.Warningf("Failed to parse topology data for node %s: %v", node, err)
			} else {
				topoMap[node] = nodeTopology
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return topoMap, nil
}

func getRegion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/region", IMDSInstanceURL)
	args := []string{"-s", "-H", "Authorization: Bearer Oracle", "-L", url}

	stdout, err := exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return "", err
	}

	return stdout.String(), nil
}
