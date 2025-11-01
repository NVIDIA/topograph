/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"io"
	"sort"

	"github.com/agrea/ptr"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
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
	id    string
	name  string
	indx  int
	nodes []string
}

type nodeInfo struct {
	instanceID string
	blockID    string
	blockIndx  *int
}

func (cfg *Config) Validate(root *topology.Vertex) error {
	if len(cfg.Topologies) != 0 { // per-partition topology
		if len(cfg.Plugin) != 0 {
			return fmt.Errorf("plugin and topologies parameters are mutually exclusive")
		}
		for topo, spec := range cfg.Topologies {
			switch spec.Plugin {
			case topology.TopologyTree:
				if _, ok := root.Vertices[topology.TopologyTree]; !ok {
					return fmt.Errorf("missing tree topology for topology %q", topo)
				}
			case topology.TopologyBlock:
				if _, ok := root.Vertices[topology.TopologyBlock]; !ok {
					return fmt.Errorf("missing block topology for topology %q", topo)
				}
			case topology.TopologyFlat:
				// nop
			default:
				return fmt.Errorf("unsupported topology plugin %q for topology %q", spec.Plugin, topo)
			}
			if len(spec.Nodes) == 0 && spec.Plugin != topology.TopologyFlat {
				return fmt.Errorf("topology %q specifies no nodes", topo)
			}
		}
	} else { // cluster-wise topology
		switch cfg.Plugin {
		case topology.TopologyTree:
			if _, ok := root.Vertices[topology.TopologyTree]; !ok {
				return fmt.Errorf("missing tree topology")
			}
		case topology.TopologyBlock:
			if _, ok := root.Vertices[topology.TopologyBlock]; !ok {
				return fmt.Errorf("missing block topology")
			}
		default:
			return fmt.Errorf("unsupported topology plugin %q", cfg.Plugin)
		}
	}
	return nil
}

func NewNetworkTopology(root *topology.Vertex, cfg *Config) (*NetworkTopology, error) {
	if err := cfg.Validate(root); err != nil {
		return nil, err
	}

	nt := &NetworkTopology{
		config:   cfg,
		tree:     make(map[string][]string),
		vertices: make(map[string]*topology.Vertex),
		nodeInfo: make(map[string]*nodeInfo),
	}

	nt.initTree(root)
	nt.initBlocks(root)

	return nt, nil
}

func (nt *NetworkTopology) initTree(root *topology.Vertex) {
	tree, ok := root.Vertices[topology.TopologyTree]
	if !ok {
		return
	}

	queue := []*topology.Vertex{tree}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		_, ok := nt.tree[v.ID]
		if !ok {
			nt.tree[v.ID] = []string{}
			nt.vertices[v.ID] = v
			if len(v.Vertices) == 0 {
				nt.nodeInfo[v.Name] = &nodeInfo{instanceID: v.ID}
				klog.V(4).InfoS("initTree: adding nodeInfo", "name", v.Name, "instanceID", v.ID)
			}
		}
		for id, w := range v.Vertices {
			nt.tree[v.ID] = append(nt.tree[v.ID], id)
			queue = append(queue, w)
		}
	}

	for _, val := range nt.tree {
		sort.Strings(val)
	}
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
		ids := make([]string, 0, len(blockRoot.Vertices))
		for id := range blockRoot.Vertices {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			v := blockRoot.Vertices[id]
			bInfo := &blockInfo{
				id:    v.ID,
				name:  v.Name,
				indx:  indx,
				nodes: make([]string, 0, len(v.Vertices)),
			}
			for _, w := range v.Vertices {
				bInfo.nodes = append(bInfo.nodes, w.Name)

				nt.nodeInfo[w.Name] = &nodeInfo{
					instanceID: w.ID,
					blockID:    id,
					blockIndx:  ptr.Int(indx),
				}
				klog.V(4).InfoS("initBlocks: adding nodeInfo", "name", w.Name, "blockID", id, "blockIndx", indx)
			}
			nt.blocks = append(nt.blocks, bInfo)
			indx++
		}
	} else {
		// set block ID for each node
		blockMap := make(map[string]*topology.Vertex)
		for _, block := range blockRoot.Vertices {
			blockMap[block.ID] = block
			for _, v := range block.Vertices {
				if info, ok := nt.nodeInfo[v.Name]; ok {
					info.blockID = block.ID
				}
			}
		}
		// sort blocks according to the node appearance in the tree
		stack := []*topology.Vertex{treeRoot}
		for len(stack) > 0 {
			v := stack[0]
			stack = stack[1:]

			if len(v.Vertices) == 0 { // a leaf (node)
				// check if this node hasn't been visited
				if nInfo, ok := nt.nodeInfo[v.Name]; ok && len(nInfo.blockID) != 0 && nInfo.blockIndx == nil {
					// mark all nodes in this block
					if block, ok := blockMap[nInfo.blockID]; ok {
						bInfo := nt.markBlockNodes(block, indx)
						nt.blocks = append(nt.blocks, bInfo)
						indx++
					}
				}
			} else {
				keys := make([]string, 0, len(v.Vertices))
				for key := range v.Vertices {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					w := v.Vertices[key]
					stack = append([]*topology.Vertex{w}, stack...)
				}
			}
		}
	}
}

// markBlockNodes assigns provided block index to the block nodes
func (nt *NetworkTopology) markBlockNodes(block *topology.Vertex, indx int) *blockInfo {
	pIndx := ptr.Int(indx)
	bInfo := &blockInfo{
		id:    block.ID,
		name:  block.Name,
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

func (nt *NetworkTopology) Generate(wr io.Writer) *httperr.Error {
	if len(nt.config.Topologies) != 0 {
		return nt.toYamlTopology(wr)
	} else {
		if nt.config.Plugin == topology.TopologyBlock {
			return nt.toBlockTopology(wr)
		}
		return nt.toTreeTopology(wr)
	}
}
