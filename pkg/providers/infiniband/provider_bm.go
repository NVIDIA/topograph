/*
 * Copyright 2024 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"context"
	"fmt"
	"net/http"

	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_BM = "infiniband-bm"

type ProviderBM struct{}

func NamedLoaderBM() (string, providers.Loader) {
	return NAME_BM, LoaderBM
}

func LoaderBM(_ context.Context, _ providers.Config) (providers.Provider, *httperr.Error) {
	return &ProviderBM{}, nil
}

func (p *ProviderBM) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	if len(cis) > 1 {
		return nil, httperr.NewError(http.StatusBadRequest, "on-prem does not support multi-region topology requests")
	}

	nodes := topology.GetNodeNameList(cis)

	output, err := exec.Pdsh(ctx, cmdClusterID, nodes)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	domainMap, err := populateDomainsFromPdshOutput(output)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, fmt.Sprintf("failed to populate NVL domains: %v", err))
	}

	treeRoot, err := getIbTree(ctx, cis, &IBNetDiscoverBM{})
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, fmt.Sprintf("getIbTree failed: %v", err))
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

// GetInstancesRegions implements slurm.instanceMapper
func (p *ProviderBM) GetInstancesRegions(ctx context.Context, nodes []string) (map[string]string, error) {
	res := make(map[string]string)
	for _, node := range nodes {
		res[node] = "local"
	}
	return res, nil
}
