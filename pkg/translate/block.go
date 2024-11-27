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
	"sort"

	"github.com/NVIDIA/topograph/pkg/topology"
)

// DomainMap maps domain name to a map of hostnames
type DomainMap map[string]map[string]struct{}

func NewDomainMap() DomainMap {
	return make(DomainMap)
}

func (m DomainMap) ToBlocks() *topology.Vertex {
	blockRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}

	domainNames := make([]string, 0, len(m))
	for domainName := range m {
		domainNames = append(domainNames, domainName)
	}
	sort.Strings(domainNames)

	for _, domainName := range domainNames {
		domain := m[domainName]
		vertex := &topology.Vertex{
			ID:       domainName,
			Vertices: make(map[string]*topology.Vertex),
		}
		for node := range domain {
			vertex.Vertices[node] = &topology.Vertex{Name: node, ID: node}
		}

		blockRoot.Vertices[domainName] = vertex
	}

	return blockRoot
}

func (m DomainMap) AddHost(domain, host string) {
	d, ok := m[domain]
	if !ok {
		m[domain] = make(map[string]struct{})
		d = m[domain]
	}
	d[host] = struct{}{}
}
