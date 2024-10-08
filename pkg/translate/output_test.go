/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package translate

import (
	"bytes"
	"testing"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/stretchr/testify/require"
)

const (
	testConfig1 = `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=Node[201-202],Node205
SwitchName=S3 Nodes=Node[304-306]
`

	testConfig2 = `SwitchName=S1 Switches=S[2-3]
SwitchName=S3 Nodes=Node[304-306]
SwitchName=S2 Nodes=Node[201-202],Node205
`
	shortNameExpectedResult = `# switch.3.1=hpcislandid-1
SwitchName=switch.3.1 Switches=switch.2.[1-2]
# switch.2.1=network-block-1
SwitchName=switch.2.1 Switches=switch.1.1
# switch.2.2=network-block-2
SwitchName=switch.2.2 Switches=switch.1.2
# switch.1.1=local-block-1
SwitchName=switch.1.1 Nodes=node-1
# switch.1.2=local-block-2
SwitchName=switch.1.2 Nodes=node-2
`
)

func TestToSLURM(t *testing.T) {
	v, _ := GetTestSet(false)
	buf := &bytes.Buffer{}
	err := ToSLURM(buf, v)
	require.NoError(t, err)
	switch buf.String() {
	case testConfig1, testConfig2:
		// nop
	default:
		t.Errorf("unexpected result %s", buf.String())
	}
}

func TestToSlurmNameShortener(t *testing.T) {
	v := &common.Vertex{
		Vertices: map[string]*common.Vertex{
			"hpcislandid-1": {
				ID:   "hpcislandid-1",
				Name: "switch.3.1",
				Vertices: map[string]*common.Vertex{
					"network-block-1": {
						ID:   "network-block-1",
						Name: "switch.2.1",
						Vertices: map[string]*common.Vertex{
							"local-block-1": {
								ID:   "local-block-1",
								Name: "switch.1.1",
								Vertices: map[string]*common.Vertex{
									"node-1": {
										ID:   "node-1-id",
										Name: "node-1",
									},
								},
							},
						},
					},
					"network-block-2": {
						ID:   "network-block-2",
						Name: "switch.2.2",
						Vertices: map[string]*common.Vertex{
							"local-block-2": {
								ID:   "local-block-2",
								Name: "switch.1.2",
								Vertices: map[string]*common.Vertex{
									"node-2": {
										ID:   "node-2-id",
										Name: "node-2",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	buf := &bytes.Buffer{}
	err := ToSLURM(buf, v)
	require.NoError(t, err)
	require.Equal(t, shortNameExpectedResult, buf.String())
}

func TestCompress(t *testing.T) {
	testCases := []struct {
		name          string
		input, output []string
	}{
		{
			name:   "Case 1: empty list",
			output: []string{},
		},
		{
			name:   "Case 2: ranges",
			input:  []string{"eos0507", "eos0509", "eos0482", "eos0483", "eos0508", "eos0484"},
			output: []string{"eos0[482-484]", "eos0[507-509]"},
		},
		{
			name:   "Case 3: singles",
			input:  []string{"eos0507", "eos0509", "eos0482"},
			output: []string{"eos0482", "eos0507", "eos0509"},
		},
		{
			name:   "Case 4: mix1",
			input:  []string{"eos0507", "eos0509", "abc", "eos0482", "eos0508"},
			output: []string{"abc", "eos0482", "eos0[507-509]"},
		},
		{
			name:   "Case 5: mix2",
			input:  []string{"eos0507", "eos0509", "abc", "eos0508", "eos0482"},
			output: []string{"abc", "eos0482", "eos0[507-509]"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.output, compress(tc.input))
		})
	}
}

func TestSplit(t *testing.T) {
	testCases := []struct {
		name                  string
		input, prefix, suffix string
	}{
		{
			name: "Case 1: empty string",
		},
		{
			name:   "Case 2: no digits",
			input:  "abc",
			prefix: "abc",
		},
		{
			name:   "Case 3: digits only",
			input:  "12345",
			suffix: "12345",
		},
		{
			name:   "Case 4: digits only, leading zeros",
			input:  "0012345",
			prefix: "00",
			suffix: "12345",
		},
		{
			name:   "Case 5: mix",
			input:  "abc1203045",
			prefix: "abc",
			suffix: "1203045",
		},
		{
			name:   "Case 6: mix, leading zeros",
			input:  "abc01203045",
			prefix: "abc0",
			suffix: "1203045",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prefix, suffix := split(tc.input)
			require.Equal(t, tc.prefix, prefix)
			require.Equal(t, tc.suffix, suffix)
		})
	}
}
