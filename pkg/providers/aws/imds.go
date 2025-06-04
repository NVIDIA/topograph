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
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/providers"
)

const (
	IMDSURL            = "http://169.254.169.254/latest"
	IMDSTokenURL       = IMDSURL + "/api/token"
	IMDSInstanceURL    = IMDSURL + "/meta-data/instance-id"
	IMDSRegionURL      = IMDSURL + "/meta-data/placement/region"
	IMDSTokenHeaderKey = "X-aws-ec2-metadata-token-ttl-seconds"
	IMDSTokenHeaderVal = "60"
	IMDSTokenHeader    = IMDSTokenHeaderKey + ": " + IMDSTokenHeaderVal
	IMDSHeaderKey      = "X-aws-ec2-metadata-token"

	tokenTimeDelay = 15 * time.Second
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	stdout, err := exec.Pdsh(ctx, pdshCmd(IMDSInstanceURL), nodes)
	if err != nil {
		return nil, err
	}

	return providers.ParseInstanceOutput(stdout)
}

func makeHeader(name, val string) string {
	return fmt.Sprintf("%s: %s", name, val)
}

func pdshCmd(url string) string {
	return fmt.Sprintf("TOKEN=$(curl -s -X PUT -H %q %s); echo $(curl -s -H %q %s)",
		IMDSTokenHeader, IMDSTokenURL, makeHeader(IMDSHeaderKey, "$TOKEN"), url)
}

func GetInstanceAndRegion() (string, string, error) {
	header := map[string]string{IMDSTokenHeaderKey: IMDSTokenHeaderVal}
	token, err := providers.HttpReq(http.MethodPut, IMDSTokenURL, header)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute token request: %v", err)
	}

	header = map[string]string{IMDSHeaderKey: token}
	instance, err := providers.HttpReq(http.MethodGet, IMDSInstanceURL, header)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute instance-id IMDS request: %v", err)
	}

	region, err := providers.HttpReq(http.MethodGet, IMDSRegionURL, header)
	if err != nil {
		return "", "", fmt.Errorf("failed to execute region IMDS request: %v", err)
	}

	return instance, region, nil
}
