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
  flat: false
  tree:
    switches:
        - switch: S1
          children: S2
        - switch: S2
          nodes: Node[201,205]
- topology: topo2
  cluster_default: false
  flat: false
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
				Plugin: topology.TopologyTree,
				Nodes:  []string{"Node[304,305]"},
			},
		},
	}
	nt := NewNetworkTopology(v, cfg)
	buf := &bytes.Buffer{}
	err := nt.Generate(buf)
	require.NoError(t, err)
	require.Equal(t, expected, buf.String())
}
