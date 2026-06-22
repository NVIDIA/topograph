/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"github.com/NVIDIA/topograph/pkg/topology"
	"k8s.io/klog/v2"
)

// groupSizeFromDomains computes how many base blocks a fully-populated accelerator
// occupies, rounded up to the nearest power of two. It finds the maximum accelerator
// host count across all domains, then returns 2^n where 2^n * baseBlockSize is the
// smallest power-of-two multiple of baseBlockSize that is >= maxAcceleratorSize.
//
// When there is only a single block tier, no aggregate-level group alignment is
// required and the group size remains 1.
func groupSizeFromDomains(domains topology.DomainMap, baseBlockSize, lastBlockSize int) int {
	if lastBlockSize == baseBlockSize {
		return 1
	}

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
// occupies complete aggregate groups within the tree. Aggregate nodes whose total leaf
// count already satisfies blockSizes[last] or 2^n * blockSizes[last] are left unpadded.
func (nt *NetworkTopology) complementBlocks(blocks []*blockInfo, blockSizes []int) []*blockInfo {
	if len(blockSizes) < 1 || nt.domains == nil {
		return blocks
	}

	domains := domainsForBlocks(nt.domains, blocks)
	if len(domains) == 0 {
		return blocks
	}

	klog.Infof("Complementing %d blocks with %d domains into tree shape %v", len(blocks), len(domains), blockSizes)
	byName := blocksByName(blocks)

	actualTree := buildBlockTree(domains, blockSizes)
	if actualTree == nil {
		return blocks
	}
	allSlots := collectBaseBlockSlots(actualTree)

	out := make([]*blockInfo, 0, len(allSlots))
	for i, bb := range allSlots {
		out = append(out, baseBlockToBlockInfo(bb, byName, i+1))
	}
	return out
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
