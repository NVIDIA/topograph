/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"io"
	"math"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/cluset"
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

func (nt *NetworkTopology) toBlockTopology(wr io.Writer) error {
	var fnc *fakeNodeConfig
	if len(nt.config.FakeNodePool) != 0 {
		fnc = getFakeNodeConfig(nt.config.FakeNodePool)
	}

	finalBlockSizes := getBlockSize(nt.blocks, nt.config.BlockSizes, fnc != nil)
	if fnc != nil {
		fnc.baseBlockSize = finalBlockSizes[0]
	}

	for _, bInfo := range nt.blocks {
		var comment string
		if len(bInfo.name) != 0 {
			comment = fmt.Sprintf("# %s=%s\n", bInfo.id, bInfo.name)
		}

		outputNodeNames := strings.Join(cluset.Compact(bInfo.nodes), ",")
		if fnc != nil && len(bInfo.nodes) < fnc.baseBlockSize {
			fakeNodeNames, err := fnc.getFreeFakeNodes(fnc.baseBlockSize - len(bInfo.nodes))
			if err != nil {
				return err
			}
			outputNodeNames = fmt.Sprintf("%s,%s", outputNodeNames, fakeNodeNames)
		}

		if _, err := fmt.Fprintf(wr, "%sBlockName=%s Nodes=%s\n", comment, bInfo.id, outputNodeNames); err != nil {
			return err
		}
	}

	bss := make([]string, 0, len(finalBlockSizes))
	for _, bs := range finalBlockSizes {
		bss = append(bss, fmt.Sprintf("%d", bs))
	}

	_, err := fmt.Fprintf(wr, "BlockSizes=%s\n", strings.Join(bss, ","))
	return err
}
