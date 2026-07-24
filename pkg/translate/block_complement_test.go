/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/stretchr/testify/require"
)

// TestComplementMissingBaseBlock verifies that when an accelerator domain is absent
// from the graph the complement tree pads to the next valid blockSizes capacity. With
// blockSizes=[4,8,16] and 3 domains each holding ≤4 nodes, a 4th empty block is added
// to reach the 16-node lastBS boundary.
func TestComplementMissingBaseBlock(t *testing.T) {
	root, _ := getBlockWithIBTestSet()
	delete(root.Domains, "B2")

	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{4, 8, 16},
	}
	nt, err := NewNetworkTopology(root, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.toBlockTopology(&buf, false))

	expected := strings.Join([]string{
		"# block001=B1",
		"BlockName=block001 Nodes=Node[104-106]",
		"# block002=B3",
		"BlockName=block002 Nodes=Node[304-306]",
		"# block003=B4",
		"BlockName=block003 Nodes=Node[401-403]",
		"BlockName=block004",
		"BlockSizes=4,8,16",
		"",
	}, "\n")
	require.Equal(t, expected, buf.String())
}

// TestComplementMissingLeafSegment verifies the asymmetric-spine case: one spine has
// 4 leaf switches and the other has 3. With blockSizes=[4,16,32] and 7 domains, the
// tree pads to the next 32-node boundary, adding one empty block008 placeholder.
func TestComplementMissingLeafSegment(t *testing.T) {
	root, _ := getBlockWithIBAsymmetricSpineTestSet()

	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{4, 16, 32},
	}
	nt, err := NewNetworkTopology(root, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.toBlockTopology(&buf, false))

	expected := strings.Join([]string{
		"# block001=B1",
		"BlockName=block001 Nodes=Node[101-103]",
		"# block002=B2",
		"BlockName=block002 Nodes=Node[201-202,205]",
		"# block003=B3",
		"BlockName=block003 Nodes=Node[301-303]",
		"# block004=B4",
		"BlockName=block004 Nodes=Node[401-403]",
		"# block005=B5",
		"BlockName=block005 Nodes=Node[501-503]",
		"# block006=B6",
		"BlockName=block006 Nodes=Node[601-603]",
		"# block007=B7",
		"BlockName=block007 Nodes=Node[701-703]",
		"BlockName=block008",
		"BlockSizes=4,16,32",
		"",
	}, "\n")
	require.Equal(t, expected, buf.String())
}

// TestNoComplementWithoutTree verifies that complementBlocks produces no empty placeholder
// slots when the graph has no Tiers (no switch tree) and no host carries a SubDomain.
// With blockSizes=[4,8,16] and each domain holding 3 nodes (DesiredNodeCount=4 = 1 base
// block), the root's DesiredNodeCount=16 drives padding only at the root level, not within
// individual domains. The per-domain output contains no empty block slots.
func TestNoComplementWithoutTree(t *testing.T) {
	root, _ := getBlockTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{4, 8, 16},
	}
	nt, err := NewNetworkTopology(root, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.toBlockTopology(&buf, false))
	require.NotContains(t, buf.String(), "BlockName=block002\n")
	require.Contains(t, buf.String(), "BlockSizes=4,8,16")
}

// TestNoComplementSingleBlockSize verifies that a single BlockSizes entry (no tiers)
// disables the complement path entirely.
func TestNoComplementSingleBlockSize(t *testing.T) {
	root, _ := getBlockWithIBTestSet()
	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{3},
	}
	nt, err := NewNetworkTopology(root, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.toBlockTopology(&buf, false))
	require.Equal(t, testBlockConfig1_2, buf.String())
}

// TestComplementKeepsSeparateAccelerators verifies that two undersized accelerators are
// never merged into a single base block. maxAcceleratorSize=3 ≤ baseBlockSize=8, so
// groupSize=1 and complement is a no-op; the original 2-block list is returned with
// each accelerator in its own separate block.
func TestComplementKeepsSeparateAccelerators(t *testing.T) {
	domains := topology.NewDomainMap()
	nodesB1 := []string{"Node101", "Node102", "Node103"}
	nodesB2 := []string{"Node201", "Node202", "Node205"}
	for _, n := range nodesB1 {
		domains.AddHostInfo(&topology.HostInfo{Domain: "B1", HostName: n, InstanceID: n})
	}
	for _, n := range nodesB2 {
		domains.AddHostInfo(&topology.HostInfo{Domain: "B2", HostName: n, InstanceID: n})
	}

	nt := &NetworkTopology{
		domains: domains,
		blocks: []*blockInfo{
			{name: "B1", nodes: nodesB1},
			{name: "B2", nodes: nodesB2},
		},
	}

	out := nt.complementBlocks(nt.blocks, []int{8, 16})
	require.Len(t, out, 2)
	require.Equal(t, "B1", out[0].name)
	require.Len(t, out[0].nodes, 3)
	require.Equal(t, "B2", out[1].name)
	require.Len(t, out[1].nodes, 3)
}

// TestComplementExcessHostsPerAccelerator verifies the split path: when a single
// accelerator has more hosts than baseBlockSize it is split into multiple base blocks,
// each carrying the same accelerator name, and every host appears exactly once.
// maxAcceleratorSize=12, baseBlockSize=4 → groupSize=4 (2^2*4=16 ≥ 12); 3 real blocks
// padded to 4 (ceil(3/4)*4).
func TestComplementExcessHostsPerAccelerator(t *testing.T) {
	domains := topology.NewDomainMap()
	nodes := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("Node%03d", 100+i)
		nodes = append(nodes, name)
		domains.AddHostInfo(&topology.HostInfo{
			Domain:     "B1",
			HostName:   name,
			InstanceID: name,
		})
	}

	nt := &NetworkTopology{
		domains: domains,
		blocks: []*blockInfo{{
			id:    "block001",
			name:  "B1",
			nodes: nodes,
		}},
	}

	out := nt.complementBlocks(nt.blocks, []int{4, 8, 16})
	// 3 base blocks (ceil(12/4)) padded to 4 (groupSize=4, ceil(3/4)*4=4).
	require.Len(t, out, 4)
	require.True(t, isEmptyBlock(out[3]), "out[3] should be the group-alignment padding block")

	seen := make(map[string]bool)
	for _, b := range out[:3] {
		require.Equal(t, "B1", b.name)
		for _, n := range b.nodes {
			seen[n] = true
		}
	}
	require.Len(t, seen, 12)
	for _, n := range nodes {
		require.True(t, seen[n])
	}
}

// TestComplementPartitionLocalDomainsOnly verifies that complementBlocks scopes domain
// lookup to the partition's own blocks. B2 exists in nt.domains but is excluded from
// partitionBlocks, so the complement result contains only B1, B3, and B4. With
// maxAcceleratorSize=3 ≤ baseBlockSize=4, groupSize=1 and no padding is applied.
func TestComplementPartitionLocalDomainsOnly(t *testing.T) {
	root, _ := getBlockWithIBTestSet()
	nt, err := NewNetworkTopology(root, &Config{Plugin: topology.TopologyBlock})
	require.NoError(t, err)

	// Partition owns B1, B3, B4 but not B2 (B2 remains in global nt.domains).
	partitionBlocks := make([]*blockInfo, 0, 3)
	for _, b := range nt.blocks {
		if b.name == "B2" {
			continue
		}
		partitionBlocks = append(partitionBlocks, b)
	}

	out := nt.complementBlocks(partitionBlocks, []int{4, 8, 16})
	// 3 real domains padded to 4 to reach the 16-node lastBS boundary.
	require.Len(t, out, 4)
	require.Equal(t, "B1", out[0].name)
	require.Equal(t, "B3", out[1].name)
	require.Equal(t, "B4", out[2].name)
	require.True(t, isEmptyBlock(out[3]))
}

// TestDomainsForBlocksFilteredToPartitionNodes is a regression test for cross-partition
// node contamination. Domain B1 holds 4 nodes globally (n1–n4), but the partition-local
// blockInfo only lists n1, n2, n3. Without filtering, domainsForBlocks would copy all 4
// hosts and n4 would appear in the complemented output. With the fix, only n1–n3 are
// used: the split produces two base blocks ([n1,n2] and [n3]) and n4 is absent.
func TestDomainsForBlocksFilteredToPartitionNodes(t *testing.T) {
	domains := topology.NewDomainMap()
	for _, n := range []string{"n1", "n2", "n3", "n4"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "B1", HostName: n, InstanceID: n})
	}

	// Partition only owns n1, n2, n3 — n4 belongs to another partition.
	nt := &NetworkTopology{
		domains: domains,
		blocks:  []*blockInfo{},
	}
	partitionBlocks := []*blockInfo{
		{name: "B1", nodes: []string{"n1", "n2", "n3"}},
	}

	out := nt.complementBlocks(partitionBlocks, []int{2, 4})
	// groupSize=2 (maxAccelSize=3, 2^1×2=4≥3); B1 splits into 2 base blocks.
	// len(packed)=2 ≠ len(input)=1 → complement applied.
	require.Len(t, out, 2)

	seen := make(map[string]bool)
	for _, b := range out {
		require.Equal(t, "B1", b.name)
		for _, n := range b.nodes {
			seen[n] = true
		}
	}
	require.True(t, seen["n1"])
	require.True(t, seen["n2"])
	require.True(t, seen["n3"])
	require.False(t, seen["n4"], "n4 belongs to another partition and must not appear")
}

// TestComplementWithMissingDomain verifies that when B2 has no entry in the domain map,
// domainsForBlocks only sees B1. The complement tree produces block001 with B1's nodes
// and an empty block002 padding slot (root-padded to reach blockSizes[last]=4).
func TestComplementWithMissingDomain(t *testing.T) {
	domains := topology.NewDomainMap()
	// Only B1 is in the domain map; B2 is not.
	for _, n := range []string{"n1", "n2"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "B1", HostName: n, InstanceID: n})
	}
	nt := &NetworkTopology{
		domains: domains,
	}
	input := []*blockInfo{
		{id: "block001", name: "B1", nodes: []string{"n1", "n2"}},
		{id: "block002", name: "B2", nodes: []string{"n3", "n4"}}, // no domain entry
	}
	out := nt.complementBlocks(input, []int{2, 4})
	require.Len(t, out, 2)
	require.Equal(t, "B1", out[0].name)
	require.Equal(t, []string{"n1", "n2"}, out[0].nodes)
	require.True(t, isEmptyBlock(out[1]))
}

// TestGetBlockTopologyUnitWithMultiAcceleratorDomains verifies the YAML per-partition
// complement path end-to-end: two domains, three accelerators (a1, a2, a3), block
// sizes [2,4]. a2 is undersized (fewer nodes than groupSize=2 requires), so it gets
// an empty padding slot; tree-capacity expansion adds two more trailing empty slots.
func TestGetBlockTopologyUnitWithMultiAcceleratorDomains(t *testing.T) {
	domains := topology.NewDomainMap()
	for _, n := range []string{"n10", "n11", "n12"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a1", HostName: n, InstanceID: n})
	}
	for _, n := range []string{"n20", "n21"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a2", HostName: n, InstanceID: n})
	}
	for _, n := range []string{"n31", "n32", "n33"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a3", HostName: n, InstanceID: n})
	}

	cfg := &Config{
		Topologies: map[string]*TopologySpec{
			"topo1": {
				Plugin:     topology.TopologyBlock,
				Nodes:      []string{"n[10-12]", "n[20-21]", "n[31-33]"},
				BlockSizes: []int{2, 4},
			},
		},
	}

	graph := &topology.Graph{Domains: domains}
	nt, err := NewNetworkTopology(graph, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.Generate(&buf))

	expected := strings.Join([]string{
		"- topology: topo1",
		"  cluster_default: false",
		"  block:",
		"    block_sizes:",
		"        - 2",
		"        - 4",
		"    blocks:",
		"        - block: block1",
		"          nodes: n[10-11]",
		"        - block: block2",
		"          nodes: n12",
		"        - block: block3",
		"          nodes: n[20-21]",
		"        - block: block4",
		"        - block: block5",
		"          nodes: n[31-32]",
		"        - block: block6",
		"          nodes: n33",
		"",
	}, "\n")
	require.Equal(t, expected, buf.String())
}

// TestGetBlockTopologyUnitSingleBlockSize verifies that a TopologySpec with a single
// BlockSizes entry still splits domains that exceed baseBlockSize, but applies no
// empty padding slots. With blockSizes=[2] and lastBS=2, all domain aggregates have
// totalCount >= 2 (a1:4, a2:2, a3:4), so the siblings-of-equal-size rule is skipped
// entirely — no group-alignment or root padding is added. The result is the 5-block
// split list replacing the original 3-block input.
func TestGetBlockTopologyUnitSingleBlockSize(t *testing.T) {
	domains := topology.NewDomainMap()
	for _, n := range []string{"n10", "n11", "n12"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a1", HostName: n, InstanceID: n})
	}
	for _, n := range []string{"n20", "n21"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a2", HostName: n, InstanceID: n})
	}
	for _, n := range []string{"n31", "n32", "n33"} {
		domains.AddHostInfo(&topology.HostInfo{Domain: "a3", HostName: n, InstanceID: n})
	}

	cfg := &Config{
		Topologies: map[string]*TopologySpec{
			"topo1": {
				Plugin:     topology.TopologyBlock,
				Nodes:      []string{"n[10-12]", "n[20-21]", "n[31-33]"},
				BlockSizes: []int{2},
			},
		},
	}

	graph := &topology.Graph{Domains: domains}
	nt, err := NewNetworkTopology(graph, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.Generate(&buf))

	// lastBS=2; all domain aggregates have totalCount >= 2, so padActualTree is a no-op.
	// Only the split of oversized domains (a1, a3 → 2 blocks each) produces new output.
	expected := strings.Join([]string{
		"- topology: topo1",
		"  cluster_default: false",
		"  block:",
		"    block_sizes:",
		"        - 2",
		"    blocks:",
		"        - block: block1",
		"          nodes: n[10-11]",
		"        - block: block2",
		"          nodes: n12",
		"        - block: block3",
		"          nodes: n[20-21]",
		"        - block: block4",
		"          nodes: n[31-32]",
		"        - block: block5",
		"          nodes: n33",
		"",
	}, "\n")
	require.Equal(t, expected, buf.String())
}

// TestComplementDualLevel validates dual-level block tree construction using the
// dual-level simulation model. Two accelerator domains (domain-01, domain-02) each
// contain sub-domains identified by SubDomain (rack-1-01 … rack-1-16 and
// rack-2-01 … rack-2-16). rack-1-03 and rack-1-13 have no nodes; rack-2-11 is absent.
//
// With BlockSizes=[9,144]:
//
//   - Group level (depth 2): max ActualNodeCount = 9 → DesiredNodeCount = 9 per group.
//     Each group leaf emits exactly 1 base block (groupSize = 9/9 = 1).
//
//   - Domain level (depth 1): max ActualNodeCount = 131 (domain-02) → DesiredNodeCount = 144.
//     domain-01 has 14 active groups → 14 real + 2 empty = 16 slots (blocks 001–016).
//     domain-02 has 15 active groups → 15 real + 1 empty = 16 slots (blocks 017–032).
//
//   - Root (depth 0): DesiredNodeCount = 144; 2 domain children total 288 > 144 → no padding.
//
//   - Total output: 32 blocks (16 per domain) followed by BlockSizes=9,144.
func TestComplementDualLevel(t *testing.T) {
	model, err := models.NewModelFromFile("dual-level.yaml")
	require.NoError(t, err)

	graph, _ := model.ToGraph(nil)
	require.NotNil(t, graph.Domains)

	cfg := &Config{
		Plugin:     topology.TopologyBlock,
		BlockSizes: []int{9, 144},
	}
	nt, err := NewNetworkTopology(graph, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.toBlockTopology(&buf, false))

	expected := strings.Join([]string{
		"# block001=rack-1-01",
		"BlockName=block001 Nodes=node[0001-0009]",
		"# block002=rack-1-02",
		"BlockName=block002 Nodes=node[0010-0018]",
		"# block003=rack-1-04",
		"BlockName=block003 Nodes=node[0028-0030,0032,0034-0036]",
		"# block004=rack-1-05",
		"BlockName=block004 Nodes=node[0037-0045]",
		"# block005=rack-1-06",
		"BlockName=block005 Nodes=node[0046-0054]",
		"# block006=rack-1-07",
		"BlockName=block006 Nodes=node[0055-0063]",
		"# block007=rack-1-08",
		"BlockName=block007 Nodes=node[0064-0072]",
		"# block008=rack-1-09",
		"BlockName=block008 Nodes=node[0073-0081]",
		"# block009=rack-1-10",
		"BlockName=block009 Nodes=node[0082-0090]",
		"# block010=rack-1-11",
		"BlockName=block010 Nodes=node[0091-0099]",
		"# block011=rack-1-12",
		"BlockName=block011 Nodes=node[0100-0108]",
		"# block012=rack-1-14",
		"BlockName=block012 Nodes=node[0118-0126]",
		"# block013=rack-1-15",
		"BlockName=block013 Nodes=node[0127-0135]",
		"# block014=rack-1-16",
		"BlockName=block014 Nodes=node[0136-0144]",
		"BlockName=block015",
		"BlockName=block016",
		"# block017=rack-2-01",
		"BlockName=block017 Nodes=node[0145-0153]",
		"# block018=rack-2-02",
		"BlockName=block018 Nodes=node[0154-0162]",
		"# block019=rack-2-03",
		"BlockName=block019 Nodes=node[0163-0171]",
		"# block020=rack-2-04",
		"BlockName=block020 Nodes=node[0172-0180]",
		"# block021=rack-2-05",
		"BlockName=block021 Nodes=node[0181-0189]",
		"# block022=rack-2-06",
		"BlockName=block022 Nodes=node[0190-0198]",
		"# block023=rack-2-07",
		"BlockName=block023 Nodes=node[0199-0207]",
		"# block024=rack-2-08",
		"BlockName=block024 Nodes=node[0208-0216]",
		"# block025=rack-2-09",
		"BlockName=block025 Nodes=node[0217-0225]",
		"# block026=rack-2-10",
		"BlockName=block026 Nodes=node[0226-0234]",
		"# block027=rack-2-12",
		"BlockName=block027 Nodes=node[0244-0252]",
		"# block028=rack-2-13",
		"BlockName=block028 Nodes=node[0253-0261]",
		"# block029=rack-2-14",
		"BlockName=block029 Nodes=node[0262-0270]",
		"# block030=rack-2-15",
		"BlockName=block030 Nodes=node[0271-0275]",
		"# block031=rack-2-16",
		"BlockName=block031 Nodes=node[0280-0288]",
		"BlockName=block032",
		"BlockSizes=9,144",
		"",
	}, "\n")
	require.Equal(t, expected, buf.String())
}

// getBlockWithIBAsymmetricSpineTestSet models two spines with four leaf switches on the
// left spine and three on the right, each leaf switch hosting one accelerator domain.
func getBlockWithIBAsymmetricSpineTestSet() (*topology.Graph, map[string]string) {
	n := func(id, name string) *topology.Vertex {
		return &topology.Vertex{ID: id, Name: name}
	}

	leaf := func(id string, nodes map[string]*topology.Vertex) *topology.Vertex {
		return &topology.Vertex{ID: id, Vertices: nodes}
	}

	l11 := leaf("L11", map[string]*topology.Vertex{"I11a": n("I11a", "Node101"), "I11b": n("I11b", "Node102"), "I11c": n("I11c", "Node103")})
	l12 := leaf("L12", map[string]*topology.Vertex{"I12a": n("I12a", "Node201"), "I12b": n("I12b", "Node202"), "I12c": n("I12c", "Node205")})
	l13 := leaf("L13", map[string]*topology.Vertex{"I13a": n("I13a", "Node301"), "I13b": n("I13b", "Node302"), "I13c": n("I13c", "Node303")})
	l14 := leaf("L14", map[string]*topology.Vertex{"I14a": n("I14a", "Node401"), "I14b": n("I14b", "Node402"), "I14c": n("I14c", "Node403")})
	l21 := leaf("L21", map[string]*topology.Vertex{"I21a": n("I21a", "Node501"), "I21b": n("I21b", "Node502"), "I21c": n("I21c", "Node503")})
	l22 := leaf("L22", map[string]*topology.Vertex{"I22a": n("I22a", "Node601"), "I22b": n("I22b", "Node602"), "I22c": n("I22c", "Node603")})
	l23 := leaf("L23", map[string]*topology.Vertex{"I23a": n("I23a", "Node701"), "I23b": n("I23b", "Node702"), "I23c": n("I23c", "Node703")})

	spine1 := &topology.Vertex{ID: "SP1", Vertices: map[string]*topology.Vertex{"L11": l11, "L12": l12, "L13": l13, "L14": l14}}
	spine2 := &topology.Vertex{ID: "SP2", Vertices: map[string]*topology.Vertex{"L21": l21, "L22": l22, "L23": l23}}
	core := &topology.Vertex{Vertices: map[string]*topology.Vertex{"SP1": spine1, "SP2": spine2}}

	domains := testDomainMap(map[string]map[string]string{
		"B1": {"Node101": "I11a", "Node102": "I11b", "Node103": "I11c"},
		"B2": {"Node201": "I12a", "Node202": "I12b", "Node205": "I12c"},
		"B3": {"Node301": "I13a", "Node302": "I13b", "Node303": "I13c"},
		"B4": {"Node401": "I14a", "Node402": "I14b", "Node403": "I14c"},
		"B5": {"Node501": "I21a", "Node502": "I21b", "Node503": "I21c"},
		"B6": {"Node601": "I22a", "Node602": "I22b", "Node603": "I22c"},
		"B7": {"Node701": "I23a", "Node702": "I23b", "Node703": "I23c"},
	})

	return &topology.Graph{Tiers: core, Domains: domains}, nil
}
