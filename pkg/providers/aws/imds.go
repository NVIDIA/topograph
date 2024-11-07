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
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/utils"
)

const (
	IMDS           = "http://169.254.169.254"
	IMDS_TOKEN_URL = IMDS + "/latest/api/token"
	IMDS_URL       = IMDS + "/latest/meta-data"

	tokenTimeDelay = 15 * time.Second
)

type Creds struct {
	Code            string `json:"Code"`
	AccessKeyId     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
	Expiration      string `json:"Expiration"`
}

func getToken() (string, error) {
	var f utils.HttpRequestFunc = (func() (*http.Request, error) {
		req, err := http.NewRequest("PUT", IMDS_TOKEN_URL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %v", err)
		}
		req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", "21600")
		return req, nil
	})

	_, data, err := utils.HttpRequest(f)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request: %v", err)
	}

	return string(data), nil
}

func addToken(req *http.Request) error {
	token, err := getToken()
	if err != nil {
		return err
	}

	if len(token) != 0 {
		req.Header.Add("X-aws-ec2-metadata-token", token)
	}

	return nil
}

func getMetadata(path string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", IMDS_URL, path)
	klog.V(4).Infof("Requesting URL %s", url)

	var f utils.HttpRequestFunc = func() (*http.Request, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %v", err)
		}
		err = addToken(req)
		if err != nil {
			return nil, err
		}
		return req, nil
	}

	resp, data, err := utils.HttpRequest(f)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status: %s", resp.Status)
	}

	return data, nil
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
		klog.V(4).Infof("Getting credentials from %s", path)
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
	args := []string{"-w", strings.Join(nodes, ","),
		fmt.Sprintf("TOKEN=$(curl -s -X PUT -H \"X-aws-ec2-metadata-token-ttl-seconds: 21600\" %s); echo $(curl -s -H \"X-aws-ec2-metadata-token: $TOKEN\" %s/instance-id)", IMDS_TOKEN_URL, IMDS_URL)}

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
