/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	IMDSPath       = "/mnt/cloud-metadata/"
	IMDSParentID   = IMDSPath + "parent-id"
	IMDSRegionPath = IMDSPath + "region-name"

	MACCmd = "iface=$(awk '$2=='00000000' {print $1, $8}' /proc/net/route | sort -k2n | head -n1 | cut -d' ' -f1); cat /sys/class/net/$iface/address | tr '[:lower:]' '[:upper:]'"
)

func instanceToNodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	stdout, err := exec.Pdsh(ctx, MACCmd, nodes)
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

func GetNodeAnnotations(ctx context.Context) (map[string]string, error) {
	mac, err := exec.Exec(ctx, "sh", []string{"-c", MACCmd}, nil)
	if err != nil {
		return nil, err
	}

	region, err := providers.ReadFile(IMDSRegionPath)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		topology.KeyNodeInstance: mac.String(),
		topology.KeyNodeRegion:   region,
	}, nil
}
