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

	"github.com/NVIDIA/topograph/internal/cluset"
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
	Switches []*Switch `yaml:"switches"`
}

type Switch struct {
	Name     string `yaml:"switch"`
	Children string `yaml:"children,omitempty"`
	Nodes    string `yaml:"nodes,omitempty"`
}

type BlockTopo struct {
	BlockSizes []int    `yaml:"block_sizes"`
	Blocks     []*Block `yaml:"blocks"`
}

type Block struct {
	Name  string `yaml:"block"`
	Nodes string `yaml:"nodes"`
}

// toYamlTopology generates SLURM cluster topology config in YAML format
func (nt *NetworkTopology) toYamlTopology(wr io.Writer) error {
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
			tu, err := nt.getTreeTopologyUnit(topoName, topoSpec)
			if err != nil {
				return err
			}
			topologies = append(topologies, tu)
		case topology.TopologyBlock:
			tu, err := nt.getBlockTopologyUnit(topoName, topoSpec)
			if err != nil {
				return err
			}
			topologies = append(topologies, tu)
		case topology.TopologyFlat:
			topologies = append(topologies, &TopologyUnit{
				Name:    topoName,
				Flat:    true,
				Default: topoSpec.ClusterDefault,
			})
		default:
			return fmt.Errorf("unsupported topology plugin %q", topoSpec.Plugin)
		}
	}

	// sort for consistency
	sort.Slice(topologies, func(i, j int) bool {
		return topologies[i].Name < topologies[j].Name
	})

	data, err := yaml.Marshal(topologies)
	if err != nil {
		return err
	}

	_, err = wr.Write(data)
	return err
}

func (nt *NetworkTopology) getBlockTopologyUnit(topoName string, topoSpec *TopologySpec) (*TopologyUnit, error) {
	// populate map [block indx : blockInfo]
	nodeNames := cluset.Expand(topoSpec.Nodes)
	blockMap := make(map[int]*blockInfo)
	for _, nodeName := range nodeNames {
		info, ok := nt.nodeInfo[nodeName]
		if !ok {
			klog.Warningf("Missing node data for node %q", nodeName)
			continue
		}
		if info.blockIndx == nil {
			klog.Warningf("Missing block index for node %q", nodeName)
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
	for indx, bInfo := range bInfos {
		blocks = append(blocks, &Block{
			Name:  fmt.Sprintf("block%d", indx),
			Nodes: strings.Join(cluset.Compact(bInfo.nodes), ","),
		})
	}

	blockSizes := getBlockSize(bInfos, topoSpec.BlockSizes, false)

	return &TopologyUnit{
		Name:    topoName,
		Default: topoSpec.ClusterDefault,
		Block: &BlockTopo{
			BlockSizes: blockSizes,
			Blocks:     blocks,
		},
	}, nil
}

func (nt *NetworkTopology) getTreeTopologyUnit(topoName string, topoSpec *TopologySpec) (*TopologyUnit, error) {
	tu := &TopologyUnit{
		Name:    topoName,
		Default: topoSpec.ClusterDefault,
		Tree: &TreeTopo{
			Switches: []*Switch{},
		},
	}

	// get participating node name and corresponding instance IDs
	nodeNames := cluset.Expand(topoSpec.Nodes)
	nodeSelector := newSelector(nodeNames)
	nodeIDs := make([]string, 0, len(nodeNames))
	for _, nodeName := range nodeNames {
		info, ok := nt.nodeInfo[nodeName]
		if !ok {
			return nil, fmt.Errorf("missing instance ID for node %q", nodeName)
		}
		nodeIDs = append(nodeIDs, info.instanceID)
	}
	// get partial tree for switches
	tree := nt.getPartitionTree(nodeIDs)
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
				if len(w.Vertices) == 0 {
					if nodeSelector[w.Name] {
						leaves = append(leaves, w.Name)
					}
				} else {
					if switchSelector[w.ID] {
						childen = append(childen, w.ID)
					}
				}
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

	return tu, nil
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
		keep := false
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
