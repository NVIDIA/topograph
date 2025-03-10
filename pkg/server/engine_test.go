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

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckCredentials(t *testing.T) {
	credPayload := map[string]string{"key1": "val1"}
	credConfig := map[string]string{"key2": "val2"}

	testCases := []struct {
		name     string
		payload  map[string]string
		config   map[string]string
		expected map[string]string
	}{
		{
			name:     "Case 1: payload only",
			payload:  credPayload,
			expected: credPayload,
		},
		{
			name:     "Case 2: config only",
			config:   credConfig,
			expected: credConfig,
		},
		{
			name:     "Case 3: both",
			payload:  credPayload,
			config:   credConfig,
			expected: credPayload,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, checkCredentials(tc.payload, tc.config))
		})
	}

}
