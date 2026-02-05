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

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/metrics"
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

// getBlockSize returns blocksize for each possible level.
// Admin provided blocksize is validated and is overriden with default blocksizes if validation fails.
func getBlockSize(blocks []*blockInfo, requestedBlockSizes []int, useFake bool) []int {
	// get smallest domain size
	var minDomainSize int
	if useFake && len(requestedBlockSizes) != 0 {
		minDomainSize = requestedBlockSizes[0]
	} else {
		minDomainSize = findMinDomainSize(blocks)
	}
	maxnumbs := int(math.Log2(float64(len(blocks))))
	outputbs := []int{}

	// validate requested block sizes
	if len(requestedBlockSizes) != 0 {
		// validate minimal block size
		var candidate int
		possiblebs := make(map[int]bool)
		for i, bs := range requestedBlockSizes {
			if i == 0 {
				if bs <= 0 || bs > minDomainSize {
					metrics.AddValidationError("bad admin blockSize")
					klog.Warningf("Overriding admin blockSizes. Planning blockSize %v does not meet criteria, should be > 0 & <= %v.", bs, minDomainSize)
					break
				}
				candidate = bs
				// get possible blocksizes with the planningBS
				for l := 0; l <= maxnumbs; l++ {
					levelblocksize := int(math.Pow(2, float64(l))) * candidate
					possiblebs[levelblocksize] = true
				}
			}

			if _, exists := possiblebs[bs]; !exists {
				metrics.AddValidationError("bad admin blockSize")
				klog.Warningf("Overriding admin blockSizes. BlockSize %v should follow the pattern (2^n) * %v, with n <= %v", bs, candidate, maxnumbs)
				break
			}
			outputbs = append(outputbs, bs)
		}

		if len(outputbs) == len(requestedBlockSizes) {
			return outputbs
		}
	}

	// reset outputbs
	outputbs = []int{minDomainSize}

	for i := 1; i <= maxnumbs; i++ {
		levelblocksize := int(math.Pow(2, float64(i))) * minDomainSize
		outputbs = append(outputbs, levelblocksize)
	}

	return outputbs
}

func (nt *NetworkTopology) toBlockTopology(wr io.Writer) *httperr.Error {
	var fnc *fakeNodeConfig
	if len(nt.config.FakeNodePool) != 0 {
		fnc = getFakeNodeConfig(nt.config.FakeNodePool)
	}

	finalBlockSizes := getBlockSize(nt.blocks, nt.config.BlockSizes, fnc != nil)
	if fnc != nil {
		fnc.baseBlockSize = finalBlockSizes[0]
	}

	dynamicNodeMap := nodeList2map(nt.config.DynamicNodes)
	for _, bInfo := range nt.blocks {
		var comment string
		if len(bInfo.name) != 0 {
			comment = fmt.Sprintf("# %s=%s\n", bInfo.id, bInfo.name)
		}

		static, dynamic := splitNodes(bInfo.nodes, dynamicNodeMap)
		if fnc != nil && len(bInfo.nodes) < fnc.baseBlockSize {
			fakeNodeNames, err := fnc.getFreeFakeNodes(fnc.baseBlockSize - len(bInfo.nodes))
			if err != nil {
				return httperr.NewError(http.StatusBadGateway, err.Error())
			}
			static = fmt.Sprintf("%s,%s", static, fakeNodeNames)
		}

		// append the block line with the names of dynamic nodes, if present
		var suffix string
		if len(dynamic) != 0 {
			suffix = fmt.Sprintf(" # dynamic=%s", dynamic)
		}

		if _, err := fmt.Fprintf(wr, "%sBlockName=%s Nodes=%s%s\n", comment, bInfo.id, static, suffix); err != nil {
			return httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	// add empty blocks if needed
	for i := len(nt.blocks) + 1; i <= nt.config.MinBlocks; i++ {
		if _, err := fmt.Fprintf(wr, "BlockName=block%03d\n", i); err != nil {
			return httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	bss := make([]string, 0, len(finalBlockSizes))
	for _, bs := range finalBlockSizes {
		bss = append(bss, fmt.Sprintf("%d", bs))
	}

	if _, err := fmt.Fprintf(wr, "BlockSizes=%s\n", strings.Join(bss, ",")); err != nil {
		return httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	return nil
}
