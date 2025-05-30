/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package topology

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func createVertex(name string, children ...*Vertex) *Vertex {
	v := &Vertex{
		Name: name,
		ID:   name,
	}

	if len(children) != 0 {
		v.Vertices = make(map[string]*Vertex)
		for _, w := range children {
			v.Vertices[w.ID] = w
		}
	}
	return v
}

func getTestGraph() []*Vertex {
	a := createVertex("A")
	b := createVertex("B")
	c := createVertex("C")
	d := createVertex("D")

	l1 := createVertex("L1", a, b)
	l2 := createVertex("L2", a, b)
	l3 := createVertex("L3", a, b)
	l4 := createVertex("L4", a, b)
	l5 := createVertex("L5", c, d)
	l6 := createVertex("L6", c, d)
	l7 := createVertex("L7", c, d)
	l8 := createVertex("L8", c, d)

	s1 := createVertex("S1", l1, l5)
	s2 := createVertex("S2", l1, l5)
	s3 := createVertex("S3", l2, l6)
	s4 := createVertex("S4", l2, l6)
	s5 := createVertex("S5", l3, l7)
	s6 := createVertex("S6", l3, l7)
	s7 := createVertex("S7", l4, l8)
	s8 := createVertex("S8", l4, l8)

	c1 := createVertex("C1", s1, s3, s5, s7)
	c2 := createVertex("C2", s1, s3, s5, s7)
	c3 := createVertex("C3", s2, s4, s6, s8)
	c4 := createVertex("C4", s2, s4, s6, s8)

	return []*Vertex{c1, c2, c3, c4}
}

func replaceName(v *Vertex, newID string, oldIDs ...string) bool {
	if slices.Contains(oldIDs, v.ID) {
		v.ID = newID
		v.Name = newID
		return true
	}
	return false
}

func replaceNames(v *Vertex) string {
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

func normalizeNames(v *Vertex) {
	replaceNames(v)
	vertices := make(map[string]*Vertex)
	for _, w := range v.Vertices {
		key := replaceNames(w)
		vertices[key] = w
		normalizeNames(w)
	}
	v.Vertices = vertices
}

func TestMerger(t *testing.T) {
	g := getTestGraph()

	m := NewMerger(g)
	m.Merge()
	top := m.TopTier()
	for _, v := range top {
		normalizeNames(v)
	}

	a := &Vertex{ID: "A", Name: "A", Vertices: map[string]*Vertex{}}
	b := &Vertex{ID: "B", Name: "B", Vertices: map[string]*Vertex{}}
	c := &Vertex{ID: "C", Name: "C", Vertices: map[string]*Vertex{}}
	d := &Vertex{ID: "D", Name: "D", Vertices: map[string]*Vertex{}}
	l1 := &Vertex{ID: "L1", Name: "L1", Vertices: map[string]*Vertex{"A": a, "B": b}}
	l5 := &Vertex{ID: "L5", Name: "L5", Vertices: map[string]*Vertex{"C": c, "D": d}}
	s1 := &Vertex{ID: "S1", Name: "S1", Vertices: map[string]*Vertex{"L1": l1, "L5": l5}}
	c1 := &Vertex{ID: "C1", Name: "C1", Vertices: map[string]*Vertex{"S1": s1}}

	require.Equal(t, []*Vertex{c1}, top)
}
