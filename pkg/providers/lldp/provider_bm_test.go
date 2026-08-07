/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

func TestLoaderBMValidatesInterfaces(t *testing.T) {
	_, httpErr := LoaderBM(context.Background(), providersConfig(map[string]any{"interfaces": []string{"eno1", "eno1"}}))
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "duplicate interface")
}
