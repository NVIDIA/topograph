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

var (
	instances = []*InstanceTopology{
		{
			InstanceID:    "i-001",
			DatacenterID:  "nn-77777777",
			SpineID:       "nn-55555555",
			BlockID:       "nn-11111111",
			AcceleratorID: "acc-111111",
		},
		{
			InstanceID:    "i-002",
			DatacenterID:  "nn-77777777",
			SpineID:       "nn-55555555",
			BlockID:       "nn-22222222",
			AcceleratorID: "acc-222222",
		},
		{
			InstanceID:   "i-003",
			DatacenterID: "nn-77777777",
			SpineID:      "nn-66666666",
			BlockID:      "nn-33333333",
		},
		{
			InstanceID:   "i-004",
			DatacenterID: "nn-77777777",
			SpineID:      "nn-66666666",
			BlockID:      "nn-44444444",
		},
	}

	n1 = &Vertex{ID: "i-001", Name: "node1"}
	n2 = &Vertex{ID: "i-002", Name: "node2"}
	n3 = &Vertex{ID: "i-003", Name: "node3"}
	n4 = &Vertex{ID: "i-004", Name: "node4"}
	n5 = &Vertex{ID: "i-cpu", Name: "node5"}

	none = &Vertex{ID: NoTopology, Vertices: map[string]*Vertex{"i-cpu": n5}}

	i2n = map[string]string{
		"i-001": "node1",
		"i-002": "node2",
		"i-003": "node3",
		"i-004": "node4",
		"i-cpu": "node5",
	}
)

func TestToThreeTierGraphNoNorm(t *testing.T) {
	topo := NewClusterTopology()
	for _, inst := range instances {
		topo.Append(inst)
	}
	require.Equal(t, len(instances), topo.Len())

	v31 := &Vertex{ID: "nn-11111111", Vertices: map[string]*Vertex{"i-001": n1}}
	v32 := &Vertex{ID: "nn-22222222", Vertices: map[string]*Vertex{"i-002": n2}}
	v33 := &Vertex{ID: "nn-33333333", Vertices: map[string]*Vertex{"i-003": n3}}
	v34 := &Vertex{ID: "nn-44444444", Vertices: map[string]*Vertex{"i-004": n4}}

	v21 := &Vertex{
		ID: "nn-55555555",
		Vertices: map[string]*Vertex{
			"nn-11111111": v31,
			"nn-22222222": v32,
		},
	}

	v22 := &Vertex{
		ID: "nn-66666666",
		Vertices: map[string]*Vertex{
			"nn-33333333": v33,
			"nn-44444444": v34,
		},
	}

	v1 := &Vertex{
		ID:       "nn-77777777",
		Vertices: map[string]*Vertex{"nn-55555555": v21, "nn-66666666": v22},
	}

	v0 := &Vertex{
		Vertices: map[string]*Vertex{
			"nn-77777777": v1,
			NoTopology:    none,
		},
	}

	b01 := &Vertex{
		ID:       "block001",
		Name:     "acc-111111",
		Vertices: map[string]*Vertex{"node1": {ID: "i-001", Name: "node1"}},
	}

	b02 := &Vertex{
		ID:       "block002",
		Name:     "acc-222222",
		Vertices: map[string]*Vertex{"node2": {ID: "i-002", Name: "node2"}},
	}

	blocks := &Vertex{
		Vertices: map[string]*Vertex{
			"acc-111111": b01,
			"acc-222222": b02,
		},
	}

	expected := &Vertex{
		Vertices: map[string]*Vertex{TopologyTree: v0, TopologyBlock: blocks},
	}

	graph, err := topo.ToThreeTierGraph("test", []ComputeInstances{{Instances: i2n}}, false)
	require.NoError(t, err)
	require.Equal(t, expected, graph)
}

func TestToThreeTierGraphNorm(t *testing.T) {
	topo := NewClusterTopology()
	for _, inst := range instances {
		topo.Append(inst)
	}
	require.Equal(t, len(instances), topo.Len())

	v31 := &Vertex{Name: "switch.1.1", ID: "nn-11111111", Vertices: map[string]*Vertex{"i-001": n1}}
	v32 := &Vertex{Name: "switch.1.2", ID: "nn-22222222", Vertices: map[string]*Vertex{"i-002": n2}}
	v33 := &Vertex{Name: "switch.1.3", ID: "nn-33333333", Vertices: map[string]*Vertex{"i-003": n3}}
	v34 := &Vertex{Name: "switch.1.4", ID: "nn-44444444", Vertices: map[string]*Vertex{"i-004": n4}}

	v21 := &Vertex{
		Name: "switch.2.1",
		ID:   "nn-55555555",
		Vertices: map[string]*Vertex{
			"nn-11111111": v31,
			"nn-22222222": v32,
		},
	}

	v22 := &Vertex{
		Name: "switch.2.2",
		ID:   "nn-66666666",
		Vertices: map[string]*Vertex{
			"nn-33333333": v33,
			"nn-44444444": v34,
		},
	}

	v1 := &Vertex{
		Name:     "switch.3.1",
		ID:       "nn-77777777",
		Vertices: map[string]*Vertex{"nn-55555555": v21, "nn-66666666": v22},
	}

	v0 := &Vertex{
		Vertices: map[string]*Vertex{
			"nn-77777777": v1,
			NoTopology:    none,
		},
	}

	b01 := &Vertex{
		ID:       "block001",
		Name:     "acc-111111",
		Vertices: map[string]*Vertex{"node1": {ID: "i-001", Name: "node1"}},
	}

	b02 := &Vertex{
		ID:       "block002",
		Name:     "acc-222222",
		Vertices: map[string]*Vertex{"node2": {ID: "i-002", Name: "node2"}},
	}

	blocks := &Vertex{
		Vertices: map[string]*Vertex{
			"acc-111111": b01,
			"acc-222222": b02,
		},
	}

	expected := &Vertex{
		Vertices: map[string]*Vertex{TopologyTree: v0, TopologyBlock: blocks},
	}

	graph, err := topo.ToThreeTierGraph("test", []ComputeInstances{{Instances: i2n}}, true)
	require.NoError(t, err)
	require.Equal(t, expected, graph)
}
