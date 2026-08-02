/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

const singleNeighborJSON = `{"lldp":{"interface":{"eno1":{"via":"LLDP","chassis":{"leaf-1":{"id":{"type":"mac","value":"00:11:22:33:44:55"}}},"port":{"id":{"value":"Ethernet1/1"}}}}}}`

func TestProviderBMGenerateTopologyConfig(t *testing.T) {
	provider := &ProviderBM{
		run: func(_ context.Context, command string, nodes []string, _ ...string) (*bytes.Buffer, error) {
			require.Equal(t, lldpctlCommand, command)
			require.ElementsMatch(t, []string{"node-1", "node-2"}, nodes)
			return bytes.NewBufferString("node-1: " + singleNeighborJSON + "\nnode-2: " + singleNeighborJSON + "\n"), nil
		},
	}
	cis := []topology.ComputeInstances{{
		Region: "local",
		Instances: map[string]string{
			"node-1": "node-1",
			"node-2": "node-2",
		},
	}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-001122334455")
	require.Len(t, graph.Tiers.Vertices["lldp-001122334455"].Vertices, 2)
}

func TestProviderBMReportsAmbiguousNode(t *testing.T) {
	compact := bytes.NewBuffer(nil)
	for _, line := range bytes.Split([]byte(multipleNeighborsJSON), []byte("\n")) {
		compact.Write(line)
	}
	provider := &ProviderBM{
		run: func(_ context.Context, _ string, _ []string, _ ...string) (*bytes.Buffer, error) {
			return bytes.NewBufferString("node-1: " + compact.String() + "\n"), nil
		},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"node-1": "node-1"}}}

	_, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "multiple LLDP switches found")
}

func TestProviderBMBuildsGraphFromInterfaceRegex(t *testing.T) {
	compact := bytes.NewBuffer(nil)
	for _, line := range bytes.Split([]byte(multipleNeighborsJSON), []byte("\n")) {
		compact.Write(line)
	}
	provider := &ProviderBM{
		selector: requireNeighborSelector(t, nil, `^eno[12]$`, ""),
		run: func(_ context.Context, _ string, _ []string, _ ...string) (*bytes.Buffer, error) {
			return bytes.NewBufferString("node-1: " + compact.String() + "\n"), nil
		},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"node-1": "node-1"}}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-001122334455")
	require.Contains(t, graph.Tiers.Vertices["lldp-001122334455"].Vertices, "node-1")
}

func TestLoaderBMValidatesInterfaces(t *testing.T) {
	_, httpErr := LoaderBM(context.Background(), providersConfig(map[string]any{"interfaces": []string{"eno1", "eno1"}}))
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "duplicate interface")
}

func TestLoaderBMValidatesInterfaceRegex(t *testing.T) {
	_, httpErr := LoaderBM(context.Background(), providersConfig(map[string]any{
		"interfaces":     []string{"eno1"},
		"interfaceRegex": `^eno`,
	}))
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "mutually exclusive")

	_, httpErr = LoaderBM(context.Background(), providersConfig(map[string]any{"interfaceRegex": `[`}))
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "invalid interfaceRegex")

	_, httpErr = LoaderBM(context.Background(), providersConfig(map[string]any{
		"interfaceRegex": `^eno`,
		"railID":         `rail$1`,
	}))
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "railID is only supported by lldp-k8s")
}
