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
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	IMDSPath         = "/mnt/cloud-metadata/"
	IMDSParentID     = IMDSPath + "parent-id"
	IMDSInstancePath = IMDSPath + "instance-id"
	IMDSRegionPath   = IMDSPath + "region-name"
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	stdout, err := exec.Pdsh(ctx, fmt.Sprintf("echo $(cat %s)", IMDSInstancePath), nodes)
	if err != nil {
		return nil, err
	}

	return providers.ParseInstanceOutput(stdout)
}

func getParentID() (string, error) {
	return providers.ReadFile(IMDSParentID)
}

func getRegion() (string, error) {
	return providers.ReadFile(IMDSRegionPath)
}

func GetNodeAnnotations() (map[string]string, error) {
	instance, err := providers.ReadFile(IMDSInstancePath)
	if err != nil {
		return nil, err
	}

	region, err := providers.ReadFile(IMDSRegionPath)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		topology.KeyNodeInstance: instance,
		topology.KeyNodeRegion:   region,
	}, nil
}
