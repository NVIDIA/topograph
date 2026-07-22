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

// nvl576DomainMap builds a small DomainMap that mirrors the NVL576 level
// hierarchy used across tests:
//
//	Level1 = "dc-1"
//	Level2 = "building-1"
//	Level3 = "room-1" (rack-a: 3 nodes) or "room-2" (rack-b: 2 nodes, rack-c: 3 nodes)
//	Domain = "rack-a" | "rack-b" | "rack-c"
func nvl576DomainMap() DomainMap {
	m := NewDomainMap()
	for _, h := range []HostInfo{
		{Domain: "rack-a", HostName: "n01", InstanceID: "i01", Level1: "dc-1", Level2: "building-1", Level3: "room-1"},
		{Domain: "rack-a", HostName: "n02", InstanceID: "i02", Level1: "dc-1", Level2: "building-1", Level3: "room-1"},
		{Domain: "rack-a", HostName: "n03", InstanceID: "i03", Level1: "dc-1", Level2: "building-1", Level3: "room-1"},
		{Domain: "rack-b", HostName: "n04", InstanceID: "i04", Level1: "dc-1", Level2: "building-1", Level3: "room-2"},
		{Domain: "rack-b", HostName: "n05", InstanceID: "i05", Level1: "dc-1", Level2: "building-1", Level3: "room-2"},
		{Domain: "rack-c", HostName: "n06", InstanceID: "i06", Level1: "dc-1", Level2: "building-1", Level3: "room-2"},
		{Domain: "rack-c", HostName: "n07", InstanceID: "i07", Level1: "dc-1", Level2: "building-1", Level3: "room-2"},
		{Domain: "rack-c", HostName: "n08", InstanceID: "i08", Level1: "dc-1", Level2: "building-1", Level3: "room-2"},
	} {
		hCopy := h
		m.AddHostInfo(&hCopy)
	}
	return m
}

func TestGetLevelInfo(t *testing.T) {
	tests := []struct {
		name        string
		domainMap   DomainMap
		level       int
		wantPresent bool
		wantMembers map[string][]string // full members map (children sorted)
	}{
		{
			name:        "empty DomainMap returns not present",
			domainMap:   NewDomainMap(),
			level:       3,
			wantPresent: false,
		},
		{
			name:        "invalid level returns not present",
			domainMap:   nvl576DomainMap(),
			level:       0,
			wantPresent: false,
		},
		{
			name: "level absent from all hosts returns not present",
			domainMap: func() DomainMap {
				m := NewDomainMap()
				// hosts have no Level1 set
				m.AddHostInfo(&HostInfo{Domain: "rack-a", HostName: "n01", InstanceID: "i01", Level3: "room-1"})
				return m
			}(),
			level:       1,
			wantPresent: false,
		},
		{
			name:        "level 4 groups by domain; children are sorted hostnames",
			domainMap:   nvl576DomainMap(),
			level:       4,
			wantPresent: true,
			wantMembers: map[string][]string{
				"rack-a": {"n01", "n02", "n03"},
				"rack-b": {"n04", "n05"},
				"rack-c": {"n06", "n07", "n08"},
			},
		},
		{
			name:        "level 3 groups by Level3; children are sorted domain IDs",
			domainMap:   nvl576DomainMap(),
			level:       3,
			wantPresent: true,
			wantMembers: map[string][]string{
				"room-1": {"rack-a"},
				"room-2": {"rack-b", "rack-c"},
			},
		},
		{
			name:        "level 2 groups by Level2; children are sorted Level3 values",
			domainMap:   nvl576DomainMap(),
			level:       2,
			wantPresent: true,
			wantMembers: map[string][]string{
				"building-1": {"room-1", "room-2"},
			},
		},
		{
			name:        "level 1 groups by Level1; children are sorted Level2 values",
			domainMap:   nvl576DomainMap(),
			level:       1,
			wantPresent: true,
			wantMembers: map[string][]string{
				"dc-1": {"building-1"},
			},
		},
		{
			name: "multiple top-level groups",
			domainMap: func() DomainMap {
				m := NewDomainMap()
				m.AddHostInfo(&HostInfo{Domain: "r1", HostName: "a", InstanceID: "a", Level1: "dc-west", Level2: "bldg-w"})
				m.AddHostInfo(&HostInfo{Domain: "r1", HostName: "b", InstanceID: "b", Level1: "dc-west", Level2: "bldg-w"})
				m.AddHostInfo(&HostInfo{Domain: "r2", HostName: "c", InstanceID: "c", Level1: "dc-east", Level2: "bldg-e"})
				m.AddHostInfo(&HostInfo{Domain: "r2", HostName: "d", InstanceID: "d", Level1: "dc-east", Level2: "bldg-e"})
				m.AddHostInfo(&HostInfo{Domain: "r2", HostName: "e", InstanceID: "e", Level1: "dc-east", Level2: "bldg-e"})
				return m
			}(),
			level:       1,
			wantPresent: true,
			wantMembers: map[string][]string{
				"dc-west": {"bldg-w"},
				"dc-east": {"bldg-e"},
			},
		},
		{
			name: "hosts with empty level value are excluded from that level",
			domainMap: func() DomainMap {
				m := NewDomainMap()
				m.AddHostInfo(&HostInfo{Domain: "rack-a", HostName: "n1", InstanceID: "i1", Level3: "room-1"})
				// n2 has no Level3; it must not appear in level-3 results
				m.AddHostInfo(&HostInfo{Domain: "rack-a", HostName: "n2", InstanceID: "i2"})
				return m
			}(),
			level:       3,
			wantPresent: true,
			wantMembers: map[string][]string{
				"room-1": {"rack-a"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			present, members := tc.domainMap.GetLevelInfo(tc.level)
			require.Equal(t, tc.wantPresent, present)
			if !tc.wantPresent {
				require.Nil(t, members)
				return
			}
			require.Equal(t, tc.wantMembers, members)
		})
	}
}

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
