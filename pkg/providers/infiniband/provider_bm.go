/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_BM = "infiniband-bm"

type ProviderBM struct{}

var ErrMultiRegionNotSupported = errors.New("on-prem does not support multi-region topology requests")

func NamedLoaderBM() (string, providers.Loader) {
	return NAME_BM, LoaderBM
}

func LoaderBM(ctx context.Context, config providers.Config) (providers.Provider, error) {
	return &ProviderBM{}, nil
}

func (p *ProviderBM) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(cis) > 1 {
		return nil, ErrMultiRegionNotSupported
	}

	nodes := topology.GetNodeNameList(cis)

	output, err := exec.Pdsh(ctx, cmdClusterID, nodes)
	if err != nil {
		return nil, err
	}

	domainMap, err := populateDomainsFromPdshOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to populate NVL domains: %v", err)
	}

	treeRoot, err := getIbTree(ctx, cis, &IBNetDiscoverBM{})
	if err != nil {
		return nil, fmt.Errorf("getIbTree failed: %v", err)
	}

	return toGraph(domainMap, treeRoot), nil
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *ProviderBM) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *ProviderBM) GetComputeInstancesRegion(_ context.Context) (string, error) {
	return "local", nil
}
