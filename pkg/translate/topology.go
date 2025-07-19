/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"io"

	"github.com/agrea/ptr"

	"github.com/NVIDIA/topograph/pkg/topology"
)

type Config struct {
	Plugin       string // topology plugin (cluster-wide)
	BlockSizes   []int
	FakeNodePool string
	Topologies   map[string]*TopologySpec // per-partiton topology settings
}

// TopologySpec define topology for a partition
type TopologySpec struct {
	Plugin         string
	BlockSizes     []int
	ClusterDefault bool
	Nodes          []string
}

type NetworkTopology struct {
	config   *Config
	tree     map[string][]string         // adjacency list
	blocks   []*blockInfo                // blocks
	vertices map[string]*topology.Vertex // object ID to Vertex map
	nodeInfo map[string]*nodeInfo        // node name to nodeInfo map
}

type blockInfo struct {
	indx  int
	nodes []string
}

type nodeInfo struct {
	instanceID string
	blockIndx  *int
}

func NewNetworkTopology(root *topology.Vertex, cfg *Config) *NetworkTopology {
	nt := &NetworkTopology{
		config:   cfg,
		tree:     make(map[string][]string),
		vertices: make(map[string]*topology.Vertex),
		nodeInfo: make(map[string]*nodeInfo),
	}

	nt.initTree(root)
	nt.initBlocks(root)

	return nt
}

func (nt *NetworkTopology) initBlocks(root *topology.Vertex) {
	blockRoot, ok := root.Vertices[topology.TopologyBlock]
	if !ok {
		return
	}

	nt.blocks = make([]*blockInfo, 0, len(blockRoot.Vertices))
	indx := 0

	treeRoot, ok := root.Vertices[topology.TopologyTree]
	if !ok { // no tree data
		for _, v := range blockRoot.Vertices {
			bInfo := &blockInfo{
				indx:  indx,
				nodes: make([]string, 0, len(v.Vertices)),
			}
			for _, w := range v.Vertices {
				bInfo.nodes = append(bInfo.nodes, w.Name)
			}
			nt.blocks = append(nt.blocks, bInfo)
			indx++
		}
	} else { // sort blocks according to the node appearance in the tree
		stack := []*topology.Vertex{treeRoot}
		for len(stack) > 0 {
			v := stack[0]
			stack = stack[1:]

			if len(v.Vertices) == 0 { // a leaf (node)
				// check if this node hasn't been visited
				if nInfo, ok := nt.nodeInfo[v.Name]; ok && nInfo.blockIndx == nil {
					// mark all nodes in this block
					if bInfo := nt.markBlockNodes(blockRoot, v.ID, indx); bInfo != nil {
						nt.blocks = append(nt.blocks, bInfo)
						indx++
					}
				}
			} else {
				for _, w := range v.Vertices {
					stack = append([]*topology.Vertex{w}, stack...)
				}
			}
		}
	}
}

func (nt *NetworkTopology) markBlockNodes(blockRoot *topology.Vertex, instanceID string, indx int) *blockInfo {
	pIndx := ptr.Int(indx)
	for _, block := range blockRoot.Vertices {
		if _, ok := block.Vertices[instanceID]; ok {

			bInfo := &blockInfo{
				indx:  indx,
				nodes: make([]string, 0, len(block.Vertices)),
			}

			for _, v := range block.Vertices {
				bInfo.nodes = append(bInfo.nodes, v.Name)
				if info, ok := nt.nodeInfo[v.Name]; ok {
					info.blockIndx = pIndx
				}
			}

			return bInfo
		}
	}
	return nil
}

func (nt *NetworkTopology) Generate(wr io.Writer) error {
	if len(nt.config.Topologies) != 0 {
		return nt.toYamlTopology(wr)
	} else {
		if nt.config.Plugin == topology.TopologyBlock {
			return nil //nt.toBlockTopology(wr)
		}
		return nt.toTreeTopology(wr)
	}
}

/*
- topology: topo1
  cluster_default: true
  tree:
    switches:
      - switch: sw_root
        children: s[1-2]
      - switch: s1
        nodes: node[01-02]
      - switch: s2
        nodes: node[03-04]
- topology: topo2
  cluster_default: false
  block:
    block_sizes:
      - 4
      - 16
    blocks:
      - block: b1
        nodes: node[01-04]
      - block: b2
        nodes: node[05-08]
      - block: b3
        nodes: node[09-12]
      - block: b4
        nodes: node[13-16]
- topology: topo3
  cluster_default: false
  flat: true
*/
