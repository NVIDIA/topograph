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

func TestDomainMapAddHost(t *testing.T) {
	domainMap := NewDomainMap()

	domainMap.AddHost("domain1", "instance1", "host1")
	domainMap.AddHost("domain1", "instance2", "host2")
	domainMap.AddHost("domain2", "instance3", "host3")
	domainMap.AddHost("", "instance4", "host4")

	require.Equal(t, DomainMap{
		"domain1": map[string]*HostInfo{
			"host1": {Domain: "domain1", InstanceID: "instance1", HostName: "host1"},
			"host2": {Domain: "domain1", InstanceID: "instance2", HostName: "host2"},
		},
		"domain2": map[string]*HostInfo{
			"host3": {Domain: "domain2", InstanceID: "instance3", HostName: "host3"},
		},
	}, domainMap)
}

// TestDomainMapEnsureDomain verifies that EnsureDomain declares a domain with
// no live hosts (so it survives iteration and can be emitted as an empty
// block) and is idempotent when the domain already exists.
func TestDomainMapEnsureDomain(t *testing.T) {
	domainMap := NewDomainMap()
	domainMap.EnsureDomain("empty")
	require.Contains(t, domainMap, "empty")
	require.Len(t, domainMap["empty"], 0)

	// Adding a host under the same domain must not lose the initial declaration.
	domainMap.AddHost("empty", "i1", "h1")
	require.Len(t, domainMap["empty"], 1)

	// EnsureDomain over an already-populated domain must not clear hosts.
	domainMap.EnsureDomain("empty")
	require.Len(t, domainMap["empty"], 1)

	// Empty domain names are ignored (matching AddHostInfo).
	domainMap.EnsureDomain("")
	require.NotContains(t, domainMap, "")
}
