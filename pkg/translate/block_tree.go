/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/NVIDIA/topograph/pkg/topology"
)

// blockTreeNode is implemented by host, base, and aggregate block nodes.
type blockTreeNode interface {
	blockTreeNode()
	levelIdentifier() string
}

// hostNode is the lowermost tree level: a host slot or an empty placeholder (host == nil).
type hostNode struct {
	host *topology.HostInfo
}

func (*hostNode) blockTreeNode() {}
func (n *hostNode) levelIdentifier() string {
	if n.host == nil {
		return ""
	}
	return n.host.HostName
}

// baseBlockNode is the Slurm base block level. It always holds exactly baseBlockSize
// host nodes; missing positions or hosts are nil-host placeholders.
type baseBlockNode struct {
	id        string
	domain    string // primary domain ID, pre-computed from id at construction
	leaves    []*hostNode
	nodeCount int // live host count (leaves with non-empty HostName)
}

func (*baseBlockNode) blockTreeNode() {}

func (n *baseBlockNode) levelIdentifier() string { return n.domain }

// aggregateBlockNode groups base blocks or other aggregates. An domain with
// multiple base blocks is represented as an aggregate of baseBlockNode children.
type aggregateBlockNode struct {
	id        string
	children  []blockTreeNode
	nodeCount int // sum of nodeCount across all children
}

func (*aggregateBlockNode) blockTreeNode()            {}
func (n *aggregateBlockNode) levelIdentifier() string { return n.id }

// splitIntoBaseBlocks splits a sorted host list into one or more base blocks of at
// most baseBlockSize leaves each. Overflow blocks get a "#N" suffix on the ID.
func splitIntoBaseBlocks(id string, hosts []*topology.HostInfo, baseBlockSize int) []*baseBlockNode {
	blocks := make([]*baseBlockNode, 0, (len(hosts)+baseBlockSize-1)/baseBlockSize)
	for start := 0; start < len(hosts); start += baseBlockSize {
		end := start + baseBlockSize
		if end > len(hosts) {
			end = len(hosts)
		}
		blockID := id
		if len(blocks) > 0 {
			blockID = fmt.Sprintf("%s#%d", id, len(blocks)+1)
		}
		blocks = append(blocks, newBaseBlock(blockID, hosts[start:end], baseBlockSize))
	}
	return blocks
}

// hostsSorted returns hosts in deterministic alphabetical order so that block
// packing is reproducible across runs.
func hostsSorted(hosts map[string]*topology.HostInfo) []*topology.HostInfo {
	list := make([]*topology.HostInfo, 0, len(hosts))
	for _, h := range hosts {
		list = append(list, h)
	}
	sortHostsByName(list)
	return list
}

// collectBaseBlockSlots returns all base blocks in the tree via a left-to-right DFS.
func collectBaseBlockSlots(tree *aggregateBlockNode) []*baseBlockNode {
	var slots []*baseBlockNode
	var walk func(blockTreeNode)
	walk = func(n blockTreeNode) {
		switch c := n.(type) {
		case *baseBlockNode:
			slots = append(slots, c)
		case *aggregateBlockNode:
			for _, ch := range c.children {
				walk(ch)
			}
		}
	}
	walk(tree)
	return slots
}

// isEmptyBlock reports whether a block carries neither a name nor any nodes.
// A block with a name but no nodes is a valid placeholder — the domain is
// identified but no live hosts were assigned — and is not considered empty.
func isEmptyBlock(b *blockInfo) bool {
	return b == nil || (len(b.name) == 0 && len(b.nodes) == 0)
}

// baseBlockToBlockInfo resolves a base block to a blockInfo using a priority fallback
// chain, because not all blocks have live hosts attached to their leaves:
//  1. Host names directly in leaves (live hosts — normal case)
//  2. Domain IDs from leaves → byName lookup (placeholder hosts: Domain set, HostName empty)
//  3. Domain ID as display name with no nodes (domain known, host list missing entirely)
//  4. Empty blockInfo (tree slot was never filled)
func baseBlockToBlockInfo(bb *baseBlockNode, byName map[string]*blockInfo, seq int) *blockInfo {
	id := fmt.Sprintf("block%03d", seq)
	domainID := bb.levelIdentifier()
	nodes := hostNamesFromLeaves(bb.leaves)
	if len(nodes) > 0 {
		return &blockInfo{id: id, name: blockDisplayName(bb.id, domainID), nodes: nodes}
	}
	for _, domain := range domainIDsFromLeaves(bb.leaves) {
		if b := byName[domain]; b != nil {
			return &blockInfo{
				id:    id,
				name:  blockDisplayName(bb.id, domain),
				nodes: append([]string(nil), b.nodes...),
			}
		}
	}
	if domainID != "" {
		return &blockInfo{id: id, name: blockDisplayName(bb.id, domainID)}
	}
	return &blockInfo{id: id}
}

func blockDisplayName(blockID, primarydomain string) string {
	if primarydomain != "" {
		return primarydomain
	}
	return blockID
}

// domainIDsFromLeaves collects unique domainID values from leaf hosts.
// Sorted for determinism; used as a fallback key set in baseBlockToBlockInfo.
func domainIDsFromLeaves(leaves []*hostNode) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, leaf := range leaves {
		if leaf.host == nil || leaf.host.Domain == "" {
			continue
		}
		if _, ok := seen[leaf.host.Domain]; ok {
			continue
		}
		seen[leaf.host.Domain] = struct{}{}
		ids = append(ids, leaf.host.Domain)
	}
	sort.Strings(ids)
	return ids
}

func hostNamesFromLeaves(leaves []*hostNode) []string {
	nodes := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.host == nil || leaf.host.HostName == "" {
			continue
		}
		nodes = append(nodes, leaf.host.HostName)
	}
	return nodes
}

// extractDomainID returns the primary domain ID from a possibly compound block ID.
// It strips everything from the first compound separator onward:
//
//	"acc-a+acc-b" → "acc-a"   (merged block; separator produced by combinedBlockID)
//	"acc/d0"      → "acc"     (domain-qualified path)
//	"acc#2"       → "acc"     (overflow block produced by splitIntoBaseBlocks)
func extractDomainID(id string) string {
	for i, r := range id {
		if r == '/' || r == '#' || r == '+' {
			return id[:i]
		}
	}
	return id
}

// newBaseBlock builds a baseBlockNode from a pre-sorted host list, filling slots
// left-to-right. Slots beyond the provided hosts remain empty placeholders.
func newBaseBlock(id string, hosts []*topology.HostInfo, baseBlockSize int) *baseBlockNode {
	leaves := make([]*hostNode, baseBlockSize)
	for i := range leaves {
		leaves[i] = &hostNode{}
	}
	nodeCount := 0
	for i, h := range hosts {
		if i >= baseBlockSize {
			break
		}
		leaves[i] = &hostNode{host: h}
		if h.HostName != "" {
			nodeCount++
		}
	}
	return &baseBlockNode{id: id, domain: extractDomainID(id), leaves: leaves, nodeCount: nodeCount}
}

func newEmptyBaseBlock(baseBlockSize int) *baseBlockNode {
	if baseBlockSize <= 0 {
		return &baseBlockNode{}
	}
	leaves := make([]*hostNode, baseBlockSize)
	for i := range leaves {
		leaves[i] = &hostNode{}
	}
	return &baseBlockNode{leaves: leaves}
}

func buildBlockTree(domains topology.DomainMap, blockSizes []int) *aggregateBlockNode {
	if len(blockSizes) == 0 || blockSizes[0] <= 0 {
		return nil
	}
	src := domains.GetDomainTree(blockSizes)
	return convert(src, blockSizes[0])
}

func convert(src *topology.DomainTreeNode, baseBlockSize int) *aggregateBlockNode {
	if src == nil {
		return nil
	}

	target := &aggregateBlockNode{id: src.Name}

	// Leaf node: split hosts into base blocks and pad to DesiredNodeCount slots.
	if len(src.Hosts) > 0 {
		groupSize := src.DesiredNodeCount / baseBlockSize
		hosts := hostsSorted(src.Hosts)
		blocks := splitIntoBaseBlocks(src.Name, hosts, baseBlockSize)
		for i := len(blocks); i < groupSize; i++ {
			blocks = append(blocks, newEmptyBaseBlock(baseBlockSize))
		}

		for _, b := range blocks {
			target.nodeCount += baseBlockSize
			target.children = append(target.children, b)
		}

		return target
	}

	// Interior node: recurse into children in alphabetical order for determinism.
	childCapacity := 0
	for _, name := range slices.Sorted(maps.Keys(src.Children)) {
		converted := convert(src.Children[name], baseBlockSize)
		target.children = append(target.children, converted)
		target.nodeCount += converted.nodeCount
		if childCapacity == 0 {
			childCapacity = converted.nodeCount
		}
	}

	// Pad with empty sibling aggregates until DesiredNodeCount is reached.
	for target.nodeCount < src.DesiredNodeCount && childCapacity > 0 {
		empty := newEmptyChildAggregate(childCapacity, baseBlockSize)
		target.children = append(target.children, empty)
		target.nodeCount += childCapacity
	}

	return target
}

// newEmptyChildAggregate returns an aggregate whose slot capacity matches a
// real child (capacity/baseBlockSize empty base blocks, nodeCount = capacity).
func newEmptyChildAggregate(capacity, baseBlockSize int) *aggregateBlockNode {
	agg := &aggregateBlockNode{}
	for i := 0; i < capacity/baseBlockSize; i++ {
		agg.children = append(agg.children, newEmptyBaseBlock(baseBlockSize))
		agg.nodeCount += baseBlockSize
	}
	return agg
}

// sortHostsByName sorts hosts alphabetically by HostName for deterministic packing.
func sortHostsByName(hosts []*topology.HostInfo) {
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].HostName < hosts[j].HostName
	})
}
