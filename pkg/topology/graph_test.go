/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
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
			FabricTiers:   ClosestFirstFabricTiers("nn-11111111", "nn-55555555", "nn-77777777"),
			AcceleratorID: "acc-111111",
		},
		{
			InstanceID:    "i-002",
			FabricTiers:   ClosestFirstFabricTiers("nn-22222222", "nn-55555555", "nn-77777777"),
			AcceleratorID: "acc-222222",
		},
		{
			InstanceID:  "i-003",
			FabricTiers: ClosestFirstFabricTiers("nn-33333333", "nn-66666666", "nn-77777777"),
		},
		{
			InstanceID:  "i-004",
			FabricTiers: ClosestFirstFabricTiers("nn-44444444", "nn-66666666", "nn-77777777"),
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

func TestToGraphNoNorm(t *testing.T) {
	topo := NewClusterTopology()
	for _, inst := range instances {
		topo.Append(inst)
	}
	require.Equal(t, len(instances), topo.Len())

	inst0 := "Instance:i-001 Fabric-Tier-0:nn-11111111 Fabric-Tier-1:nn-55555555 Fabric-Tier-2:nn-77777777 Accelerator:acc-111111"
	require.Equal(t, inst0, topo.Instances[0].String())

	inst2 := "Instance:i-003 Fabric-Tier-0:nn-33333333 Fabric-Tier-1:nn-66666666 Fabric-Tier-2:nn-77777777"
	require.Equal(t, inst2, topo.Instances[2].String())

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

	domains := NewDomainMap()
	domains.AddHost("acc-111111", "i-001", "node1")
	domains.AddHost("acc-222222", "i-002", "node2")

	expected := &Graph{
		Tiers:   v0,
		Domains: domains,
	}

	graph := topo.ToGraph("test", []ComputeInstances{{Instances: i2n}}, 0, false)
	require.Equal(t, expected, graph)
}

func TestToGraphNorm(t *testing.T) {
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

	domains := NewDomainMap()
	domains.AddHost("acc-111111", "i-001", "node1")
	domains.AddHost("acc-222222", "i-002", "node2")

	expected := &Graph{
		Tiers:   v0,
		Domains: domains,
	}

	graph := topo.ToGraph("test", []ComputeInstances{{Instances: i2n}}, 0, true)
	require.Equal(t, expected, graph)

	inst0 := "Instance:i-001 Fabric-Tier-0:nn-11111111 (switch.1.1) Fabric-Tier-1:nn-55555555 (switch.2.1) Fabric-Tier-2:nn-77777777 (switch.3.1) Accelerator:acc-111111"
	require.Equal(t, inst0, topo.Instances[0].String())

	inst2 := "Instance:i-003 Fabric-Tier-0:nn-33333333 (switch.1.3) Fabric-Tier-1:nn-66666666 (switch.2.2) Fabric-Tier-2:nn-77777777 (switch.3.1)"
	require.Equal(t, inst2, topo.Instances[2].String())
}

func TestToGraphIncludesInstanceData(t *testing.T) {
	topo := NewClusterTopology()
	topo.Append(&InstanceTopology{
		InstanceID:    "i-001",
		FabricTiers:   ClosestFirstFabricTiers("leaf-1", "spine-1", "core-1"),
		AcceleratorID: "nvl-1",
		Instance: &Instance{
			ID:            "i-001",
			NetworkLayers: []string{"leaf-1", "spine-1", "core-1"},
			Labels:        map[string]string{KeyNvidiaGPUProduct: "H100"},
		},
	})

	graph := topo.ToGraph("test", []ComputeInstances{
		{
			Region:    "region-1",
			Instances: map[string]string{"i-001": "node1"},
		},
	}, 1, false)

	require.Equal(t, map[string]Instance{
		"i-001": {
			ID:            "i-001",
			NetworkLayers: []string{"leaf-1", "spine-1"},
			Labels: map[string]string{
				KeyNvidiaGPUProduct:    "H100",
				KeyTopologyAccelerator: "nvl-1",
			},
		},
	}, graph.Instances)
}

func TestTrimTiers(t *testing.T) {
	tests := []struct {
		name      string
		trimTiers int
		in        InstanceTopology
		out       []string
	}{
		{
			name:      "Case 1: trim none",
			trimTiers: 0,
			in: InstanceTopology{
				FabricTiers: ClosestFirstFabricTiers("leaf1", "spine1", "core1"),
			},
			out: []string{"leaf1", "spine1", "core1"},
		},
		{
			name:      "Case 2: trim 1 tier",
			trimTiers: 1,
			in: InstanceTopology{
				FabricTiers: ClosestFirstFabricTiers("leaf1", "spine1", "core1"),
			},
			out: []string{"leaf1", "spine1"},
		},
		{
			name:      "Case 3: trim 2 tiers",
			trimTiers: 2,
			in: InstanceTopology{
				FabricTiers: ClosestFirstFabricTiers("leaf1", "spine1", "core1"),
			},
			out: []string{"leaf1"},
		},
		{
			name:      "Case 4: trim all tiers",
			trimTiers: 3,
			in: InstanceTopology{
				FabricTiers: ClosestFirstFabricTiers("leaf1", "spine1", "core1"),
			},
			out: []string{},
		},
		{
			name:      "Case 5: trim more than available",
			trimTiers: 10,
			in: InstanceTopology{
				FabricTiers: ClosestFirstFabricTiers("leaf1", "spine1", "core1"),
			},
			out: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := tt.in
			tiers := trimmedTiers(&inst, tt.trimTiers)
			ids := make([]string, len(tiers))
			for i := range tiers {
				ids[i] = tiers[i].ID
			}
			require.Equal(t, tt.out, ids)
		})
	}
}

func TestToGraphSupportsVariableTierCount(t *testing.T) {
	topo := NewClusterTopology()
	topo.Append(&InstanceTopology{
		InstanceID:    "instance-1",
		FabricTiers:   ClosestFirstFabricTiers("fabric-0", "fabric-1", "fabric-2", "fabric-3"),
		AcceleratorID: "accelerator",
	})

	graph := topo.ToGraph("test", []ComputeInstances{{
		Instances: map[string]string{"instance-1": "node-1"},
	}}, 0, false)

	vertex := graph.Tiers.Vertices["fabric-3"]
	for _, id := range []string{"fabric-2", "fabric-1", "fabric-0", "instance-1"} {
		require.NotNil(t, vertex)
		vertex = vertex.Vertices[id]
	}
	require.Equal(t, "node-1", vertex.Name)
	require.Contains(t, graph.Domains["accelerator"], "node-1")
}

func TestToGraphKeepsSameIDAtDifferentLevelsDistinct(t *testing.T) {
	topo := NewClusterTopology()
	topo.Append(&InstanceTopology{
		InstanceID:  "instance-1",
		FabricTiers: ClosestFirstFabricTiers("shared", "shared"),
	})

	graph := topo.ToGraph("test", []ComputeInstances{{
		Instances: map[string]string{"instance-1": "node-1"},
	}}, 0, false)

	outer := graph.Tiers.Vertices["shared"]
	inner := outer.Vertices["shared"]
	require.NotSame(t, outer, inner)
	require.Equal(t, "node-1", inner.Vertices["instance-1"].Name)
}

func TestToGraphMergesMixedDepthPathsAtSharedRoot(t *testing.T) {
	topo := NewClusterTopology()
	topo.Append(&InstanceTopology{
		InstanceID:  "instance-1",
		FabricTiers: ClosestFirstFabricTiers("leaf-1", "shared-root"),
	})
	topo.Append(&InstanceTopology{
		InstanceID:  "instance-2",
		FabricTiers: ClosestFirstFabricTiers("leaf-2", "spine-2", "shared-root"),
	})

	graph := topo.ToGraph("test", []ComputeInstances{{
		Instances: map[string]string{
			"instance-1": "node-1",
			"instance-2": "node-2",
		},
	}}, 0, false)

	root := graph.Tiers.Vertices["shared-root"]
	require.NotNil(t, root)
	require.Contains(t, root.Vertices, "leaf-1")
	require.Contains(t, root.Vertices, "spine-2")
	require.Equal(t, "node-1", root.Vertices["leaf-1"].Vertices["instance-1"].Name)
	require.Equal(t, "node-2", root.Vertices["spine-2"].Vertices["leaf-2"].Vertices["instance-2"].Name)
}

func TestTrimTiersTreatsNegativeAsZero(t *testing.T) {
	inst := &InstanceTopology{FabricTiers: ClosestFirstFabricTiers("fabric-0", "fabric-1")}
	require.Equal(t, inst.FabricTiers, trimmedTiers(inst, -1))
}
