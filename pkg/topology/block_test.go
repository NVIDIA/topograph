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

package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToBlocks(t *testing.T) {
	testCases := []struct {
		name      string
		domainMap DomainMap
		blocks    *Vertex
	}{
		{
			name: "Case 1: no input",
			blocks: &Vertex{
				Vertices: make(map[string]*Vertex),
			},
		},
		{
			name:      "Case 2: one block",
			domainMap: DomainMap{"domain1": {"host1": "instance1", "host2": "instance2"}},
			blocks: &Vertex{
				Vertices: map[string]*Vertex{
					"domain1": {
						Name: "domain1",
						ID:   "block001",
						Vertices: map[string]*Vertex{
							"host1": {ID: "instance1", Name: "host1"},
							"host2": {ID: "instance2", Name: "host2"},
						},
					},
				},
			},
		},
		{
			name: "Case 3: two blocks",
			domainMap: DomainMap{
				"domain1": {"host1": "instance1", "host2": "instance2"},
				"domain2": {"host3": "instance3"},
			},
			blocks: &Vertex{
				Vertices: map[string]*Vertex{
					"domain1": {
						Name: "domain1",
						ID:   "block001",
						Vertices: map[string]*Vertex{
							"host1": {ID: "instance1", Name: "host1"},
							"host2": {ID: "instance2", Name: "host2"},
						},
					},
					"domain2": {
						Name: "domain2",
						ID:   "block002",
						Vertices: map[string]*Vertex{
							"host3": {ID: "instance3", Name: "host3"},
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
				for hostname, instance := range domain {
					domainMap.AddHost(domainName, instance, hostname)
				}
			}
			require.Equal(t, tc.blocks, domainMap.ToBlocks())
		})
	}
}
