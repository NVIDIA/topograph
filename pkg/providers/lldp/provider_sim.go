/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type ProviderSim struct {
	neighbor Neighbor
}

type SimParams struct {
	LLDPFileName   string   `mapstructure:"lldpFileName"`
	Interfaces     []string `mapstructure:"interfaces"`
	InterfaceRegex string   `mapstructure:"interfaceRegex"`
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(_ context.Context, providerConfig providers.Config) (providers.Provider, *httperr.Error) {
	params := &SimParams{}
	if err := config.Decode(providerConfig.Params, params); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	params.LLDPFileName = strings.TrimSpace(params.LLDPFileName)
	if params.LLDPFileName == "" {
		return nil, httperr.NewError(http.StatusBadRequest, "provider parameter lldpFileName is required")
	}
	selector, err := newNeighborSelector(params.Interfaces, params.InterfaceRegex, "")
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	data, err := os.ReadFile(params.LLDPFileName)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("failed to read LLDP file %q: %v", params.LLDPFileName, err))
	}
	neighbors, err := parseNeighbors(data)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("failed to load LLDP file %q: %v", params.LLDPFileName, err))
	}
	neighbor, _, err := selectNeighborAndInterfaceRails(neighbors, selector)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("failed to select LLDP neighbor from %q: %v", params.LLDPFileName, err))
	}

	return &ProviderSim{neighbor: neighbor}, nil
}

func (p *ProviderSim) GenerateTopologyConfig(_ context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	if len(cis) > 1 {
		return nil, httperr.NewError(http.StatusBadRequest, "on-prem does not support multi-region topology requests")
	}
	if len(cis) == 0 || len(cis[0].Instances) == 0 {
		return nil, httperr.NewError(http.StatusBadRequest, "no nodes provided for LLDP simulation")
	}

	switchID, err := switchID(p.neighbor.chassisKey())
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	topo := topology.NewClusterTopology()
	for instanceID := range cis[0].Instances {
		topo.Append(&topology.InstanceTopology{
			InstanceID:  instanceID,
			FabricTiers: topology.ClosestFirstFabricTiers(switchID),
		})
	}
	return topo.ToGraph(NAME_SIM, cis, 0, false), nil
}

// Instances2NodeMap implements slurm.instanceMapper.
func (p *ProviderSim) Instances2NodeMap(_ context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string, len(nodes))
	for _, node := range nodes {
		i2n[node] = node
	}
	return i2n, nil
}

// GetInstancesRegions implements slurm.instanceMapper.
func (p *ProviderSim) GetInstancesRegions(_ context.Context, nodes []string) (map[string]string, error) {
	regions := make(map[string]string, len(nodes))
	for _, node := range nodes {
		regions[node] = "local"
	}
	return regions, nil
}
