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
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/exec"
)

const (
	IMDSURL = "http://169.254.169.254/opc/v2/instance"
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	args := []string{"-w", strings.Join(nodes, ","), fmt.Sprintf("echo $(curl -s  -H \"Authorization: Bearer Oracle\" -L %s/id)", IMDSURL)}

	stdout, err := exec.Exec(ctx, "pdsh", args, nil)
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
	url := fmt.Sprintf("%s/region", IMDSURL)
	args := []string{"-s", "-H", "Authorization: Bearer Oracle", "-L", url}

	stdout, err := exec.Exec(ctx, "curl", args, nil)
	if err != nil {
		return "", err
	}

	return stdout.String(), nil
}
