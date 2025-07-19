/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestTreeNetworkTopology(t *testing.T) {
	v, _ := GetTreeTestSet(false)
	cfg := &Config{
		Plugin: topology.TopologyTree,
	}
	nt := NewNetworkTopology(v, cfg)
	expected := map[string][]string{
		"":    {"S1"},
		"S1":  {"S2", "S3"},
		"S2":  {"I21", "I22", "I25"},
		"S3":  {"I34", "I35", "I36"},
		"I21": {},
		"I22": {},
		"I25": {},
		"I34": {},
		"I35": {},
		"I36": {},
	}
	require.Equal(t, expected, nt.tree)

	part := nt.getPartitionTree([]string{"I34", "I35"})
	expected = map[string][]string{
		"":   {"S1"},
		"S1": {"S3"},
		"S3": {"I34", "I35"},
		//"I34": {},
		//"I35": {},
	}

	//map[string][]string{"":[]string{"S1"}, "S1":[]string{"S3"}, "S3":[]string{"I34", "I35"}}
	require.Equal(t, expected, part)

	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, testTreeConfig, buf.String())
}

func TestBlockNetworkTopology(t *testing.T) {
	v, _ := getBlockWithDiffNumNodeTestSet()
	cfg := &Config{
		Plugin: topology.TopologyBlock,
	}
	_ = NewNetworkTopology(v, cfg)
	t.Error("XXX")
}
