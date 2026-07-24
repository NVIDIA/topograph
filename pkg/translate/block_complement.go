/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"strings"

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
//
// In stable-ID mode, block IDs are preserved through padding and the function
// fails if any physical block would be split across multiple base blocks: a
// captured index must identify exactly one emitted base block.
func (nt *NetworkTopology) complementBlocks(blocks []*blockInfo, blockSizes []int) ([]*blockInfo, error) {
	if len(blockSizes) < 1 || nt.domains == nil {
		return blocks, nil
	}

	domains := domainsForBlocks(nt.domains, blocks, nt.stableIDs)
	if len(domains) == 0 {
		return blocks, nil
	}

	klog.Infof("Complementing %d blocks with %d domains into tree shape %v", len(blocks), len(domains), blockSizes)
	byName := blocksByName(blocks)

	actualTree := buildBlockTree(domains, blockSizes, nt.stableIDs)
	if actualTree == nil {
		return blocks, nil
	}
	allSlots := collectBaseBlockSlots(actualTree)

	stableIDs := nt.stableIDs

	if stableIDs {
		// A split physical block would force non-unique IDs; reject it with
		// an actionable message pointing at blockSizes[0].
		for _, bb := range allSlots {
			if strings.Contains(bb.id, "#") {
				return nil, fmt.Errorf("blockHostnameRegex: physical block %q would be split into multiple base blocks (blockSizes[0]=%d is smaller than the block's host count); increase blockSizes[0] to fit each physical block",
					bb.domain, blockSizes[0])
			}
		}
	}

	padCtr := &paddingIDCounter{}
	out := make([]*blockInfo, 0, len(allSlots))
	seenIDs := make(map[string]bool, len(allSlots))
	for i, bb := range allSlots {
		info := baseBlockToBlockInfo(bb, byName, i+1, stableIDs, padCtr)
		if stableIDs && info != nil {
			if seenIDs[info.id] {
				return nil, fmt.Errorf("blockHostnameRegex: duplicate block ID %q emitted; check that no two physical blocks share the same captured index", info.id)
			}
			seenIDs[info.id] = true
		}
		out = append(out, info)
	}
	return out, nil
}

// domainsForBlocks returns a subset of the cluster domain map containing only
// hosts owned by the given partition-local blocks. Nodes owned by another
// partition in the same accelerator domain are excluded.
//
// preserveEmpty keeps hostless domains (from DomainMap.EnsureDomain) so
// stable-ID mode can emit empty declarations for blocks without live pods.
func domainsForBlocks(all topology.DomainMap, blocks []*blockInfo, preserveEmpty bool) topology.DomainMap {
	if all == nil {
		return nil
	}
	local := topology.NewDomainMap()
	for _, b := range blocks {
		domainKey := b.domainName()
		if b == nil || domainKey == "" {
			continue
		}
		hosts, ok := all[domainKey]
		if !ok {
			continue
		}
		if preserveEmpty {
			local.EnsureDomain(domainKey)
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

// blocksByName builds a domainName -> blockInfo index used by
// baseBlockToBlockInfo when a leaf carries only a domain reference.
func blocksByName(blocks []*blockInfo) map[string]*blockInfo {
	m := make(map[string]*blockInfo, len(blocks))
	for _, b := range blocks {
		key := b.domainName()
		if b == nil || key == "" {
			continue
		}
		m[key] = b
	}
	return m
}
