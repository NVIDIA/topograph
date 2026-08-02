/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type pdshRunner func(context.Context, string, []string, ...string) (*bytes.Buffer, error)

type ProviderBM struct {
	selector neighborSelector
	run      pdshRunner
}

type BMParams struct {
	Interfaces     []string `mapstructure:"interfaces"`
	InterfaceRegex string   `mapstructure:"interfaceRegex"`
	RailID         string   `mapstructure:"railID"`
}

func NamedLoaderBM() (string, providers.Loader) {
	return NAME_BM, LoaderBM
}

func LoaderBM(_ context.Context, providerConfig providers.Config) (providers.Provider, *httperr.Error) {
	params := &BMParams{}
	if err := config.Decode(providerConfig.Params, params); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(params.RailID) != "" {
		return nil, httperr.NewError(http.StatusBadRequest, "provider parameter railID is only supported by lldp-k8s")
	}
	selector, err := newNeighborSelector(params.Interfaces, params.InterfaceRegex, "")
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	return &ProviderBM{selector: selector, run: exec.Pdsh}, nil
}

func (p *ProviderBM) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	if len(cis) > 1 {
		return nil, httperr.NewError(http.StatusBadRequest, "on-prem does not support multi-region topology requests")
	}
	nodes := topology.GetNodeNameList(cis)
	if len(nodes) == 0 {
		return nil, httperr.NewError(http.StatusBadRequest, "no nodes provided for LLDP discovery")
	}

	stdout, err := p.run(ctx, lldpctlCommand, nodes)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("LLDP collection failed: %v", err))
	}
	neighbors, err := parsePdshOutput(stdout, p.selector)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	topo := topology.NewClusterTopology()
	for instanceID, nodeName := range cis[0].Instances {
		neighbor, ok := neighbors[nodeName]
		if !ok {
			klog.Warningf("No LLDP switch discovered for node %q", nodeName)
			continue
		}
		switchID, err := switchID(neighbor.chassisKey())
		if err != nil {
			return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("node %q: %v", nodeName, err))
		}
		topo.Append(&topology.InstanceTopology{
			InstanceID:  instanceID,
			FabricTiers: topology.ClosestFirstFabricTiers(switchID),
		})
	}
	if topo.Len() == 0 {
		return nil, httperr.NewError(http.StatusBadGateway, "no LLDP switches discovered; verify lldpd is receiving advertisements and provider.params.interfaces or provider.params.interfaceRegex selects the data-plane interface")
	}

	return topo.ToGraph(NAME_BM, cis, 0, false), nil
}

func parsePdshOutput(stdout *bytes.Buffer, selector neighborSelector) (map[string]Neighbor, error) {
	neighbors := make(map[string]Neighbor)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		node, data, ok := strings.Cut(scanner.Text(), ": ")
		if !ok || strings.TrimSpace(data) == "" {
			continue
		}
		parsed, err := parseNeighbors([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("node %q: %w", node, err)
		}
		neighbor, _, err := selectNeighborAndInterfaceRails(parsed, selector)
		if errors.Is(err, errNoLLDPNeighbor) {
			klog.Warningf("node %q: %v", node, err)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("node %q: %w", node, err)
		}
		neighbors[strings.TrimSpace(node)] = neighbor
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan pdsh LLDP output: %w", err)
	}
	return neighbors, nil
}

// Instances2NodeMap implements slurm.instanceMapper.
func (p *ProviderBM) Instances2NodeMap(_ context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string, len(nodes))
	for _, node := range nodes {
		i2n[node] = node
	}
	return i2n, nil
}

// GetInstancesRegions implements slurm.instanceMapper.
func (p *ProviderBM) GetInstancesRegions(_ context.Context, nodes []string) (map[string]string, error) {
	regions := make(map[string]string, len(nodes))
	for _, node := range nodes {
		regions[node] = "local"
	}
	return regions, nil
}
