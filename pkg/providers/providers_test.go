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

	"github.com/stretchr/testify/require"
)

func TestParseInstanceOutput(t *testing.T) {
	input := `node1: instance1
node2: instance2
node3: instance3
node4: instance4
`
	expected := map[string]string{"instance1": "node1", "instance2": "node2", "instance3": "node3", "instance4": "node4"}

	output, err := ParseInstanceOutput(bytes.NewBufferString(input))
	require.NoError(t, err)
	require.Equal(t, expected, output)
}

func TestReadFile(t *testing.T) {
	testCases := []struct {
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
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.exists {
				f, err := os.CreateTemp("", "test-*")
				require.NoError(t, err)
				path = f.Name()
				defer func() { _ = os.Remove(path) }()
				defer func() { _ = f.Close() }()
				if len(tc.data) != 0 {
					n, err := f.WriteString(tc.data)
					require.NoError(t, err)
					require.Equal(t, len(tc.data), n)
					err = f.Sync()
					require.NoError(t, err)
				}
			} else {
				path = "/does/not/exist"
			}

			data, err := ReadFile(path)
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.data, data)
			}
		})
	}
}
