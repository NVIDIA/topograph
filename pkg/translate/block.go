/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"math"

	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
	"k8s.io/klog/v2"
)

func findMinDomainSize(blockRoot *topology.Vertex) int {
	minDomainSize := -1
	for _, block := range blockRoot.Vertices {
		blocklen := len(block.Vertices)
		if minDomainSize == -1 || minDomainSize > blocklen {
			minDomainSize = blocklen
		}
	}
	return minDomainSize
}

// getBlockSize returns blocksize for qeach possible level.
// Admin provided blocksize is validated and is overriden with default blocksizes if validation fails.
func getBlockSize(blockRoot *topology.Vertex, requestedBlockSizes []int) ([]int, error) {
	// get smallest domain size
	minDomainSize := findMinDomainSize(blockRoot)

	maxnumbs := int(math.Log2(float64(len(blockRoot.Vertices))))
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
			return outputbs, nil
		}
	}

	outputbs = nil

	bs := minDomainSize
	outputbs = append(outputbs, bs)

	for i := 1; i <= maxnumbs; i++ {
		levelblocksize := int(math.Pow(2, float64(i))) * bs
		outputbs = append(outputbs, levelblocksize)
	}

	return outputbs, nil
}

/*
func (nt *NetworkTopology) toBlockTopology(wr io.Writer) error {
	// traverse tree topology in DFS manner and when a node is reached, check within blockRoot for domain and print that domain.
	// keep a map of which domain has been printed

	visited := make(map[string]bool)
	domainVisited := make(map[string]int)

	var fnc *fakeNodeConfig
	var err error
	if len(nt.config.FakeNodePool) != 0 {
		fnc = getFakeNodeConfig(nt.config.FakeNodePool)
	}

	finalBlockSizes, err := getBlockSize(nt.blocks, nt.config.BlockSizes)
	if err != nil {
		return err
	}
	if fnc != nil {
		fnc.baseBlockSize = finalBlockSizes[0]
	}

	// print blocks
	if treeRoot != nil {
		err = dfsTraversal(wr, treeRoot, blockRoot, visited, domainVisited, fnc)
		if err != nil {
			return err
		}
	}
	err = printDisconnectedBlocks(wr, nt.blocks, domainVisited, fnc)
	if err != nil {
		return err
	}

	bss := make([]string, 0, len(finalBlockSizes))
	for _, bs := range finalBlockSizes {
		bss = append(bss, fmt.Sprintf("%d", bs))
	}

	_, err = fmt.Fprintf(wr, "BlockSizes=%s\n", strings.Join(bss, ","))

	return err
}
*/
