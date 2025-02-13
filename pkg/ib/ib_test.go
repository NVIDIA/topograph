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

package ib

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/maps"
)

func TestSimplifyTreeDupAt3(t *testing.T) {
	input := &Switch{
		ID:   "1",
		Name: "Switch1",
		Conn: make(map[string]string),
		Children: map[string]*Switch{
			"2": {
				ID:      "2",
				Name:    "Switch2",
				Parents: map[string]bool{"1": true},
				Conn:    make(map[string]string),
				Children: map[string]*Switch{
					"4": {
						ID:   "4",
						Name: "Switch4",
						Nodes: map[string]string{
							"8": "HCA8",
							"9": "HCA9",
						},
						Conn:    make(map[string]string),
						Parents: map[string]bool{"2": true},
					},
					"5": {
						ID:   "5",
						Name: "Switch5",
						Nodes: map[string]string{
							"8": "HCA8",
							"9": "HCA9",
						},
						Conn:    make(map[string]string),
						Parents: map[string]bool{"2": true},
					},
				},
			},
			"3": {
				ID:   "3",
				Name: "Switch3",
				Children: map[string]*Switch{
					"4": {
						ID:   "4",
						Name: "Switch4",
						Nodes: map[string]string{
							"8": "HCA8",
							"9": "HCA9",
						},
						Conn: make(map[string]string),
						Parents: map[string]bool{
							"3": true,
						},
					},
					"5": {
						ID:   "5",
						Name: "Switch5",
						Nodes: map[string]string{
							"8": "HCA8",
							"9": "HCA9",
						},
						Conn:    make(map[string]string),
						Parents: map[string]bool{"3": true},
					},
				},
			},
		},
	}

	seen = make(map[int]map[string]*Switch)
	input.simplify(input.getHeight())

	assert.Equal(t, 1, len(input.Children), "Root should have only one child")

	child := maps.Keys(input.Children)[0]
	assert.Equal(t, 1, len(input.Children[child].Children), "Root's child should have only one child")

	grandchild := maps.Keys(input.Children[child].Children)[0]
	assert.Equal(t, 2, len(input.Children[child].Children[grandchild].Nodes), "there should be 2 leaf nodes")
}

func TestSimplifyTreeDupAt2(t *testing.T) {
	input := &Switch{
		ID:   "1",
		Name: "Switch1",
		Conn: make(map[string]string),
		Children: map[string]*Switch{
			"2": {
				ID:      "2",
				Name:    "Switch2",
				Parents: map[string]bool{"1": true},
				Conn:    make(map[string]string),
				Nodes: map[string]string{
					"8": "HCA8",
					"9": "HCA9",
					"4": "HCA4",
					"5": "HCA5",
				},
				Children: make(map[string]*Switch),
			},
			"3": {
				ID:   "3",
				Name: "Switch3",
				Nodes: map[string]string{
					"8": "HCA8",
					"9": "HCA9",
					"4": "HCA4",
					"5": "HCA5",
				},
				Conn:     make(map[string]string),
				Parents:  map[string]bool{"1": true},
				Children: make(map[string]*Switch),
			},
		},
	}

	seen = make(map[int]map[string]*Switch)
	input.simplify(input.getHeight())

	assert.Equal(t, 1, len(input.Children), "Root should have only one child")

	child := maps.Keys(input.Children)[0]
	assert.Equal(t, 4, len(input.Children[child].Nodes), "there should be 4 leaf nodes")
}

func TestSwitch_GetHeight(t *testing.T) {
	testCases := []struct {
		Name     string
		Input    *Switch
		Expected int
	}{
		{
			Name: "Multiple levels of children",
			Input: &Switch{
				ID:   "1",
				Name: "Switch1",
				Children: map[string]*Switch{
					"2": {
						ID:   "2",
						Name: "Switch2",
						Children: map[string]*Switch{
							"3": {
								ID:   "3",
								Name: "Switch3",
								Children: map[string]*Switch{
									"4": {
										ID:   "4",
										Name: "Switch4",
										Children: map[string]*Switch{
											"5": {
												ID:   "5",
												Name: "Switch5",
												Children: map[string]*Switch{
													"6": {
														ID:   "6",
														Name: "Switch6",
														Children: map[string]*Switch{
															"7": {
																ID:   "7",
																Name: "Switch7",
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Expected: 6,
		},
		{
			Name: "No children",
			Input: &Switch{
				ID:   "1",
				Name: "Switch1",
			},
			Expected: 0,
		},
	}

	for _, tc := range testCases {
		actualHeight := tc.Input.getHeight()
		assert.Equal(t, tc.Expected, actualHeight, tc.Name)
	}
}

func TestParseIbnetdiscoverFile(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedSw  map[string]*Switch
		expectedHCA map[string]string
		expectedErr bool
	}{
		{
			name:        "Empty file",
			input:       []byte(""),
			expectedSw:  map[string]*Switch{},
			expectedHCA: map[string]string{},
			expectedErr: false,
		},
		{
			name: "Single CA",
			input: []byte(`
Ca	1 "H-043f720300f4bc9e"		# "ngcprd10-luna3086 mlx5_3"
					`),
			expectedSw:  map[string]*Switch{},
			expectedHCA: map[string]string{"H-043f720300f4bc9e": "ngcprd10-luna3086"},
			expectedErr: false,
		},
		{
			name: "Single Switch",
			input: []byte(`
Switch  1   "Switch-1"         # "switch1"
					`),
			expectedSw: map[string]*Switch{
				"Switch-1": {
					ID:       "Switch-1",
					Name:     "switch1",
					Conn:     make(map[string]string),
					Parents:  make(map[string]bool),
					Children: make(map[string]*Switch),
					Nodes:    make(map[string]string),
				},
			},
			expectedHCA: map[string]string{},
			expectedErr: false,
		},
		{
			name: "Switch with connections",
			input: []byte(`
Switch	41 "S-08c0eb03008cc87c"		# "MF0;IB-ComputeSpine-03:MQM8700/U1" enhanced port 0 lid 304 lmc 0
[1]	"S-08c0eb0300539a5c"[23]		# "MF0;IB-ComputeLeaf-101:MQM8700/U1" lid 1 4xHDR
[2]	"S-08c0eb0300539a9c"[23]		# "MF0;IB-ComputeLeaf-102:MQM8700/U1" lid 383 

`),
			expectedSw: map[string]*Switch{
				"S-08c0eb03008cc87c": {
					ID:       "S-08c0eb03008cc87c",
					Name:     "IB-ComputeSpine-03",
					Parents:  make(map[string]bool),
					Children: make(map[string]*Switch),
					Nodes:    make(map[string]string),
					Conn: map[string]string{
						"S-08c0eb0300539a5c": "MF0;IB-ComputeLeaf-101:MQM8700/U1",
						"S-08c0eb0300539a9c": "MF0;IB-ComputeLeaf-102:MQM8700/U1",
					},
				},
			},
			expectedHCA: map[string]string{},
			expectedErr: false,
		},
		{
			name: "Multiple entries",
			input: []byte(`
Switch	41 "S-08c0eb03008cc87c"		# "MF0;IB-ComputeSpine-03:MQM8700/U1" enhanced port 0 lid 304 lmc 0
[1]	"S-08c0eb0300539a5c"[23]		# "MF0;IB-ComputeLeaf-101:MQM8700/U1" lid 1 4xHDR
[2]	"S-08c0eb0300539a9c"[23]		# "MF0;IB-ComputeLeaf-102:MQM8700/U1" lid 383 

Ca	1 "H-043f720300f4bc9e"		# "ngcprd10-luna3086 mlx5_3"

Switch	41 "S-b8cef603008032b8"		# "MF0;SJC3-A04-IB-ComputeLeaf-007:MQM8700/U1" enhanced port 0 lid 2382 lmc 0
[21]	"S-08c0eb03008ccc3c"[39]		# "MF0;IB-ComputeSpine-01:MQM8700/U1" lid 159 4xHDR
[22]	"S-08c0eb03008ccb5c"[39]		# "MF0;IB-ComputeSpine-02:MQM8700/U1" lid 230 4xHDR
[23]	"S-08c0eb03008cc87c"[39]		# "MF0;IB-ComputeSpine-03:MQM8700/U1" lid 304 4xHDR
			`),
			expectedSw: map[string]*Switch{
				"S-08c0eb03008cc87c": {
					ID:       "S-08c0eb03008cc87c",
					Name:     "IB-ComputeSpine-03",
					Parents:  make(map[string]bool),
					Children: make(map[string]*Switch),
					Nodes:    make(map[string]string),
					Conn: map[string]string{
						"S-08c0eb0300539a5c": "MF0;IB-ComputeLeaf-101:MQM8700/U1",
						"S-08c0eb0300539a9c": "MF0;IB-ComputeLeaf-102:MQM8700/U1",
					},
				},
				"S-b8cef603008032b8": {
					ID:   "S-b8cef603008032b8",
					Name: "SJC3-A04-IB-ComputeLeaf-007",
					Conn: map[string]string{
						"S-08c0eb03008ccc3c": "MF0;IB-ComputeSpine-01:MQM8700/U1",
						"S-08c0eb03008ccb5c": "MF0;IB-ComputeSpine-02:MQM8700/U1",
						"S-08c0eb03008cc87c": "MF0;IB-ComputeSpine-03:MQM8700/U1",
					},
					Parents:  make(map[string]bool),
					Children: make(map[string]*Switch),
					Nodes:    make(map[string]string),
				},
			},
			expectedHCA: map[string]string{"H-043f720300f4bc9e": "ngcprd10-luna3086"},
			expectedErr: false,
		},
		{
			name: "Invalid format",
			input: []byte(`
		Invalid entry
					`),
			expectedSw:  map[string]*Switch{},
			expectedHCA: map[string]string{},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switches, hca, err := ParseIbnetdiscoverFile(tt.input)
			if (err != nil) != tt.expectedErr {
				t.Fatalf("expected error: %v, got: %v", tt.expectedErr, err)
			}

			for key, expectedSwitch := range tt.expectedSw {
				if actualSwitch, ok := switches[key]; ok {
					if !reflect.DeepEqual(*actualSwitch, *expectedSwitch) {
						t.Errorf("expected switch for key %v: %v, got: %v", key, expectedSwitch, actualSwitch)
					}
				} else {
					t.Errorf("expected switch with key %s not found", key)
				}
			}

			if !reflect.DeepEqual(hca, tt.expectedHCA) {
				t.Errorf("expected hca: %v, got: %v", tt.expectedHCA, hca)
			}
		})
	}
}

func TestBuildTree(t *testing.T) {
	// Simulate switches and HCAs
	switches := map[string]*Switch{
		"1": {
			ID:   "1",
			Name: "Switch1",
			Conn: map[string]string{
				"2": "Switch2",
				"3": "Switch3",
			},
			Nodes:    make(map[string]string),
			Parents:  make(map[string]bool),
			Children: make(map[string]*Switch),
		},
		"2": {
			ID:   "2",
			Name: "Switch2",
			Conn: map[string]string{
				"hca1": "HCA1",
				"hca2": "HCA2",
			},
			Nodes:    make(map[string]string),
			Parents:  make(map[string]bool),
			Children: make(map[string]*Switch),
		},
		"3": {
			ID:   "3",
			Name: "Switch3",
			Conn: map[string]string{
				"hca3": "HCA3",
			},
			Nodes:    make(map[string]string),
			Parents:  make(map[string]bool),
			Children: make(map[string]*Switch),
		},
	}

	hca := map[string]string{
		"hca1": "HCA1",
		"hca2": "HCA2",
		"hca3": "HCA3",
	}

	expectedTree := &Switch{
		ID:      "1",
		Name:    "Switch1",
		Parents: make(map[string]bool),
		Nodes:   make(map[string]string),
		Children: map[string]*Switch{
			"2": {
				ID:   "2",
				Name: "Switch2",
				Nodes: map[string]string{
					"hca1": "HCA1",
					"hca2": "HCA2",
				},
				Children: make(map[string]*Switch),
				Parents:  map[string]bool{"1": true},
			},
			"3": {
				ID:   "3",
				Name: "Switch3",
				Nodes: map[string]string{
					"hca3": "HCA3",
				},
				Children: make(map[string]*Switch),
				Parents:  map[string]bool{"1": true},
			},
		},
	}

	nodesInCluster := map[string]bool{
		"HCA1": true,
		"HCA2": true,
		"HCA3": true,
	}
	tree, err := buildTree(switches, hca, nodesInCluster)

	expectedRoot := &Switch{
		Children: map[string]*Switch{
			expectedTree.ID: expectedTree,
		},
	}
	assert.NoError(t, err, "Building tree should not return an error")
	assert.Equal(t, expectedRoot, tree, "Built tree should match the expected tree")
}
