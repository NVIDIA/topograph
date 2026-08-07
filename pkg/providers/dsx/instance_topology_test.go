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

package dsx

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

// mockClient is a controllable Client for unit testing pagination and error paths.
type mockClient struct {
	responses []*TopologyResponse
	errors    []error
	idx       int
}

func (m *mockClient) GetTopology(_ context.Context, _ string, _ []string, _ int, _ string) (*TopologyResponse, error) {
	i := m.idx
	m.idx++
	if i < len(m.errors) && m.errors[i] != nil {
		return nil, m.errors[i]
	}
	if i < len(m.responses) {
		return m.responses[i], nil
	}
	return &TopologyResponse{}, nil
}

func newProvider(client Client) *baseProvider {
	return &baseProvider{
		clientFactory: func() (Client, error) { return client, nil },
	}
}

// ---------------------------------------------------------------------------
// buildClusterTopology unit tests
// ---------------------------------------------------------------------------

func TestBuildClusterTopology(t *testing.T) {
	tests := []struct {
		name      string
		switches  []map[string]SwitchAdjacency
		want      map[string]struct{}
		wantLen   int
		checkInst func(t *testing.T, topo *topology.ClusterTopology)
	}{
		{
			name:     "empty switches returns empty topology",
			switches: nil,
			want:     map[string]struct{}{"n1": {}},
			wantLen:  0,
		},
		{
			name: "empty want set returns empty topology",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
			},
			want:    map[string]struct{}{},
			wantLen: 0,
		},
		{
			name: "2-tier: leaf with no parent produces single-tier FabricTiers",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
			},
			want:    map[string]struct{}{"n1": {}},
			wantLen: 1,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				inst := topo.Instances[0]
				require.Equal(t, "n1", inst.InstanceID)
				require.Len(t, inst.FabricTiers, 1, "2-tier network: only leaf tier expected")
				require.Equal(t, "leaf", inst.FabricTiers[0].ID)
			},
		},
		{
			name: "3-tier: core→spine→leaf produces 3-element FabricTiers, closest-first",
			switches: []map[string]SwitchAdjacency{
				{"core": {Switches: []string{"spine"}}},
				{"spine": {Switches: []string{"leaf"}}},
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
			},
			want:    map[string]struct{}{"n1": {}},
			wantLen: 1,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				inst := topo.Instances[0]
				require.Len(t, inst.FabricTiers, 3, "3-tier network: leaf, spine, core expected")
				require.Equal(t, "leaf", inst.FabricTiers[0].ID)
				require.Equal(t, "spine", inst.FabricTiers[1].ID)
				require.Equal(t, "core", inst.FabricTiers[2].ID)
			},
		},
		{
			name: "NVLink accelerated_network_id is propagated to XclrDomainID",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1", AcceleratedNetworkID: "nvl-domain-1"}}}},
			},
			want:    map[string]struct{}{"n1": {}},
			wantLen: 1,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				require.Equal(t, "nvl-domain-1", topo.Instances[0].XclrDomainID)
			},
		},
		{
			name: "empty accelerated_network_id leaves XclrDomainID unset",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1", AcceleratedNetworkID: ""}}}},
			},
			want:    map[string]struct{}{"n1": {}},
			wantLen: 1,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				require.Empty(t, topo.Instances[0].XclrDomainID)
			},
		},
		{
			name: "nodes not in want set are filtered out",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}, {NodeID: "n2"}, {NodeID: "n3"}}}},
			},
			want:    map[string]struct{}{"n1": {}, "n3": {}},
			wantLen: 2,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				ids := make(map[string]struct{}, 2)
				for _, inst := range topo.Instances {
					ids[inst.InstanceID] = struct{}{}
				}
				require.Contains(t, ids, "n1")
				require.Contains(t, ids, "n3")
				require.NotContains(t, ids, "n2")
			},
		},
		{
			name: "node in want but absent from response is not emitted",
			switches: []map[string]SwitchAdjacency{
				{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
			},
			want:    map[string]struct{}{"n1": {}, "n2": {}},
			wantLen: 1,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				require.Equal(t, "n1", topo.Instances[0].InstanceID)
			},
		},
		{
			name: "multiple leaves under one spine: each node gets correct leaf and shared spine",
			switches: []map[string]SwitchAdjacency{
				{"spine": {Switches: []string{"leaf1", "leaf2"}}},
				{"leaf1": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
				{"leaf2": {Nodes: []NodeInfo{{NodeID: "n2"}}}},
			},
			want:    map[string]struct{}{"n1": {}, "n2": {}},
			wantLen: 2,
			checkInst: func(t *testing.T, topo *topology.ClusterTopology) {
				byID := make(map[string]*topology.InstanceTopology, 2)
				for _, inst := range topo.Instances {
					byID[inst.InstanceID] = inst
				}
				n1 := byID["n1"]
				require.Len(t, n1.FabricTiers, 2)
				require.Equal(t, "leaf1", n1.FabricTiers[0].ID)
				require.Equal(t, "spine", n1.FabricTiers[1].ID)

				n2 := byID["n2"]
				require.Len(t, n2.FabricTiers, 2)
				require.Equal(t, "leaf2", n2.FabricTiers[0].ID)
				require.Equal(t, "spine", n2.FabricTiers[1].ID)
			},
		},
		{
			name: "switch entry with only downstream switches and no nodes emits nothing",
			switches: []map[string]SwitchAdjacency{
				{"core": {Switches: []string{"spine"}}},
				{"spine": {Switches: []string{"leaf"}}},
				// leaf is referenced but never defined with nodes
			},
			want:    map[string]struct{}{"n1": {}},
			wantLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			topo := buildClusterTopology(tc.switches, tc.want)
			require.Len(t, topo.Instances, tc.wantLen)
			if tc.checkInst != nil {
				tc.checkInst(t, topo)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// generateInstanceTopology — pagination and error paths via mock client
// ---------------------------------------------------------------------------

func TestGenerateInstanceTopologyCrossPageAncestry(t *testing.T) {
	// Spine and core arrive on page 1; their leaf child and node arrive on page 2.
	// Without the two-phase accumulation fix, n1 would get a 1-tier topology because
	// parentOf would only be built from page 2's switches.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{
				Switches: []map[string]SwitchAdjacency{
					{"core": {Switches: []string{"spine"}}},
					{"spine": {Switches: []string{"leaf"}}},
				},
				NextPageToken: "page2",
			},
			{
				Switches: []map[string]SwitchAdjacency{
					{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
				},
				NextPageToken: "",
			},
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1"}}}
	topo, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.Nil(t, err)
	require.Len(t, topo.Instances, 1)
	inst := topo.Instances[0]
	require.Len(t, inst.FabricTiers, 3, "cross-page ancestry must produce full 3-tier hierarchy")
	require.Equal(t, "leaf", inst.FabricTiers[0].ID)
	require.Equal(t, "spine", inst.FabricTiers[1].ID)
	require.Equal(t, "core", inst.FabricTiers[2].ID)
}

func TestGenerateInstanceTopologyMultiPageNVLink(t *testing.T) {
	// Verify that NVLink domain IDs are correctly propagated through pagination.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{
				Switches: []map[string]SwitchAdjacency{
					{"leaf1": {Nodes: []NodeInfo{{NodeID: "n1", AcceleratedNetworkID: "nvl-a"}}}},
				},
				NextPageToken: "p2",
			},
			{
				Switches: []map[string]SwitchAdjacency{
					{"leaf2": {Nodes: []NodeInfo{{NodeID: "n2", AcceleratedNetworkID: "nvl-b"}}}},
				},
				NextPageToken: "",
			},
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1", "n2": "node2"}}}
	topo, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.Nil(t, err)
	require.Len(t, topo.Instances, 2)
	byID := make(map[string]*topology.InstanceTopology)
	for _, inst := range topo.Instances {
		byID[inst.InstanceID] = inst
	}
	require.Equal(t, "nvl-a", byID["n1"].XclrDomainID)
	require.Equal(t, "nvl-b", byID["n2"].XclrDomainID)
}

func TestGenerateInstanceTopologyAPIErrorOnSecondPage(t *testing.T) {
	mc := &mockClient{
		responses: []*TopologyResponse{
			{
				Switches:      []map[string]SwitchAdjacency{{"spine": {Switches: []string{"leaf"}}}},
				NextPageToken: "p2",
			},
		},
		errors: []error{nil, errors.New("backend unavailable")},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1"}}}
	_, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.NotNil(t, err)
	require.Equal(t, http.StatusBadGateway, err.Code())
	require.Contains(t, err.Error(), "backend unavailable")
}

func TestGenerateInstanceTopologyDirectCyclePageToken(t *testing.T) {
	// A→A: the API echoes the same token on the very next response.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{NextPageToken: "stuck"},
			{NextPageToken: "stuck"},
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1"}}}
	_, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.NotNil(t, err)
	require.Equal(t, http.StatusBadGateway, err.Code())
	require.Contains(t, err.Error(), "page token cycle")
}

func TestGenerateInstanceTopologyNonConsecutiveCyclePageToken(t *testing.T) {
	// A→B→A: the cycle skips one hop, so comparing only against the previous
	// token would miss it and loop indefinitely.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{NextPageToken: "A"},
			{NextPageToken: "B"},
			{NextPageToken: "A"}, // revisits "A" — cycle detected here
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1"}}}
	_, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.NotNil(t, err)
	require.Equal(t, http.StatusBadGateway, err.Code())
	require.Contains(t, err.Error(), "page token cycle")
}

func TestGenerateInstanceTopologyPageLimit(t *testing.T) {
	// Lower the cap so the test doesn't need thousands of mock responses.
	old := maxPaginationPages
	maxPaginationPages = 3
	defer func() { maxPaginationPages = old }()

	// Four pages of unique tokens — exceeds the cap of 3.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{NextPageToken: "t1"},
			{NextPageToken: "t2"},
			{NextPageToken: "t3"},
			{NextPageToken: "t4"},
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1"}}}
	_, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.NotNil(t, err)
	require.Equal(t, http.StatusBadGateway, err.Code())
	require.Contains(t, err.Error(), "maximum page limit")
}

func TestGenerateInstanceTopologyEmptyInstances(t *testing.T) {
	// No instances requested: one API call is made but no nodes are emitted.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{
				Switches: []map[string]SwitchAdjacency{
					{"leaf": {Nodes: []NodeInfo{{NodeID: "n1"}}}},
				},
			},
		},
	}

	topo, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, nil)
	require.Nil(t, err)
	require.Equal(t, 0, topo.Len())
}

func TestGenerateInstanceTopologySinglePage(t *testing.T) {
	// Happy path with a single complete response page.
	mc := &mockClient{
		responses: []*TopologyResponse{
			{
				Switches: []map[string]SwitchAdjacency{
					{"core": {Switches: []string{"spine"}}},
					{"spine": {Switches: []string{"leaf"}}},
					{"leaf": {Nodes: []NodeInfo{
						{NodeID: "n1", AcceleratedNetworkID: "nvl1"},
						{NodeID: "n2", AcceleratedNetworkID: "nvl1"},
					}}},
				},
			},
		},
	}

	cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "node1", "n2": "node2"}}}
	topo, err := newProvider(mc).generateInstanceTopology(context.Background(), nil, cis)

	require.Nil(t, err)
	require.Equal(t, 2, topo.Len())
	for _, inst := range topo.Instances {
		require.Len(t, inst.FabricTiers, 3)
		require.Equal(t, "nvl1", inst.XclrDomainID)
	}
}
