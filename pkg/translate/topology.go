/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agrea/ptr"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type Config struct {
	Plugin     string // topology plugin (cluster-wide)
	BlockSizes []int
	Topologies map[string]*TopologySpec // per-partiton topology settings
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
	switches   []string
}

func (cfg *Config) Validate(graph *topology.Graph) error {
	if len(cfg.Topologies) != 0 { // per-partition topology
		if len(cfg.Plugin) != 0 {
			return fmt.Errorf("plugin and topologies parameters are mutually exclusive")
		}
		for topo, spec := range cfg.Topologies {
			switch spec.Plugin {
			case topology.TopologyTree:
				if graph == nil || graph.Tiers == nil {
					return fmt.Errorf("missing tree topology for topology %q", topo)
				}
			case topology.TopologyBlock:
				if graph == nil || graph.Domains == nil {
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
	} else { // cluster-wide topology
		switch cfg.Plugin {
		case topology.TopologyTree:
			if graph == nil || graph.Tiers == nil {
				return fmt.Errorf("missing tree topology")
			}
		case topology.TopologyBlock:
			if graph == nil || graph.Domains == nil {
				return fmt.Errorf("missing block topology")
			}
		default:
			return fmt.Errorf("unsupported topology plugin %q", cfg.Plugin)
		}
	}
	return nil
}

func NewNetworkTopology(graph *topology.Graph, cfg *Config) (*NetworkTopology, error) {
	if err := cfg.Validate(graph); err != nil {
		return nil, err
	}

	nt := &NetworkTopology{
		config:   cfg,
		tree:     make(map[string][]string),
		vertices: make(map[string]*topology.Vertex),
		nodeInfo: make(map[string]*nodeInfo),
	}

	nt.initTree(graph)
	nt.initBlocks(graph)

	return nt, nil
}

func (nt *NetworkTopology) initTree(graph *topology.Graph) {
	if graph == nil || graph.Tiers == nil {
		return
	}

	parentMap := make(map[string][]string)
	queue := []*topology.Vertex{graph.Tiers}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		_, ok := nt.tree[v.ID]
		if !ok {
			nt.tree[v.ID] = []string{}
			nt.vertices[v.ID] = v
			if len(v.Vertices) == 0 {
				nt.nodeInfo[v.Name] = &nodeInfo{instanceID: v.ID, switches: parentMap[v.ID]}
				klog.V(4).InfoS("initTree: adding nodeInfo", "name", v.Name, "instanceID", v.ID, "switches", parentMap[v.ID])
			}
		}
		for id, w := range v.Vertices {
			if len(v.ID) != 0 {
				parentMap[w.ID] = append([]string{}, parentMap[v.ID]...)
				parentMap[w.ID] = append(parentMap[w.ID], v.ID)
			}
			nt.tree[v.ID] = append(nt.tree[v.ID], id)
			queue = append(queue, w)
		}
	}

	for _, val := range nt.tree {
		sort.Strings(val)
	}
}

func toBlockInfos(domains topology.DomainMap) []*blockInfo {
	domainNames := make([]string, 0, len(domains))
	for domainName := range domains {
		domainNames = append(domainNames, domainName)
	}
	sort.Strings(domainNames)

	blocks := make([]*blockInfo, 0, len(domainNames))
	for i, domainName := range domainNames {
		domain := domains[domainName]
		nodes := make([]string, 0, len(domain))
		for node := range domain {
			nodes = append(nodes, node)
		}
		sort.Strings(nodes)

		blocks = append(blocks, &blockInfo{
			id:    fmt.Sprintf("block%03d", i+1),
			name:  domainName,
			nodes: nodes,
		})
	}

	return blocks
}

func (nt *NetworkTopology) initBlocks(graph *topology.Graph) {
	if graph == nil || graph.Domains == nil {
		klog.Warning("block topology data not found")
		return
	}

	if len(graph.Domains) == 0 {
		klog.Warning("no blocks found in block topology")
		return
	}

	domainBlocks := toBlockInfos(graph.Domains)
	nt.blocks = make([]*blockInfo, 0, len(domainBlocks))
	indx := 0

	if graph.Tiers == nil { // no tree data
		for _, bInfo := range domainBlocks {
			bInfo.indx = indx
			for _, node := range bInfo.nodes {
				nt.nodeInfo[node] = &nodeInfo{
					instanceID: graph.Domains[bInfo.name][node],
					blockID:    bInfo.id,
					blockIndx:  ptr.Int(indx),
				}
				klog.V(4).InfoS("initBlocks: adding nodeInfo", "name", node, "blockID", bInfo.id, "blockIndx", indx)
			}
			nt.blocks = append(nt.blocks, bInfo)
			indx++
		}
	} else {
		// set block ID for each node
		blockMap := make(map[string]*blockInfo)
		for _, block := range domainBlocks {
			blockMap[block.id] = block
			for _, node := range block.nodes {
				if info, ok := nt.nodeInfo[node]; ok {
					info.blockID = block.id
				}
			}
		}
		// sort blocks according to the node appearance in the tree
		queue := []*topology.Vertex{graph.Tiers}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]

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
					queue = append(queue, w)
				}
			}
		}
	}
}

// markBlockNodes assigns provided block index to the block nodes
func (nt *NetworkTopology) markBlockNodes(block *blockInfo, indx int) *blockInfo {
	pIndx := ptr.Int(indx)
	bInfo := &blockInfo{
		id:    block.id,
		name:  block.name,
		indx:  indx,
		nodes: append([]string(nil), block.nodes...),
	}
	for _, node := range bInfo.nodes {
		if info, ok := nt.nodeInfo[node]; ok {
			info.blockIndx = pIndx
		}
	}
	return bInfo
}

func (nt *NetworkTopology) Generate(wr io.Writer) *httperr.Error {
	_, err := nt.GenerateTopologyConfig(wr, false)
	return err
}

func (nt *NetworkTopology) GenerateTopologyConfig(wr io.Writer, skeletonOnly bool) ([]*TopologyUnit, *httperr.Error) {
	topologies, httpErr := nt.GetTopologies()
	if httpErr != nil {
		return topologies, httpErr
	}

	if len(nt.config.Topologies) != 0 {
		return topologies, nt.toYamlTopology(wr, topologies, skeletonOnly)
	} else {
		if nt.config.Plugin == topology.TopologyBlock {
			return topologies, nt.toBlockTopology(wr, skeletonOnly)
		}
		return topologies, nt.toTreeTopology(wr, skeletonOnly)
	}
}

func (nt *NetworkTopology) GetNodeTopologySpec(node string, topologies []*TopologyUnit) (string, *httperr.Error) {

	if _, exists := nt.nodeInfo[node]; !exists {
		return "", nil
	}

	if len(nt.config.Topologies) != 0 {
		return getTopologySpec(node, topologies)
	} else {
		nodeInfo, ok := nt.nodeInfo[node]
		if !ok {
			return "", nil
		}
		switch nt.config.Plugin {
		case topology.TopologyBlock:
			return fmt.Sprintf("default:%s", nodeInfo.blockID), nil
		case topology.TopologyTree:
			return fmt.Sprintf("default:%s", strings.Join(nodeInfo.switches, ":")), nil
		default:
			return "", nil
		}
	}
}
