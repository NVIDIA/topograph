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

package providers

import (
	"bytes"
	"os"
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestParsePdshOutput(t *testing.T) {
	input := `node1: instance1
node2: instance2
node3: instance3
node4: instance4
`
	expected := map[string]string{"instance1": "node1", "instance2": "node2", "instance3": "node3", "instance4": "node4"}

	output, err := ParseInstanceOutput(bytes.NewBufferString(input))
	require.NoError(t, err)
	require.Equal(t, expected, output)

	expected = map[string]string{"node1": "instance1", "node2": "instance2", "node3": "instance3", "node4": "instance4"}

	output, err = ParsePdshOutput(bytes.NewBufferString(input), true)
	require.NoError(t, err)
	require.Equal(t, expected, output)
}

func TestReadFile(t *testing.T) {
	tests := []struct {
		name   string
		exists bool
		data   string
		err    bool
	}{
		{
			name: "Case 1: file does not exist",
			err:  true,
		},
		{
			name:   "Case 2: empty file",
			exists: true,
		},
		{
			name:   "Case 3: text file",
			exists: true,
			data: `line1
line2`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.exists {
				f, err := os.CreateTemp("", "test-*")
				require.NoError(t, err)
				path = f.Name()
				defer func() { _ = os.Remove(path) }()
				defer func() { _ = f.Close() }()
				if len(tt.data) != 0 {
					n, err := f.WriteString(tt.data)
					require.NoError(t, err)
					require.Equal(t, len(tt.data), n)
					err = f.Sync()
					require.NoError(t, err)
				}
			} else {
				path = "/does/not/exist"
			}

			data, err := ReadFile(path)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.data, data)
			}
		})
	}
}

func TestFromMap(t *testing.T) {
	m := map[string]any{
		"name":  "switch01",
		"count": 3,
		"nilv":  nil,
	}

	tests := []struct {
		name   string
		key    string
		must   bool
		expect any
		err    string
		testFn func(string, map[string]any, bool) (any, error)
	}{
		{
			name:   "Case 1: string success",
			key:    "name",
			must:   true,
			expect: "switch01",
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[string](k, m, must)
			},
		},
		{
			name:   "Case 2: int success",
			key:    "count",
			must:   true,
			expect: 3,
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[int](k, m, must)
			},
		},
		{
			name:   "Case 3: missing optional",
			key:    "missing",
			must:   false,
			expect: "",
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[string](k, m, must)
			},
		},
		{
			name: "Case 4: missing required",
			key:  "missing",
			must: true,
			err:  "missing 'missing'",
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[string](k, m, must)
			},
		},
		{
			name: "Case 5: wrong type",
			key:  "name",
			must: true,
			err:  "'name' must be of type int",
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[int](k, m, must)
			},
		},
		{
			name: "Case 6: nil required",
			key:  "nilv",
			must: true,
			err:  "missing 'nilv'",
			testFn: func(k string, m map[string]any, must bool) (any, error) {
				return FromMap[string](k, m, must)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := tt.testFn(tt.key, m, tt.must)

			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expect, v)
			}
		})
	}
}

func TestGetTrimTiers(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		expected int
		err      string
	}{
		{
			name:   "Case 1: missing key",
			params: map[string]any{},
		},
		{
			name: "Case 2: nil value",
			params: map[string]any{
				topology.KeyTrimTiers: nil,
			},
		},
		{
			name: "Case 3: int value",
			params: map[string]any{
				topology.KeyTrimTiers: 1,
			},
			expected: 1,
		},
		{
			name: "Case 4: float64 value",
			params: map[string]any{
				topology.KeyTrimTiers: float64(2),
			},
			expected: 2,
		},
		{
			name: "Case 5: negative value",
			params: map[string]any{
				topology.KeyTrimTiers: -1,
			},
			err: "invalid 'trimTiers' value '-1': must be an integer between 0 and 2",
		},
		{
			name: "Case 6: value greater than 2",
			params: map[string]any{
				topology.KeyTrimTiers: 3,
			},
			err: "invalid 'trimTiers' value '3': must be an integer between 0 and 2",
		},
		{
			name: "Case 7: unsupported type",
			params: map[string]any{
				topology.KeyTrimTiers: "1",
			},
			expected: 0,
			err:      "invalid 'trimTiers' value '1': unsupported type string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTrimTiers(tt.params)

			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
