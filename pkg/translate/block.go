/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/httperr"
)

func findMinDomainSize(blocks []*blockInfo) int {
	minDomainSize := -1
	for _, block := range blocks {
		blocklen := len(block.nodes)
		if minDomainSize == -1 || minDomainSize > blocklen {
			minDomainSize = blocklen
		}
	}
	return minDomainSize
}

// getBlockSizes returns the BlockSizes list for Slurm's block topology.
// If requestedBlockSizes is non-empty it is returned unchanged. Otherwise the
// result is [D, 2D, 4D, ..., 2^k*D], where D is the smallest block's node
// count and k = floor(log2(N)) for N blocks: the base size matches the
// smallest accelerator domain and each successive level doubles, up to the
// largest power-of-two multiple that fits the block count.
func getBlockSizes(blocks []*blockInfo, requestedBlockSizes []int) []int {
	if len(requestedBlockSizes) != 0 {
		return requestedBlockSizes
	}
	// get smallest domain size
	minDomainSize := findMinDomainSize(blocks)
	outputbs := []int{minDomainSize}
	maxnumbs := int(math.Log2(float64(len(blocks))))

	for i := 1; i <= maxnumbs; i++ {
		levelblocksize := int(math.Pow(2, float64(i))) * minDomainSize
		outputbs = append(outputbs, levelblocksize)
	}

	return outputbs
}

func (nt *NetworkTopology) toBlockTopology(wr io.Writer, skeletonOnly bool) *httperr.Error {
	blockSizes := getBlockSizes(nt.blocks, nt.config.BlockSizes)

	for _, bInfo := range nt.blocks {
		var comment string
		if len(bInfo.name) != 0 {
			comment = fmt.Sprintf("# %s=%s\n", bInfo.id, bInfo.name)
		}

		outputNodeNames := strings.Join(cluset.Compact(bInfo.nodes), ",")

		var err error
		if skeletonOnly {
			_, err = fmt.Fprintf(wr, "%sBlockName=%s\n", comment, bInfo.id)
		} else {
			_, err = fmt.Fprintf(wr, "%sBlockName=%s Nodes=%s\n", comment, bInfo.id, outputNodeNames)
		}
		if err != nil {
			return httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	bss := make([]string, 0, len(blockSizes))
	for _, bs := range blockSizes {
		bss = append(bss, fmt.Sprintf("%d", bs))
	}

	if _, err := fmt.Fprintf(wr, "BlockSizes=%s\n", strings.Join(bss, ",")); err != nil {
		return httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	return nil
}
