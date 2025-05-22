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

package gcp

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/providers"
)

const (
	IMDSURL         = "http://metadata.google.internal/computeMetadata/v1"
	IMDSInstanceURL = IMDSURL + "/instance/id"
	IMDSRegionURL   = IMDSURL + "/instance/zone"
	IMDSHeaderKey   = "Metadata-Flavor"
	IMDSHeaderVal   = "Google"
	IMDSHeader      = IMDSHeaderKey + ": " + IMDSHeaderVal
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	stdout, err := exec.Exec(ctx, "pdsh", pdshParams(nodes, IMDSInstanceURL), nil)
	if err != nil {
		return nil, err
	}

	i2n := map[string]string{}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), ": ")
		if len(arr) == 2 {
			node, instance := arr[0], arr[1]
			klog.V(4).Infoln("Node name: ", node, "Instance ID: ", instance)
			i2n[instance] = node
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return i2n, nil
}

func getRegion(ctx context.Context) (string, error) {
	stdout, err := exec.Exec(ctx, "curl", imdsCurlParams(IMDSRegionURL), nil)
	if err != nil {
		return "", err
	}

	// zone format is "projects/<PROJECT ID>/zones/<ZONE NAME>"
	// we need to return zone name only
	zone := stdout.String()
	indx := strings.LastIndex(zone, "/")

	return zone[indx+1:], nil
}

func imdsCurlParams(url string) []string {
	return []string{"-s", "-H", IMDSHeader, url}
}

func pdshParams(nodes []string, url string) []string {
	return []string{"-w", strings.Join(cluset.Compact(nodes), ","), fmt.Sprintf("echo $(curl -s -H %q %s)", IMDSHeader, url)}
}

func convertRegion(region string) string {
	// convert "projects/<project id>/zones/<region>" to "<region>"
	indx := strings.LastIndex(region, "/")
	return region[indx+1:]
}

func GetInstanceAndRegion() (string, string, error) {
	header := map[string]string{IMDSHeaderKey: IMDSHeaderVal}
	instance, err := providers.HttpReq(http.MethodGet, IMDSInstanceURL, header)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute instance-id IMDS request: %v", err)
	}

	region, err := providers.HttpReq(http.MethodGet, IMDSRegionURL, header)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute region IMDS request: %v", err)
	}

	return instance, convertRegion(region), nil
}
