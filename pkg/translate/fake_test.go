/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeNodeConfig(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		counts  []int
		outputs []any
	}{
		{
			name:    "Case 1: not enough fake nodes",
			input:   "fake[001-010]",
			counts:  []int{11},
			outputs: []any{errNotEnoughFakeNodes},
		},
		{
			name:    "Case 2: exact fake nodes in one step",
			input:   "fake[12-17]",
			counts:  []int{6},
			outputs: []any{"fake[12-17]"},
		},
		{
			name:    "Case 3: exact fake nodes in two steps",
			input:   "fake[12-17]",
			counts:  []int{4, 2},
			outputs: []any{"fake[12-15]", "fake[16-17]"},
		},
		{
			name:    "Case 4: not enough fake nodes",
			input:   "fake[1-10]",
			counts:  []int{4, 2, 1, 4},
			outputs: []any{"fake[1-4]", "fake[5-6]", "fake7", errNotEnoughFakeNodes},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fnc := getFakeNodeConfig(tc.input)

			for i, count := range tc.counts {
				output := tc.outputs[i]
				nodes, err := fnc.getFreeFakeNodes(count)

				if e, ok := output.(error); ok {
					require.EqualError(t, err, e.Error())
				} else {
					require.NoError(t, err)
					require.Equal(t, output, nodes)
				}
			}
		})
	}
}
