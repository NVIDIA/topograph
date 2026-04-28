/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package sim

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYAMLModelsGenerateConsistentJSON(t *testing.T) {
	t.Parallel()
	stems := []string{"small-tree", "medium", "large", "nvl72"}
	for _, stem := range stems {
		t.Run(stem, func(t *testing.T) {
			t.Parallel()
			path := stem + ".yaml"
			a, err := responseBytesFromModelFile(path)
			require.NoError(t, err)
			b, err := responseBytesFromModelFile(path)
			require.NoError(t, err)
			require.Equal(t, a, b)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(a, &doc))
			require.Contains(t, doc, "switches")
		})
	}
}

func TestLargeYAMLGeneratesDSXJSON(t *testing.T) {
	t.Parallel()
	b, err := responseBytesFromModelFile("large.yaml")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	sw, ok := doc["switches"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, sw, "core")
	leaf := sw["leaf-1-1"].(map[string]any)["nodes"].([]any)
	require.GreaterOrEqual(t, len(leaf), 1)
	first := leaf[0].(map[string]any)
	require.Equal(t, "n-1101", first["node_id"])
}
