/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

type TopologyUnit struct {
	Name    string     `yaml:"topology"`
	Default bool       `yaml:"cluster_default"`
	Flat    bool       `yaml:"flat,omitempty"`
	Tree    *TreeTopo  `yaml:"tree,omitempty"`
	Block   *BlockTopo `yaml:"block,omitempty"`
}

type TreeTopo struct {
	Switches []*Switch           `yaml:"switches"`
	parents  map[string][]string `yaml:"-"`
}

type Switch struct {
	Name     string `yaml:"switch"`
	Children string `yaml:"children,omitempty"`
	Nodes    string `yaml:"nodes,omitempty"`
}

type BlockTopo struct {
	BlockSizes []int             `yaml:"block_sizes"`
	Blocks     []*Block          `yaml:"blocks"`
	parents    map[string]string `yaml:"-"`
}

type Block struct {
	Name  string `yaml:"block"`
	Nodes string `yaml:"nodes,omitempty"`
}

func copySkeleton(topologies []*TopologyUnit) []*TopologyUnit {
	replicas := make([]*TopologyUnit, len(topologies))
	for i, tu := range topologies {
		replica := *tu
		replicas[i] = &replica
		if tu.Tree != nil {
			replica.Tree = &TreeTopo{}
			for _, sw := range tu.Tree.Switches {
				replica.Tree.Switches = append(replica.Tree.Switches, &Switch{
					Name:     sw.Name,
					Children: sw.Children,
				})
			}
		}

		if tu.Block != nil {
			replica.Block = &BlockTopo{
				BlockSizes: append([]int(nil), tu.Block.BlockSizes...),
			}
			for _, b := range tu.Block.Blocks {
				replica.Block.Blocks = append(replica.Block.Blocks, &Block{
					Name: b.Name,
				})
			}
		}
	}
	return replicas
}

// GetTopologies returns a list of TopologyUnit for all topologies defined in the config, ordered by topology name.
func (nt *NetworkTopology) GetTopologies() ([]*TopologyUnit, *httperr.Error) {
	topoNames := make([]string, 0, len(nt.config.Topologies))
	for topoName := range nt.config.Topologies {
		topoNames = append(topoNames, topoName)
	}
	sort.Strings(topoNames)

	topologies := make([]*TopologyUnit, 0, len(topoNames))
	for _, topoName := range topoNames {
		topoSpec := nt.config.Topologies[topoName]
		switch topoSpec.Plugin {
		case topology.TopologyTree:
			tu := nt.getTreeTopologyUnit(topoName, topoSpec)
			topologies = append(topologies, tu)
		case topology.TopologyBlock:
			tu := nt.getBlockTopologyUnit(topoName, topoSpec)
			topologies = append(topologies, tu)
		case topology.TopologyFlat:
			topologies = append(topologies, &TopologyUnit{
				Name:    topoName,
				Flat:    true,
				Default: topoSpec.ClusterDefault,
			})
		default:
			return topologies, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("unsupported topology plugin %q", topoSpec.Plugin))
		}
	}

	return topologies, nil
}

// toYamlTopology generates SLURM cluster topology config in YAML format
func (nt *NetworkTopology) toYamlTopology(wr io.Writer, topologies []*TopologyUnit, skeletonOnly bool) *httperr.Error {

	//Copy only the skeleton (topology structure without node names) if skeletonOnly is true,
	srcForGeneration := topologies
	if skeletonOnly {
		srcForGeneration = copySkeleton(topologies)
	}

	data, err := yaml.Marshal(srcForGeneration)
	if err != nil {
		return httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	if _, err = wr.Write(data); err != nil {
		return httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	return nil
}

func (nt *NetworkTopology) getBlockTopologyUnit(topoName string, topoSpec *TopologySpec) *TopologyUnit {
	// populate map [block indx : blockInfo]
	nodeNames := cluset.Expand(topoSpec.Nodes)
	blockMap := make(map[int]*blockInfo)
	for _, nodeName := range nodeNames {
		info, ok := nt.nodeInfo[nodeName]
		if !ok {
			klog.Warningf("Omitting node %q from partition topology %q: missing node data", nodeName, topoName)
			continue
		}
		if info.blockIndx == nil {
			klog.Warningf("Omitting node %q from partition topology %q: missing block index", nodeName, topoName)
			continue
		}
		indx := *info.blockIndx
		bInfo, ok := blockMap[indx]
		if !ok {
			blockMap[indx] = &blockInfo{
				indx:  indx,
				nodes: []string{nodeName},
			}
		} else {
			bInfo.nodes = append(bInfo.nodes, nodeName)
		}
	}

	tu := &TopologyUnit{
		Name:    topoName,
		Default: topoSpec.ClusterDefault,
	}

	if nBlocks := len(blockMap); nBlocks == 0 {
		tu.Flat = true
	} else {
		// sort blockInfo by block index
		bInfos := make([]*blockInfo, 0, len(blockMap))
		for _, bInfo := range blockMap {
			bInfos = append(bInfos, bInfo)
		}
		sort.Slice(bInfos, func(i, j int) bool {
			return bInfos[i].indx < bInfos[j].indx
		})

		// populate block topology units ordered by block indices
		blocks := make([]*Block, 0, len(bInfos))
		parents := make(map[string]string)
		for indx, bInfo := range bInfos {
			blockName := fmt.Sprintf("block%d", indx+1)
			blocks = append(blocks, &Block{
				Name:  blockName,
				Nodes: strings.Join(cluset.Compact(bInfo.nodes), ","),
			})

			for _, nodeName := range bInfo.nodes {
				parents[nodeName] = blockName
			}
		}

		tu.Block = &BlockTopo{
			BlockSizes: getBlockSizes(bInfos, topoSpec.BlockSizes),
			Blocks:     blocks,
			parents:    parents,
		}
	}
	return tu
}

func (nt *NetworkTopology) getTreeTopologyUnit(topoName string, topoSpec *TopologySpec) *TopologyUnit {
	tu := &TopologyUnit{
		Name:    topoName,
		Default: topoSpec.ClusterDefault,
	}

	// get participating node name and corresponding instance IDs
	nodeNames := cluset.Expand(topoSpec.Nodes)
	nodeSelector := newSelector(nodeNames)
	nodeIDs := make([]string, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		if info, ok := nt.nodeInfo[nodeName]; !ok {
			klog.Warningf("Omitting node %q from partition topology %q: missing instance ID", nodeName, topoName)
			delete(nodeSelector, nodeName)
		} else {
			nodeIDs = append(nodeIDs, info.instanceID)
		}
	}
	// get partial tree for switches
	tree := nt.getPartitionTree(nodeIDs)
	if len(tree) == 0 {
		tu.Flat = true
	} else {
		tu.Tree = &TreeTopo{Switches: []*Switch{}, parents: make(map[string][]string)}

		queue := []string{""}
		for len(queue) > 0 {
			switchID := queue[0]
			queue = queue[1:]
			connects, ok := tree[switchID]
			if !ok {
				// ignore the leaves (nodes)
				continue
			}
			if len(switchID) != 0 {
				v := nt.vertices[switchID]
				sw := &Switch{Name: v.ID}
				childen := []string{}
				leaves := []string{}
				switchSelector := newSelector(connects)
				for _, w := range v.Vertices {
					key := w.ID
					if len(w.Vertices) == 0 {
						key = w.Name
						if nodeSelector[w.Name] {
							leaves = append(leaves, w.Name)
						}
					} else {
						if switchSelector[w.ID] {
							childen = append(childen, w.ID)
						}
					}
					// record parent switch for each child for later use in generating topology spec for each node
					tu.Tree.parents[key] = append([]string{}, tu.Tree.parents[v.ID]...)
					tu.Tree.parents[key] = append(tu.Tree.parents[key], v.ID)
				}
				if len(childen) != 0 || len(leaves) != 0 {
					if len(childen) != 0 {
						sw.Children = strings.Join(cluset.Compact(childen), ",")
					}
					if len(leaves) != 0 {
						sw.Nodes = strings.Join(cluset.Compact(leaves), ",")
					}
					tu.Tree.Switches = append(tu.Tree.Switches, sw)
				}
			}
			queue = append(queue, connects...)
		}
	}
	return tu
}

func (nt *NetworkTopology) getPartitionTree(nodes []string) map[string][]string {
	leaves := make(map[string]bool)
	for _, node := range nodes {
		leaves[node] = true
	}
	return nt.buildSubTree(nt.findRequiredNodes(leaves))
}

// Find all nodes in the path to any target leaf
func (nt *NetworkTopology) findRequiredNodes(leaves map[string]bool) map[string]bool {
	required := make(map[string]bool)

	var dfs func(node string) bool
	dfs = func(node string) bool {
		var keep bool
		if leaves[node] {
			keep = true
		}
		for _, child := range nt.tree[node] {
			if dfs(child) {
				keep = true
			}
		}
		if keep {
			required[node] = true
		}
		return keep
	}
	dfs("")
	return required
}

// Build pruned tree
func (nt *NetworkTopology) buildSubTree(required map[string]bool) map[string][]string {
	subtree := make(map[string][]string)
	for parent, children := range nt.tree {
		if required[parent] {
			for _, child := range children {
				if required[child] {
					subtree[parent] = append(subtree[parent], child)
				}
			}
		}
	}
	return subtree
}

type selector map[string]bool

func newSelector(slice []string) selector {
	s := make(map[string]bool)
	for _, v := range slice {
		s[v] = true
	}
	return s
}

func getTopologySpec(node string, topologies []*TopologyUnit) (string, *httperr.Error) {

	buf := &bytes.Buffer{}
	for _, tu := range topologies {
		parent := ""
		if tu.Tree != nil {
			if switches, exists := tu.Tree.parents[node]; exists {
				parent = strings.Join(switches, ":")
			}
		} else if tu.Block != nil {
			if block, exists := tu.Block.parents[node]; exists {
				parent = block
			}
		}

		if len(parent) == 0 {
			continue
		}

		if buf.Len() != 0 {
			if _, err := fmt.Fprint(buf, ","); err != nil {
				return "", httperr.NewError(http.StatusInternalServerError, err.Error())
			}
		}

		if _, err := fmt.Fprintf(buf, "%s:%s", tu.Name, parent); err != nil {
			return "", httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}
	return buf.String(), nil
}
