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
	"testing"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestToBlocks(t *testing.T) {
	testCases := []struct {
		name      string
		domainMap DomainMap
		blocks    *topology.Vertex
	}{
		{
			name: "Case 1: no input",
			blocks: &topology.Vertex{
				Vertices: make(map[string]*topology.Vertex),
			},
		},
		{
			name:      "Case 2: one block",
			domainMap: DomainMap{"domain1": {"host1": struct{}{}, "host2": struct{}{}}},
			blocks: &topology.Vertex{
				Vertices: map[string]*topology.Vertex{
					"domain1": {
						ID: "domain1",
						Vertices: map[string]*topology.Vertex{
							"host1": {ID: "host1", Name: "host1"},
							"host2": {ID: "host2", Name: "host2"},
						},
					},
				},
			},
		},
		{
			name: "Case 3: two blocks",
			domainMap: DomainMap{
				"domain1": {"host1": struct{}{}, "host2": struct{}{}},
				"domain2": {"host3": struct{}{}},
			},
			blocks: &topology.Vertex{
				Vertices: map[string]*topology.Vertex{
					"domain1": {
						ID: "domain1",
						Vertices: map[string]*topology.Vertex{
							"host1": {ID: "host1", Name: "host1"},
							"host2": {ID: "host2", Name: "host2"},
						},
					},
					"domain2": {
						ID: "domain2",
						Vertices: map[string]*topology.Vertex{
							"host3": {ID: "host3", Name: "host3"},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			domainMap := NewDomainMap()
			for domainName, domain := range tc.domainMap {
				for hostname := range domain {
					domainMap.AddHost(domainName, hostname)
				}
			}
			require.Equal(t, tc.blocks, domainMap.ToBlocks())
		})
	}
}
