/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"slices"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestParseNetq(t *testing.T) {
	links := []Links{
		{CompoundedLinks: []CompoundedLinks{{Id: "A:swp19-*-L1:swp20"}}, Id: "A-*-L1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "A:swp19-*-L2:swp20"}}, Id: "A-*-L2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "A:swp19-*-L3:swp20"}}, Id: "A-*-L3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "A:swp19-*-L4:swp20"}}, Id: "A-*-L4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "B:swp19-*-L1:swp20"}}, Id: "B-*-L1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "B:swp19-*-L2:swp20"}}, Id: "B-*-L2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "B:swp19-*-L3:swp20"}}, Id: "B-*-L3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "B:swp19-*-L4:swp20"}}, Id: "B-*-L4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "C:swp19-*-L5:swp20"}}, Id: "C-*-L5"},
		{CompoundedLinks: []CompoundedLinks{{Id: "C:swp19-*-L6:swp20"}}, Id: "C-*-L6"},
		{CompoundedLinks: []CompoundedLinks{{Id: "C:swp19-*-L7:swp20"}}, Id: "C-*-L7"},
		{CompoundedLinks: []CompoundedLinks{{Id: "C:swp19-*-L8:swp20"}}, Id: "C-*-L8"},
		{CompoundedLinks: []CompoundedLinks{{Id: "D:swp19-*-L5:swp20"}}, Id: "D-*-L5"},
		{CompoundedLinks: []CompoundedLinks{{Id: "D:swp19-*-L6:swp20"}}, Id: "D-*-L6"},
		{CompoundedLinks: []CompoundedLinks{{Id: "D:swp19-*-L7:swp20"}}, Id: "D-*-L7"},
		{CompoundedLinks: []CompoundedLinks{{Id: "D:swp19-*-L8:swp20"}}, Id: "D-*-L8"},

		{CompoundedLinks: []CompoundedLinks{{Id: "L1:swp19-*-S1:swp20"}}, Id: "L1-*-S1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L1:swp19-*-S2:swp20"}}, Id: "L1-*-S2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L5:swp19-*-S1:swp20"}}, Id: "L5-*-S1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L5:swp19-*-S2:swp20"}}, Id: "L5-*-S2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L2:swp19-*-S3:swp20"}}, Id: "L2-*-S3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L2:swp19-*-S4:swp20"}}, Id: "L2-*-S4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L6:swp19-*-S3:swp20"}}, Id: "L6-*-S3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L6:swp19-*-S4:swp20"}}, Id: "L6-*-S4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L3:swp19-*-S5:swp20"}}, Id: "L3-*-S5"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L3:swp19-*-S6:swp20"}}, Id: "L3-*-S6"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L7:swp19-*-S5:swp20"}}, Id: "L7-*-S5"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L7:swp19-*-S6:swp20"}}, Id: "L7-*-S6"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L4:swp19-*-S7:swp20"}}, Id: "L4-*-S7"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L4:swp19-*-S8:swp20"}}, Id: "L4-*-S8"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L8:swp19-*-S7:swp20"}}, Id: "L8-*-S7"},
		{CompoundedLinks: []CompoundedLinks{{Id: "L8:swp19-*-S8:swp20"}}, Id: "L8-*-S8"},

		{CompoundedLinks: []CompoundedLinks{{Id: "S1:swp19-*-C1:swp20"}}, Id: "S1-*-C1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S1:swp19-*-C2:swp20"}}, Id: "S1-*-C2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S2:swp19-*-C3:swp20"}}, Id: "S2-*-C3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S2:swp19-*-C4:swp20"}}, Id: "S2-*-C4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S3:swp19-*-C1:swp20"}}, Id: "S3-*-C1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S3:swp19-*-C2:swp20"}}, Id: "S3-*-C2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S4:swp19-*-C3:swp20"}}, Id: "S4-*-C3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S4:swp19-*-C4:swp20"}}, Id: "S4-*-C4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S5:swp19-*-C1:swp20"}}, Id: "S5-*-C1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S5:swp19-*-C2:swp20"}}, Id: "S5-*-C2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S6:swp19-*-C3:swp20"}}, Id: "S6-*-C3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S6:swp19-*-C4:swp20"}}, Id: "S6-*-C4"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S7:swp19-*-C1:swp20"}}, Id: "S7-*-C1"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S7:swp19-*-C2:swp20"}}, Id: "S7-*-C2"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S8:swp19-*-C3:swp20"}}, Id: "S8-*-C3"},
		{CompoundedLinks: []CompoundedLinks{{Id: "S8:swp19-*-C4:swp20"}}, Id: "S8-*-C4"},
	}
	nodes := []Nodes{{
		Cnode: []CNode{
			{Id: "A", Name: "A", Tier: -1},
			{Id: "B", Name: "B", Tier: -1},
			{Id: "C", Name: "C", Tier: -1},
			{Id: "D", Name: "D", Tier: -1},

			{Id: "L1", Name: "L1", Tier: 1},
			{Id: "L2", Name: "L2", Tier: 1},
			{Id: "L3", Name: "L3", Tier: 1},
			{Id: "L4", Name: "L4", Tier: 1},
			{Id: "L5", Name: "L5", Tier: 1},
			{Id: "L6", Name: "L6", Tier: 1},
			{Id: "L7", Name: "L7", Tier: 1},
			{Id: "L8", Name: "L8", Tier: 1},

			{Id: "S1", Name: "S1", Tier: 2},
			{Id: "S2", Name: "S2", Tier: 2},
			{Id: "S3", Name: "S3", Tier: 2},
			{Id: "S4", Name: "S4", Tier: 2},
			{Id: "S5", Name: "S5", Tier: 2},
			{Id: "S6", Name: "S6", Tier: 2},
			{Id: "S7", Name: "S7", Tier: 2},
			{Id: "S8", Name: "S8", Tier: 2},

			{Id: "C1", Name: "C1", Tier: 3},
			{Id: "C2", Name: "C2", Tier: 3},
			{Id: "C3", Name: "C3", Tier: 3},
			{Id: "C4", Name: "C4", Tier: 3},
		},
	}}
	netqResponse := []NetqResponse{{
		Links: links,
		Nodes: nodes,
	}}

	root, _ := parseNetq(netqResponse, []string{"A"})

	got := []*topology.Vertex{}
	for _, v := range root.Vertices[topology.TopologyTree].Vertices {
		normalizeNames(v)
		got = append(got, v)
	}

	a := &topology.Vertex{ID: "A", Name: "A", Vertices: map[string]*topology.Vertex{}}
	l1 := &topology.Vertex{ID: "L1", Name: "L1", Vertices: map[string]*topology.Vertex{"A": a}}
	s1 := &topology.Vertex{ID: "S1", Name: "S1", Vertices: map[string]*topology.Vertex{"L1": l1}}
	c1 := &topology.Vertex{ID: "C1", Name: "C1", Vertices: map[string]*topology.Vertex{"S1": s1}}

	require.Equal(t, []*topology.Vertex{c1}, got)
}

func replaceName(v *topology.Vertex, newID string, oldIDs ...string) bool {
	if slices.Contains(oldIDs, v.ID) {
		v.ID = newID
		v.Name = newID
		return true
	}
	return false
}

func replaceNames(v *topology.Vertex) string {
	if replaceName(v, "L1", "L2", "L3", "L4") {
		return "L1"
	}
	if replaceName(v, "L5", "L6", "L7", "L8") {
		return "L5"
	}
	if replaceName(v, "S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8") {
		return "S1"
	}
	if replaceName(v, "C1", "C2", "C3", "C4") {
		return "C1"
	}
	return v.ID
}

func normalizeNames(v *topology.Vertex) {
	replaceNames(v)
	vertices := make(map[string]*topology.Vertex)
	for _, w := range v.Vertices {
		key := replaceNames(w)
		vertices[key] = w
		normalizeNames(w)
	}
	v.Vertices = vertices
}
