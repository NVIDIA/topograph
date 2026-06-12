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
}

// hostNode is the lowermost tree level: a host slot or an empty placeholder (host == nil).
type hostNode struct {
	host *topology.HostInfo
}

func (*hostNode) blockTreeNode() {}

// baseBlockNode is the Slurm base block level. It always holds exactly baseBlockSize
// host nodes; missing positions or hosts are nil-host placeholders.
type baseBlockNode struct {
	id        string
	domain    string // primary domain ID, pre-computed from id at construction
	leaves    []*hostNode
	nodeCount int // live host count (leaves with non-empty HostName)
}

func (*baseBlockNode) blockTreeNode() {}

func (n *baseBlockNode) domainIdentifier() string { return n.domain }

// aggregateBlockNode groups base blocks or other aggregates. An domain with
// multiple base blocks is represented as an aggregate of baseBlockNode children.
type aggregateBlockNode struct {
	children  []blockTreeNode
	nodeCount int // sum of nodeCount across all children
}

func (*aggregateBlockNode) blockTreeNode() {}

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
	domainID := bb.domainIdentifier()
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

// buildBlockTree packs domains into a padded block tree shaped by blockSizes.
//
// packDomainNodes handles all domain-level work (packing, equalization, root
// padding) and returns both the fully padded domain nodes and the live bb count.
// buildBlockTree uses the capacity recorded on each domain node to decide whether
// higher-tier aggregation is needed, then delegates to packAggregateNodes.
func buildBlockTree(domains topology.DomainMap, blockSizes []int) *aggregateBlockNode {
	baseBlockSize := blockSizes[0]
	groupSize := groupSizeFromDomains(domains, baseBlockSize, blockSizes[len(blockSizes)-1])

	//Pad each domain to a multiple of groupSize base blocks,
	//then pack those blocks into aggregate nodes of size groupSize until we reach the top tier or satisfy blockSizes[last].
	domainNodes := packDomainNodes(domains, baseBlockSize, groupSize)
	if len(domainNodes) == 0 {
		return nil
	}

	// All domain nodes returned by packDomainNodes are aggregateBlockNodes whose
	// nodeCount reflects the slot capacity of that domain (not just live hosts).
	// When all domains are complete, their capacity is already >= lastBS so
	// getRemainingBlocks returns nil and we return the flat tree unchanged.
	domCapacity := getNodeCount(domainNodes[0])
	remaining := getRemainingBlocks(blockSizes, domCapacity)
	if len(remaining) == 0 {
		total := 0
		for _, n := range domainNodes {
			total += getNodeCount(n)
		}
		return &aggregateBlockNode{children: domainNodes, nodeCount: total}
	}

	//Build a higher-tier tree above the domain nodes with groupSize fanout and enough levels to satisfy blockSizes[last].
	//The tree is padded with empty aggregate nodes as needed to reach the next power-of-two multiple of groupSize,
	//so every domain occupies complete groups at every tier.
	//The final tree may be wider than blockSizes[last] but is guaranteed to have at least that capacity.
	completed := []int{baseBlockSize, domCapacity}
	desiredGroupSize := remaining[len(remaining)-1] / domCapacity
	nodesMap := map[string][]blockTreeNode{"root": domainNodes}
	aggregateNodes, aggCount := packAggregateNodes(nodesMap, completed, desiredGroupSize)
	return &aggregateBlockNode{children: aggregateNodes, nodeCount: aggCount}
}

func packDomainNodes(domains topology.DomainMap, baseBlockSize, groupSize int) []blockTreeNode {
	if baseBlockSize <= 0 {
		return nil
	}
	domainIDs := slices.Sorted(maps.Keys(domains))
	if len(domainIDs) == 0 {
		return nil
	}

	blockNodes := make([]blockTreeNode, 0, len(domainIDs))

	for _, domainID := range domainIDs {
		hosts := hostsSorted(domains[domainID])
		blocks := splitIntoBaseBlocks(domainID, hosts, baseBlockSize)
		for i := len(blocks); i < groupSize; i++ {
			blocks = append(blocks, newEmptyBaseBlock(baseBlockSize))
		}

		aggregateNode := &aggregateBlockNode{}
		for _, b := range blocks {
			aggregateNode.nodeCount += baseBlockSize
			aggregateNode.children = append(aggregateNode.children, b)
		}

		blockNodes = append(blockNodes, aggregateNode)
	}

	return blockNodes
}

func packAggregateNodes(nodesMap map[string][]blockTreeNode, completed []int, groupSize int) ([]blockTreeNode, int) {
	if groupSize <= 0 || len(completed) == 0 {
		return nil, 0
	}
	nodeIDs := slices.Sorted(maps.Keys(nodesMap))
	if len(nodeIDs) == 0 {
		return nil, 0
	}
	aggregateNodes := make([]blockTreeNode, 0, len(nodeIDs))
	total := 0
	for _, nodeID := range nodeIDs {
		children := nodesMap[nodeID]
		blocks := make([]blockTreeNode, 0, (len(children)+groupSize-1)/groupSize)
		for start := 0; start < len(children); start += groupSize {
			end := start + groupSize
			if end > len(children) {
				end = len(children)
			}
			blocks = append(blocks, newAggregateBlock(children[start:end], groupSize, completed))
		}
		// compute this node's capacity only from its blocks
		localCount := 0
		for _, b := range blocks {
			localCount += getNodeCount(b)
		}
		aggregateNodes = append(aggregateNodes, &aggregateBlockNode{children: blocks, nodeCount: localCount})
		total += localCount
	}
	return aggregateNodes, total
}

func newAggregateBlock(children []blockTreeNode, size int, blockSizes []int) blockTreeNode {
	if size <= 0 {
		return nil
	}
	result := make([]blockTreeNode, size)
	count := 0
	for i := 0; i < size; i++ {
		var child blockTreeNode
		if i < len(children) {
			child = children[i]
		} else {
			child = newEmptyAggregateBlock(blockSizes)
		}
		result[i] = child
		switch c := child.(type) {
		case *aggregateBlockNode:
			count += c.nodeCount
		case *baseBlockNode:
			count += c.nodeCount
		}
	}
	return &aggregateBlockNode{children: result, nodeCount: count}
}

func newEmptyAggregateBlock(blockSizes []int) blockTreeNode {
	levels := len(blockSizes)
	if levels <= 0 {
		return &aggregateBlockNode{}
	}
	if levels == 1 {
		return newEmptyBaseBlock(blockSizes[0])
	}
	fanout := blockSizes[levels-1] / blockSizes[levels-2]
	children := make([]blockTreeNode, fanout)
	count := 0
	for i := 0; i < fanout; i++ {
		child := newEmptyAggregateBlock(blockSizes[:levels-1])
		children[i] = child
		count += getNodeCount(child)
	}
	return &aggregateBlockNode{children: children, nodeCount: count}
}

func getRemainingBlocks(blockSizes []int, aggregateSize int) []int {
	for i, bs := range blockSizes {
		if bs > aggregateSize {
			return blockSizes[i:]
		}
	}
	return nil
}

// sortHostsByName sorts hosts alphabetically by HostName for deterministic packing.
func sortHostsByName(hosts []*topology.HostInfo) {
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].HostName < hosts[j].HostName
	})
}

func getNodeCount(tree blockTreeNode) int {
	switch n := tree.(type) {
	case *baseBlockNode:
		return n.nodeCount
	case *aggregateBlockNode:
		return n.nodeCount
	default:
		return 0
	}
}
