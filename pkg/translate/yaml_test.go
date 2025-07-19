/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"bytes"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestTreeYamlTopology(t *testing.T) {
	expected := `- topology: topo1
  cluster_default: false
  tree:
    switches:
        - switch: S1
          children: S2
        - switch: S2
          nodes: Node[201,205]
- topology: topo2
  cluster_default: true
  tree:
    switches:
        - switch: S1
          children: S3
        - switch: S3
          nodes: Node[304-305]
`
	v, _ := GetTreeTestSet(false)
	cfg := &Config{
		Topologies: map[string]*TopologySpec{
			"topo1": {
				Plugin: topology.TopologyTree,
				Nodes:  []string{"Node[201,205]"},
			},
			"topo2": {
				Plugin:         topology.TopologyTree,
				Nodes:          []string{"Node[304,305]"},
				ClusterDefault: true,
			},
		},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, expected, buf.String())
}

func TestBlockYamlTopology(t *testing.T) {
	expected := `- topology: topo1
  cluster_default: true
  block:
    block_sizes:
        - 2
    blocks:
        - block: block0
          nodes: Node[104-105]
- topology: topo2
  cluster_default: false
  block:
    block_sizes:
        - 2
    blocks:
        - block: block0
          nodes: Node[301,303]
`
	v, _ := GetBlockWithMultiIBTestSet()
	cfg := &Config{
		Topologies: map[string]*TopologySpec{
			"topo1": {
				Plugin:         topology.TopologyBlock,
				Nodes:          []string{"Node[104,105]"},
				ClusterDefault: true,
			},
			"topo2": {
				Plugin: topology.TopologyBlock,
				Nodes:  []string{"Node[301,303]"},
			},
		},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, expected, buf.String())
}

func TestMixedYamlTopology(t *testing.T) {
	expected := `- topology: topo1
  cluster_default: false
  tree:
    switches:
        - switch: IB2
          children: S1
        - switch: S1
          children: S3
        - switch: S3
          nodes: Node[201,205]
- topology: topo2
  cluster_default: false
  tree:
    switches:
        - switch: IB1
          children: S4
        - switch: IB2
          children: S1
        - switch: S4
          children: S6
        - switch: S1
          children: S[2-3]
        - switch: S6
          nodes: Node[401-403]
        - switch: S2
          nodes: Node[104-105]
        - switch: S3
          nodes: Node[201,205]
- topology: topo3
  cluster_default: false
  block:
    block_sizes:
        - 2
    blocks:
        - block: block0
          nodes: Node[104-105]
- topology: topo4
  cluster_default: false
  block:
    block_sizes:
        - 2
    blocks:
        - block: block0
          nodes: Node[301-303]
- topology: topo5
  cluster_default: true
  flat: true
`
	v, _ := GetBlockWithMultiIBTestSet()
	cfg := &Config{
		Topologies: map[string]*TopologySpec{
			"topo1": {
				Plugin: topology.TopologyTree,
				Nodes:  []string{"Node[201,205]"},
			},
			"topo2": {
				Plugin: topology.TopologyTree,
				Nodes:  []string{"Node[104,105]", "Node[201,205]", "Node[401-403]"},
			},
			"topo3": {
				Plugin: topology.TopologyBlock,
				Nodes:  []string{"Node[104,105]"},
			},
			"topo4": {
				Plugin:     topology.TopologyBlock,
				Nodes:      []string{"Node[301,302,303]"},
				BlockSizes: []int{2},
			},
			"topo5": {
				Plugin:         topology.TopologyFlat,
				ClusterDefault: true,
			},
		},
	}
	nt, _ := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, expected, buf.String())
}
