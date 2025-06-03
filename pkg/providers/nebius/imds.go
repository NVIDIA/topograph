/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/providers"
)

const (
	IMDSPath         = "/mnt/cloud-metadata"
	IMDSInstancePath = IMDSPath + "/instance-id"
	IMDSRegionPath   = IMDSPath + "/region-name"
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	stdout, err := exec.Pdsh(ctx, fmt.Sprintf("echo $(cat %s)", IMDSInstancePath), nodes)
	if err != nil {
		return nil, err
	}

	return providers.ParseInstanceOutput(stdout)
}

func getRegion(ctx context.Context) (string, error) {
	stdout, err := exec.Exec(ctx, "cat", []string{IMDSRegionPath}, nil)
	if err != nil {
		return "", err
	}

	return stdout.String(), nil
}
