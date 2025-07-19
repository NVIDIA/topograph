/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"errors"
	"strings"

	"github.com/NVIDIA/topograph/internal/cluset"
)

var errNotEnoughFakeNodes = errors.New("not enough fake nodes available")

type fakeNodeConfig struct {
	baseBlockSize int
	index         int
	nodes         []string
}

func getFakeNodeConfig(fakeNodeData string) *fakeNodeConfig {
	return &fakeNodeConfig{
		nodes: cluset.Expand([]string{fakeNodeData}),
		index: 0,
	}
}

// getFreeFakeNodes generates fake nodes names.
func (fnc *fakeNodeConfig) getFreeFakeNodes(count int) (string, error) {
	start := fnc.index
	end := fnc.index + count
	if end > len(fnc.nodes) {
		return "", errNotEnoughFakeNodes
	}
	fnc.index = end
	return strings.Join(cluset.Compact(fnc.nodes[start:end]), ","), nil
}
