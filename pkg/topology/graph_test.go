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

func TestToThreeTierGraph(t *testing.T) {
	instances := []*InstanceTopology{
		{
			InstanceID:   "i-0febfe7a633a552cc",
			DatacenterID: "nn-098f9e7674016cb1c",
			SpineID:      "nn-224a2a4d9df61a975",
			BlockID:      "nn-20da390f7d602f42f",
		},
		{
			InstanceID:   "i-0727864293842c5f1",
			DatacenterID: "nn-098f9e7674016cb1c",
			SpineID:      "nn-224a2a4d9df61a975",
			BlockID:      "nn-568b52163b3ce19c8",
		},
		{
			InstanceID: "i-04e4ca4199532bbba",

			DatacenterID: "nn-098f9e7674016cb1c",
			SpineID:      "nn-224a2a4d9df61a975",
			BlockID:      "nn-d7d7a965aec389018",
		},
		{
			InstanceID:   "i-0359d6503bf895535",
			DatacenterID: "nn-098f9e7674016cb1c",
			SpineID:      "nn-224a2a4d9df61a975",
			BlockID:      "nn-ef5c999131844763a",
		},
	}

	topo := NewClusterTopology()
	for _, inst := range instances {
		topo.Append(inst)
	}
	require.Equal(t, len(instances), topo.Len())

	i2n := map[string]string{
		"i-0febfe7a633a552cc": "node1",
		"i-0727864293842c5f1": "node2",
		"i-04e4ca4199532bbba": "node3",
		"i-0359d6503bf895535": "node4",
		"i-cpu":               "node5",
	}

	n1 := &Vertex{ID: "i-0febfe7a633a552cc", Name: "node1"}
	n2 := &Vertex{ID: "i-0727864293842c5f1", Name: "node2"}
	n3 := &Vertex{ID: "i-04e4ca4199532bbba", Name: "node3"}
	n4 := &Vertex{ID: "i-0359d6503bf895535", Name: "node4"}
	n5 := &Vertex{ID: "i-cpu", Name: "node5"}

	none := &Vertex{ID: NoTopology, Vertices: map[string]*Vertex{"i-cpu": n5}}

	v31 := &Vertex{ID: "nn-20da390f7d602f42f", Vertices: map[string]*Vertex{"i-0febfe7a633a552cc": n1}}
	v32 := &Vertex{ID: "nn-568b52163b3ce19c8", Vertices: map[string]*Vertex{"i-0727864293842c5f1": n2}}
	v33 := &Vertex{ID: "nn-d7d7a965aec389018", Vertices: map[string]*Vertex{"i-04e4ca4199532bbba": n3}}
	v34 := &Vertex{ID: "nn-ef5c999131844763a", Vertices: map[string]*Vertex{"i-0359d6503bf895535": n4}}

	v2 := &Vertex{
		ID: "nn-224a2a4d9df61a975",
		Vertices: map[string]*Vertex{
			"nn-20da390f7d602f42f": v31,
			"nn-568b52163b3ce19c8": v32,
			"nn-d7d7a965aec389018": v33,
			"nn-ef5c999131844763a": v34,
		},
	}

	v1 := &Vertex{
		ID:       "nn-098f9e7674016cb1c",
		Vertices: map[string]*Vertex{"nn-224a2a4d9df61a975": v2},
	}

	v0 := &Vertex{
		Vertices: map[string]*Vertex{
			"nn-098f9e7674016cb1c": v1,
			NoTopology:             none,
		},
	}

	expected := &Vertex{
		Vertices: map[string]*Vertex{TopologyTree: v0},
	}

	tree, err := topo.ToThreeTierGraph("test", []ComputeInstances{{Instances: i2n}})
	require.NoError(t, err)
	require.Equal(t, expected, tree)
}
