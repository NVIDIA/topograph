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

package aws

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/utils"
)

const (
	IMDSURL = "http://169.254.169.254/latest/meta-data/"

	tokenTimeDelay = 15 * time.Second
)

type Creds struct {
	Code            string `json:"Code"`
	AccessKeyId     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
}

func getMetadata(path string) ([]byte, error) {
	url := IMDSURL + path

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func GetRegion() (string, error) {
	data, err := getMetadata("placement/region")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func GetCredentials() (*Creds, error) {
	path := "iam/security-credentials"
	data, err := getMetadata(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		path = fmt.Sprintf("%s/%s", path, line)
		break
	}

	// ensure the credentials remain valid for at least the next tokenTimeDelay
	for {
		klog.V(4).Infof("Getting credentials from path %s", path)
		data, err = getMetadata(path)
		if err != nil {
			return nil, err
		}

		creds := &Creds{}
		if err = json.Unmarshal(data, creds); err != nil {
			return nil, err
		}

		klog.V(4).Infof("Credentials expire at %s", creds.Expiration)
		expiration, err := time.Parse(time.RFC3339, creds.Expiration)
		if err != nil {
			klog.Errorf("Error parsing expiration time %q: %v", creds.Expiration, err)
		} else if time.Now().Add(tokenTimeDelay).After(expiration) {
			klog.V(4).Infof("Waiting %s for new token", tokenTimeDelay.String())
			time.Sleep(tokenTimeDelay)
			continue
		}

		if creds.Code != "Success" {
			return nil, fmt.Errorf("failed to get creds: status %s", creds.Code)
		}
		return creds, nil
	}
}

func Instance2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	args := []string{"-w", strings.Join(nodes, ","), fmt.Sprintf("echo $(curl -s %s/instance-id)", IMDSURL)}

	stdout, err := utils.Exec(ctx, "pdsh", args, nil)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("data: %s", stdout.String())

	i2n := map[string]string{}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), ": ")
		if len(arr) == 2 {
			node, instance := arr[0], arr[1]
			i2n[instance] = node
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return i2n, nil
}
