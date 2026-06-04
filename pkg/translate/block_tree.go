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
	id     string
	domain string // primary domain ID, pre-computed from id at construction
	leaves []*hostNode
}

func (*baseBlockNode) blockTreeNode() {}

func (n *baseBlockNode) domainIdentifier() string { return n.domain }

// aggregateBlockNode groups base blocks or other aggregates. An domain with
// multiple base blocks is represented as an aggregate of baseBlockNode children.
type aggregateBlockNode struct {
	id       string
	children []blockTreeNode
}

func (*aggregateBlockNode) blockTreeNode() {}

// buildAggregateShape wraps the recursive shape builder so callers always receive an
// *aggregateBlockNode. When fanouts produces a single base block (no intermediate
// tiers), the base block is wrapped in a one-child aggregate.
func buildAggregateShape(fanouts []int, baseBlockSize int) *aggregateBlockNode {
	node := buildShapeLevel(fanouts, 0, baseBlockSize)
	agg, ok := node.(*aggregateBlockNode)
	if !ok {
		return &aggregateBlockNode{children: []blockTreeNode{node}}
	}
	return agg
}

// buildShapeLevel recurses top-down through the fanout tiers. level == len(fanouts)
// is the base case; it emits an empty base block. Returns blockTreeNode because the
// base case produces *baseBlockNode while all other levels produce *aggregateBlockNode.
func buildShapeLevel(fanouts []int, level int, baseBlockSize int) blockTreeNode {
	if level == len(fanouts) {
		return newEmptyBaseBlock(baseBlockSize)
	}
	count := fanoutAtLevel(level, fanouts)
	children := make([]blockTreeNode, count)
	for i := range children {
		children[i] = buildShapeLevel(fanouts, level+1, baseBlockSize)
	}
	return &aggregateBlockNode{children: children}
}

// totalBaseBlockSlots returns the product of all fanout tiers, which equals the
// total number of base-block leaves in the shaped tree.
func totalBaseBlockSlots(fanouts []int) int {
	n := 1
	for _, f := range fanouts {
		n *= f
	}
	return n
}

// expandFanoutsForCapacity grows the last fanout tier (power-of-two steps) until the
// tree has at least required base-block leaves. Returns fanouts unchanged when it is
// empty or required is non-positive — an empty slice would panic on the index write.
func expandFanoutsForCapacity(fanouts []int, required int) []int {
	if required <= 0 || len(fanouts) == 0 {
		return fanouts
	}
	out := append([]int(nil), fanouts...)
	for totalBaseBlockSlots(out) < required {
		out[len(out)-1] *= 2
	}
	return out
}

// mergeBaseBlocksIntoTree fills tree leaf slots left-to-right from the ordered packed list.
func mergeBaseBlocksIntoTree(tree *aggregateBlockNode, packed []*baseBlockNode) {
	slots := collectBaseBlockSlots(tree)
	for i, bb := range packed {
		if i >= len(slots) {
			break
		}
		*slots[i] = *bb
	}
}

// packDomainsIntoBaseBlocks packs all domain hosts into baseBlockSize-sized blocks.
// Each domain's hosts are split into base blocks independently; no merging across
// domains is performed. When groupSize > 1, each domain's base block count is
// rounded up to the nearest multiple of groupSize by appending empty base blocks, so
// that each domain occupies complete aggregate groups within the tree.
func packDomainsIntoBaseBlocks(domains topology.DomainMap, baseBlockSize, groupSize int) []*baseBlockNode {
	if baseBlockSize <= 0 {
		return nil
	}
	domainIDs := slices.Sorted(maps.Keys(domains))
	if len(domainIDs) == 0 {
		return nil
	}

	var blocks []*baseBlockNode
	for _, domainID := range domainIDs {
		hosts := hostsSorted(domains[domainID])
		baseBlocks := splitIntoBaseBlocks(domainID, hosts, baseBlockSize)
		blocks = append(blocks, baseBlocks...)
		// Pad to a multiple of groupSize so the domain fills complete aggregate groups.
		if groupSize > 1 {
			n := len(baseBlocks)
			padded := ((n + groupSize - 1) / groupSize) * groupSize
			for i := n; i < padded; i++ {
				blocks = append(blocks, newEmptyBaseBlock(baseBlockSize))
			}
		}
	}
	return blocks
}

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
// The returned order is identical to the slot order used by mergeBaseBlocksIntoTree,
// so the slice can be indexed by the same position numbers.
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

// blocksFromShapedTree converts the tree's filled base-block slots to named blockInfo
// records, stopping at required slots so unused trailing capacity is not emitted.
func blocksFromShapedTree(tree *aggregateBlockNode, byName map[string]*blockInfo, required int) []*blockInfo {
	slots := collectBaseBlockSlots(tree)
	out := make([]*blockInfo, 0, required)
	for i, bb := range slots {
		if i >= required {
			break
		}
		out = append(out, baseBlockToBlockInfo(bb, byName, i+1))
	}
	return out
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
	for i, h := range hosts {
		if i >= baseBlockSize {
			break
		}
		leaves[i] = &hostNode{host: h}
	}
	return &baseBlockNode{id: id, domain: extractDomainID(id), leaves: leaves}
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

// sortHostsByName sorts hosts alphabetically by HostName for deterministic packing.
func sortHostsByName(hosts []*topology.HostInfo) {
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].HostName < hosts[j].HostName
	})
}

// fanoutAtLevel returns the child count for the given tree level. fanouts is ordered
// leaf-to-root (bottom-up), but the tree is built top-down, so level 0 reads from the
// end of the slice (the outermost tier) and deeper levels read toward the front.
func fanoutAtLevel(level int, fanouts []int) int {
	idx := len(fanouts) - 1 - level
	if idx < 0 {
		idx = 0
	}
	return fanouts[idx]
}
