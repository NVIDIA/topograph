/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import "github.com/NVIDIA/topograph/pkg/topology"

// groupSizeFromDomains computes how many base blocks a fully-populated accelerator
// occupies, rounded up to the nearest power of two. It finds the maximum accelerator
// host count across all domains, then returns 2^n where 2^n * baseBlockSize is the
// smallest power-of-two multiple of baseBlockSize that is >= maxAcceleratorSize.
// Returns 1 when every accelerator fits within a single base block (no padding needed).
func groupSizeFromDomains(domains topology.DomainMap, baseBlockSize int) int {
	maxNodes := 0
	for _, hosts := range domains {
		if len(hosts) > maxNodes {
			maxNodes = len(hosts)
		}
	}
	groupSize := 1
	capacity := baseBlockSize
	for capacity < maxNodes {
		groupSize *= 2
		capacity *= 2
	}
	return groupSize
}

// complementBlocks builds a block tree shaped by BlockSizes, packs domain hosts into
// it, and returns the flat block list derived from low-level tree nodes.
//
// Only domains for accelerators present in blocks are used so per-partition YAML
// complementing is not masked by domains owned by other partitions in nt.domains.
//
// The group size is derived from the maximum accelerator host count: it is the smallest
// 2^n such that 2^n * baseBlockSize >= maxAcceleratorSize. Each accelerator's base
// block count is then padded to a multiple of that groupSize so every accelerator
// occupies complete aggregate groups within the tree.
func (nt *NetworkTopology) complementBlocks(blocks []*blockInfo, blockSizes []int) []*blockInfo {
	fanouts, ok := fanoutsPerLevel(blockSizes)
	if !ok || nt.domains == nil {
		return blocks
	}

	domains := domainsForBlocks(nt.domains, blocks)
	if len(domains) == 0 {
		return blocks
	}

	byName := blocksByName(blocks)
	baseBlockSize := blockSizes[0]
	groupSize := groupSizeFromDomains(domains, baseBlockSize)
	packed := packDomainsIntoBaseBlocks(domains, baseBlockSize, groupSize)
	expandedFanouts := expandFanoutsForCapacity(fanouts, len(packed))
	tree := buildAggregateShape(expandedFanouts, baseBlockSize)
	mergeBaseBlocksIntoTree(tree, packed)

	// When no padding was added (packed count equals input count), stop at the exact
	// packed count so trailing tree-capacity slots do not falsely trigger complement usage.
	required := len(packed)
	if len(packed) != len(blocks) {
		required = totalBaseBlockSlots(expandedFanouts)
	}

	out := blocksFromShapedTree(tree, byName, required)
	if !shouldUseComplementedBlocks(blocks, out) {
		return blocks
	}
	return out
}

// shouldUseComplementedBlocks reports whether the tree-derived list should replace the
// input. Two cases warrant replacement:
//   - Interior empty slots: the tree has gaps where no accelerator was placed, meaning
//     the shaped output carries structural information the flat input list cannot express.
//   - Count increase: an accelerator had more hosts than baseBlockSize and was split
//     into multiple base blocks, so the output is longer than the input.
//
// A shorter output is never used: domainsForBlocks may skip blocks whose domain is
// absent from the global map, producing fewer packed blocks than the input. Replacing
// the input in that case would silently drop blocks.
func shouldUseComplementedBlocks(input, out []*blockInfo) bool {
	if len(out) < len(input) {
		return false
	}
	if hasEmptyBlockSlots(out) {
		return true
	}
	return len(out) > len(input)
}

// domainsForBlocks returns a subset of the cluster domain map containing only the
// hosts that belong to the given partition-local blocks. For each block it intersects
// the global domain with the block's own node list, so that nodes owned by another
// partition in the same accelerator domain are never included.
func domainsForBlocks(all topology.DomainMap, blocks []*blockInfo) topology.DomainMap {
	if all == nil {
		return nil
	}
	local := topology.NewDomainMap()
	for _, b := range blocks {
		if b == nil || b.name == "" {
			continue
		}
		hosts, ok := all[b.name]
		if !ok {
			continue
		}
		// Restrict to nodes the partition actually owns; a domain may span multiple
		// partitions and the global map holds all of them.
		partitionNodes := make(map[string]struct{}, len(b.nodes))
		for _, n := range b.nodes {
			partitionNodes[n] = struct{}{}
		}
		for _, hi := range hosts {
			if _, owned := partitionNodes[hi.HostName]; !owned {
				continue
			}
			copy := *hi
			local.AddHostInfo(&copy)
		}
	}
	return local
}

// blocksByName builds an accelerator-ID → blockInfo index used by baseBlockToBlockInfo
// to look up node lists when a block's leaves carry only an accelerator reference.
// Nil entries and blocks without a name are skipped, matching the guard in domainsForBlocks.
func blocksByName(blocks []*blockInfo) map[string]*blockInfo {
	m := make(map[string]*blockInfo, len(blocks))
	for _, b := range blocks {
		if b == nil || b.name == "" {
			continue
		}
		m[b.name] = b
	}
	return m
}

// hasEmptyBlockSlots reports whether any interior slot in blocks is empty.
// Trailing empty blocks are not considered complement slots because they arise from
// tree capacity rounding and carry no structural meaning for the scheduler.
// "Trailing" means everything after the last non-empty block; only empty slots
// that appear before that boundary are treated as structural gaps.
func hasEmptyBlockSlots(blocks []*blockInfo) bool {
	lastNonEmpty := -1
	for i := len(blocks) - 1; i >= 0; i-- {
		if !isEmptyBlock(blocks[i]) {
			lastNonEmpty = i
			break
		}
	}
	for i := 0; i < lastNonEmpty; i++ {
		if isEmptyBlock(blocks[i]) {
			return true
		}
	}
	return false
}

// fanoutsPerLevel derives child segment counts from BlockSizes.
// Each ratio BlockSizes[i]/BlockSizes[i-1] must be a power of two greater than 1.
func fanoutsPerLevel(blockSizes []int) ([]int, bool) {
	if len(blockSizes) < 2 {
		return nil, false
	}
	prev := blockSizes[0]
	if prev <= 0 {
		return nil, false
	}
	out := make([]int, 0, len(blockSizes)-1)
	for i := 1; i < len(blockSizes); i++ {
		cur := blockSizes[i]
		if cur <= prev || cur%prev != 0 {
			return nil, false
		}
		ratio := cur / prev
		// Power-of-two check: ratio & (ratio-1) is zero only for powers of two.
		if ratio&(ratio-1) != 0 {
			return nil, false
		}
		out = append(out, ratio)
		prev = cur
	}
	return out, true
}
