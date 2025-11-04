/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestParseNetq(t *testing.T) {
	links := []Links{
		{Id: "A-*-L1"},
		{Id: "A-*-L2"},
		{Id: "A-*-L3"},
		{Id: "A-*-L4"},
		{Id: "B-*-L1"},
		{Id: "B-*-L2"},
		{Id: "B-*-L3"},
		{Id: "B-*-L4"},
		{Id: "C-*-L5"},
		{Id: "C-*-L6"},
		{Id: "C-*-L7"},
		{Id: "C-*-L8"},
		{Id: "D-*-L5"},
		{Id: "D-*-L6"},
		{Id: "D-*-L7"},
		{Id: "D-*-L8"},

		{Id: "L1-*-S1"},
		{Id: "L1-*-S2"},
		{Id: "L5-*-S1"},
		{Id: "L5-*-S2"},
		{Id: "L2-*-S3"},
		{Id: "L2-*-S4"},
		{Id: "L6-*-S3"},
		{Id: "L6-*-S4"},
		{Id: "L3-*-S5"},
		{Id: "L3-*-S6"},
		{Id: "L7-*-S5"},
		{Id: "L7-*-S6"},
		{Id: "L4-*-S7"},
		{Id: "L4-*-S8"},
		{Id: "L8-*-S7"},
		{Id: "L8-*-S8"},

		{Id: "S1-*-C1"},
		{Id: "S1-*-C2"},
		{Id: "S2-*-C3"},
		{Id: "S2-*-C4"},
		{Id: "S3-*-C1"},
		{Id: "S3-*-C2"},
		{Id: "S4-*-C3"},
		{Id: "S4-*-C4"},
		{Id: "S5-*-C1"},
		{Id: "S5-*-C2"},
		{Id: "S6-*-C3"},
		{Id: "S6-*-C4"},
		{Id: "S7-*-C1"},
		{Id: "S7-*-C2"},
		{Id: "S8-*-C3"},
		{Id: "S8-*-C4"},
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

	// valid input
	netqResponse := []NetqResponse{{
		Links: links,
		Nodes: nodes,
	}}

	root, err := parseNetq(netqResponse, map[string]bool{"A": true})
	require.Nil(t, err)

	top := []*topology.Vertex{}
	for _, v := range root.Vertices[topology.TopologyTree].Vertices {
		top = append(top, v)
	}

	a := &topology.Vertex{ID: "A", Name: "A"}
	l1 := &topology.Vertex{ID: "L1", Name: "L1", Vertices: map[string]*topology.Vertex{"A": a}}
	s1 := &topology.Vertex{ID: "S1", Name: "S1", Vertices: map[string]*topology.Vertex{"L1": l1}}
	c1 := &topology.Vertex{ID: "C1", Name: "C1", Vertices: map[string]*topology.Vertex{"S1": s1}}

	require.Equal(t, []*topology.Vertex{c1}, top)

	// invalid input
	netqResponse = []NetqResponse{{}, {}}
	_, err = parseNetq(netqResponse, map[string]bool{})
	require.EqualError(t, err, "invalid NetQ response: multiple entries")
}

func TestGetURL(t *testing.T) {
	testCases := []struct {
		name    string
		baseURL string
		paths   []string
		query   map[string]string
		url     string
		err     string
	}{
		{
			name:    "Case 1: bad base URL",
			baseURL: "123:",
			err:     `parse "123:": first path segment in URL cannot contain colon`,
		},
		{
			name:    "Case 2: single base URL",
			baseURL: "http://localhost",
			url:     "http://localhost",
		},
		{
			name:    "Case 3: base URL with path",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			url:     "http://localhost/a/b/c/d",
		},
		{
			name:    "Case 3: base URL with path and query",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			query:   map[string]string{"key1": "val1", "key2": "val2"},
			url:     "http://localhost/a/b/c/d?key1=val1&key2=val2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := getURL(tc.baseURL, tc.query, tc.paths...)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {

				require.Nil(t, err)
				require.Equal(t, tc.url, u)
			}
		})
	}
}
