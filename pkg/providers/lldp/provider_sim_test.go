/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func writeLLDPOutput(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lldp.out")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	return path
}

func TestProviderSimNamedLoader(t *testing.T) {
	name, loader := NamedLoaderSim()
	require.Equal(t, NAME_SIM, name)
	require.NotNil(t, loader)
}

func TestLoaderSimValidatesInput(t *testing.T) {
	_, httpErr := LoaderSim(context.Background(), providers.Config{})
	require.NotNil(t, httpErr)
	require.ErrorContains(t, httpErr, "lldpFileName is required")

	_, httpErr = LoaderSim(context.Background(), providers.Config{Params: map[string]any{
		"lldpFileName": filepath.Join(t.TempDir(), "missing.out"),
	}})
	require.NotNil(t, httpErr)
	require.ErrorContains(t, httpErr, "failed to read LLDP file")

	_, httpErr = LoaderSim(context.Background(), providers.Config{Params: map[string]any{
		"lldpFileName": writeLLDPOutput(t, `{"lldp":`),
	}})
	require.NotNil(t, httpErr)
	require.ErrorContains(t, httpErr, "failed to load LLDP file")

	_, httpErr = LoaderSim(context.Background(), providers.Config{Params: map[string]any{
		"lldpFileName": multipleNeighborOutput(t),
	}})
	require.NotNil(t, httpErr)
	require.ErrorContains(t, httpErr, "multiple LLDP switches found")
}

func multipleNeighborOutput(t *testing.T) string {
	t.Helper()
	return writeLLDPOutput(t, multipleNeighborsJSON)
}

func TestProviderSimGenerateTopologyConfig(t *testing.T) {
	provider, httpErr := LoaderSim(context.Background(), providers.Config{Params: map[string]any{
		"lldpFileName": writeLLDPOutput(t, singleNeighborJSON),
	}})
	require.Nil(t, httpErr)

	cis := []topology.ComputeInstances{{
		Region: "local",
		Instances: map[string]string{
			"instance-1": "node-1",
			"instance-2": "node-2",
		},
	}}
	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-001122334455")
	require.Len(t, graph.Tiers.Vertices["lldp-001122334455"].Vertices, 2)
}

func TestProviderSimUsesLLDPCTLFixture(t *testing.T) {
	const fixture = "../../../tests/output/lldp/lldp.log"
	data, err := os.ReadFile(fixture)
	require.NoError(t, err)

	neighbors, err := parseNeighbors(data)
	require.NoError(t, err)
	require.Len(t, neighbors, 49)
	selector := requireNeighborSelector(t, nil, `^eth_r([0-9]+)_p[0-9]+$`, `rail$1`)
	neighbor, interfaceRails, err := selectNeighborAndInterfaceRails(neighbors, selector)
	require.NoError(t, err)
	require.Equal(t, "spcx-qa-spc06-02", neighbor.SystemName)
	require.Equal(t, "f4:20:4d:da:ac:1a", neighbor.ChassisID)
	require.Len(t, interfaceRails, 32)
	require.Equal(t, []string{"rail0"}, interfaceRails["eth_r0_p0"])
	require.Equal(t, []string{"rail1"}, interfaceRails["eth_r1_p7"])
	require.Equal(t, []string{"rail2"}, interfaceRails["eth_r2_p4"])
	require.Equal(t, []string{"rail3"}, interfaceRails["eth_r3_p7"])

	provider, httpErr := LoaderSim(context.Background(), providers.Config{Params: map[string]any{
		"lldpFileName":   fixture,
		"interfaceRegex": `^eth_r([0-9]+)_p[0-9]+$`,
	}})
	require.Nil(t, httpErr)
	cis := []topology.ComputeInstances{{
		Region:    "local",
		Instances: map[string]string{"instance-1": "node-1"},
	}}
	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-f4204ddaac1a")
	require.Contains(t, graph.Tiers.Vertices["lldp-f4204ddaac1a"].Vertices, "instance-1")
}
