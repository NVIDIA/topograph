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
	"strings"

	"github.com/NVIDIA/topograph/pkg/topology"
	"k8s.io/klog/v2"
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
		klog.V(4).Infof("buildBlockTree: skipping — invalid blockSizes %v", blockSizes)
		return nil
	}
	klog.V(4).Infof("buildBlockTree: building tree for %d domain(s) with blockSizes=%v", len(domains), blockSizes)
	root := domains.GetDomainTree()
	result := toRootAggregate(root, blockSizes)
	if result == nil {
		klog.V(4).Infof("buildBlockTree: result is empty (no domains or no hosts)")
	} else {
		klog.V(4).Infof("buildBlockTree: done, root nodeCount=%d", result.nodeCount)
	}
	return result
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

// toDomainAggregate converts a BlockVertex into a flat aggregateBlockNode whose
// slot capacity is determined by the maximum actualNodeCount among the vertex's
// siblings (maxSiblingNodes).
//
// Slot capacity: nodeCount = aggregateSlotCapacity(maxSiblingNodes, blockSizes[0]).
// Base block count: numBaseBlocks = nodeCount / blockSizes[0].
//
// Base blocks are filled by one of three strategies:
//
//  1. Leaf (src.Hosts != nil): split hosts with splitIntoBaseBlocks and pad
//     to numBaseBlocks with empty base blocks.
//
//  2. Interior, children fill at most half a base block (MaxChildNodeCount ≤ blockSizes[0]/2):
//     collect all hosts from every leaf child, sort them across sub-domain boundaries,
//     split into base blocks, and pad to numBaseBlocks. This avoids wasting empty host
//     slots when sub-domains are small enough to share a base block.
//
//  3. Interior, children are larger than half a base block (MaxChildNodeCount > blockSizes[0]/2):
//     call toDomainAggregate recursively per child using child.MaxChildNodeCount() as
//     maxSiblingNodes, flatten the resulting base blocks into this aggregate, and pad
//     to numBaseBlocks.
func toDomainAggregate(src *topology.BlockVertex, maxSiblingNodes int, blockSizes []int) *aggregateBlockNode {
	if src == nil || len(blockSizes) == 0 {
		return nil
	}
	baseBlockSize := blockSizes[0]
	lastBlockSize := blockSizes[len(blockSizes)-1]
	// When the max sibling count exceeds the last block size the slot is capped to the
	// domain's own ActualNodeCount. This prevents a single large domain from inflating
	// every sibling's slot to an impractical size when blockSizes is under-configured.
	//
	// Otherwise all siblings use the same maxSiblingNodes so that toRootAggregate's
	// childCapacity is uniform and empty padding aggregates are correctly sized.
	var nodeCount int
	if lastBlockSize < maxSiblingNodes {
		nodeCount = aggregateSlotCapacity(src.ActualNodeCount, baseBlockSize)
	} else {
		nodeCount = aggregateSlotCapacity(maxSiblingNodes, baseBlockSize)
	}
	numBaseBlocks := nodeCount / baseBlockSize
	if numBaseBlocks < 1 {
		numBaseBlocks = 1
	}

	// Strategy 1: leaf vertex — split hosts directly into base blocks.
	if src.Hosts != nil {
		return packHostsIntoAggregate(src.ID, hostsSorted(src.Hosts), numBaseBlocks, baseBlockSize)
	}

	// Strategy 2: children fill less than or equal to half a base block — combine their hosts.
	if src.MaxChildNodeCount > 0 && src.MaxChildNodeCount <= baseBlockSize/2 {
		return combineChildHostsIntoAggregate(src, numBaseBlocks, baseBlockSize)
	}

	// Strategy 3: children are at least base-block sized — recurse per child.
	return recurseChildrenIntoAggregate(src, numBaseBlocks, blockSizes)
}

// toRootAggregate builds the root-level aggregateBlockNode by calling toDomainAggregate on
// each domain child of root and appending empty domain aggregates until the total
// nodeCount reaches a positive multiple of blockSizes[last].
//
// The slot capacity for each domain is determined by root.MaxChildNodeCount() so that
// every domain aggregate occupies the same number of base blocks. Empty domain
// aggregates are sized to match that same per-domain capacity.
func toRootAggregate(root *topology.BlockVertex, blockSizes []int) *aggregateBlockNode {
	if root == nil || len(blockSizes) == 0 || blockSizes[0] <= 0 {
		return nil
	}
	baseBlockSize := blockSizes[0]
	rootDesired := blockSizes[len(blockSizes)-1]

	result := &aggregateBlockNode{id: root.ID}
	childCapacity := 0
	for _, name := range slices.Sorted(maps.Keys(root.Children)) {
		domain := root.ChildAt(name)
		domainAgg := toDomainAggregate(domain, root.MaxChildNodeCount, blockSizes)
		if domainAgg == nil {
			continue
		}
		result.children = append(result.children, domainAgg)
		result.nodeCount += domainAgg.nodeCount
		if childCapacity == 0 {
			childCapacity = domainAgg.nodeCount
		} else {
			childCapacity = blockTreeGCD(childCapacity, domainAgg.nodeCount)
		}
	}

	// Pad with empty domain aggregates to reach the nearest positive multiple of
	// blockSizes[last]. The LCM-based step ensures correctness when childCapacity
	// does not evenly divide rootDesired (non-power-of-2-aligned block size pairs).
	if result.nodeCount > 0 && childCapacity > 0 && result.nodeCount%rootDesired != 0 {
		g := blockTreeGCD(childCapacity, rootDesired)
		step := childCapacity / g * rootDesired // = lcm(childCapacity, rootDesired)
		targetCount := ((result.nodeCount + step - 1) / step) * step
		for result.nodeCount < targetCount {
			result.children = append(result.children, newEmptyChildAggregate(childCapacity, baseBlockSize))
			result.nodeCount += childCapacity
		}
		klog.V(4).Infof("toRootTree: padded to nodeCount=%d (rootDesired=%d)", result.nodeCount, rootDesired)
	}

	return result
}

// aggregateSlotCapacity returns the smallest power-of-2 multiple of baseBlockSize
// that is >= maxSiblingNodes, which is the slot nodeCount for a vertex sized by its
// sibling maximum. When maxSiblingNodes is zero or negative it falls back to one slot.
func aggregateSlotCapacity(maxSiblingNodes, baseBlockSize int) int {
	capacity := baseBlockSize
	for capacity < maxSiblingNodes {
		capacity *= 2
	}
	return capacity
}

// packHostsIntoAggregate splits sortedHosts into baseBlockSize-wide base blocks,
// pads the list to numBaseBlocks with empty base blocks, and wraps the result in an
// aggregateBlockNode. It is the common leaf-packing routine shared by strategies 1 and 2.
func packHostsIntoAggregate(id string, sortedHosts []*topology.HostInfo, numBaseBlocks, baseBlockSize int) *aggregateBlockNode {
	blocks := splitIntoBaseBlocks(id, sortedHosts, baseBlockSize)
	for len(blocks) < numBaseBlocks {
		blocks = append(blocks, newEmptyBaseBlock(baseBlockSize))
	}
	agg := &aggregateBlockNode{id: id}
	for _, b := range blocks {
		agg.children = append(agg.children, b)
		agg.nodeCount += baseBlockSize
	}
	return agg
}

// combineChildHostsIntoAggregate implements strategy 2: it iterates over children
// sorted by name, accumulates their hosts along with the sub-domain names, and flushes
// a base block whenever the running host count reaches baseBlockSize. Each flushed block
// is named with the "+" delimited sub-domains that contributed to it, built inline.
// Any remaining hosts at the end form a partial base block. The result is padded to
// numBaseBlocks with empty base blocks.
func combineChildHostsIntoAggregate(src *topology.BlockVertex, numBaseBlocks, baseBlockSize int) *aggregateBlockNode {
	var blocks []*baseBlockNode
	var pendingHosts []*topology.HostInfo
	var pendingNames []string

	for _, sdName := range slices.Sorted(maps.Keys(src.Children)) {
		child := src.ChildAt(sdName)

		// Collect and sort this child's hosts for deterministic ordering.
		childHosts := make([]*topology.HostInfo, 0, len(child.Hosts))
		for _, h := range child.Hosts {
			childHosts = append(childHosts, h)
		}
		sortHostsByName(childHosts)

		// Flush pending hosts into a base block once their count exceeds half the base block size.
		if len(pendingHosts) > baseBlockSize/2 {
			blockName := strings.Join(pendingNames, "+")
			bb := newBaseBlock(blockName, pendingHosts, baseBlockSize)
			bb.domain = blockName
			blocks = append(blocks, bb)

			//reset the pending hosts and names for the next block
			pendingHosts, pendingNames = nil, nil
		}

		pendingHosts = append(pendingHosts, childHosts...)
		pendingNames = append(pendingNames, sdName)
	}

	//Add the last partial block if any hosts remain
	if len(pendingHosts) > 0 {
		blockName := strings.Join(pendingNames, "+")
		bb := newBaseBlock(blockName, pendingHosts, baseBlockSize)
		bb.domain = blockName
		blocks = append(blocks, bb)
	}

	//Pad the list of blocks to numBaseBlocks with empty base blocks
	for len(blocks) < numBaseBlocks {
		blocks = append(blocks, newEmptyBaseBlock(baseBlockSize))
	}

	//Wrap all base blocks into an aggregate block node and return.
	agg := &aggregateBlockNode{id: src.ID}
	for _, b := range blocks {
		agg.children = append(agg.children, b)
		agg.nodeCount += baseBlockSize
	}
	return agg
}

// recurseChildrenIntoAggregate implements strategy 3: it calls toDomainAggregate on
// each child of src using src.MaxChildNodeCount as the sibling-max, collects all
// base blocks produced by those recursive calls (via collectBaseBlockSlots), and pads
// the flat result to numBaseBlocks with empty base blocks.
func recurseChildrenIntoAggregate(src *topology.BlockVertex, numBaseBlocks int, blockSizes []int) *aggregateBlockNode {
	baseBlockSize := blockSizes[0]
	childMax := src.MaxChildNodeCount
	agg := &aggregateBlockNode{id: src.ID}
	for _, name := range slices.Sorted(maps.Keys(src.Children)) {
		child := src.ChildAt(name)
		childAgg := toDomainAggregate(child, childMax, blockSizes)
		if childAgg == nil {
			continue
		}
		for _, bb := range collectBaseBlockSlots(childAgg) {
			agg.children = append(agg.children, bb)
			agg.nodeCount += baseBlockSize
		}
	}
	for agg.nodeCount/baseBlockSize < numBaseBlocks {
		agg.children = append(agg.children, newEmptyBaseBlock(baseBlockSize))
		agg.nodeCount += baseBlockSize
	}
	return agg
}

// blockTreeGCD returns the greatest common divisor of two positive integers.
func blockTreeGCD(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// sortHostsByName sorts hosts alphabetically by HostName for deterministic packing.
func sortHostsByName(hosts []*topology.HostInfo) {
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].HostName < hosts[j].HostName
	})
}
