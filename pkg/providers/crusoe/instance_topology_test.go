/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const (
	partitionA = "0cf47922-9f9e-4a2e-b187-5c51697e7739"
	partitionB = "1d0e8b3a-2c47-4f16-9a55-7be3d9c4f082"
	podA1      = "9b1c7d54-3e08-4a92-8f61-2d5a4c7b0e13"
	podA2      = "4f6a2e91-8d75-4c30-b1e8-6a9c3f5d7b24"
	podB1      = "7c3d9f68-5b21-4e84-a0f3-9e1b8d2c6a57"
)

func gpuNode(name, partition, pod string) nodeMetadata {
	return nodeMetadata{
		InstanceID: name,
		Labels: map[string]string{
			labelPartitionID: partition,
			labelPodID:       pod,
		},
	}
}

func TestBuildInstanceTopology(t *testing.T) {
	testCases := []struct {
		name       string
		node       nodeMetadata
		wantTiers  []topology.FabricTier
		wantFabric bool
	}{
		{
			name:       "InfiniBand node maps pod, partition and root",
			node:       gpuNode("gpu-01", partitionA, podA1),
			wantTiers:  topology.ClosestFirstFabricTiers(podA1, partitionA, rootTier),
			wantFabric: true,
		},
		{
			name:      "node without InfiniBand sits under the same root",
			node:      nodeMetadata{InstanceID: "cpu-01"},
			wantTiers: topology.ClosestFirstFabricTiers(cpuPod, cpuPartition, rootTier),
		},
		{
			name: "a partition without a pod is not enough",
			node: nodeMetadata{
				InstanceID: "half-labelled",
				Labels:     map[string]string{labelPartitionID: partitionA},
			},
			wantTiers: topology.ClosestFirstFabricTiers(cpuPod, cpuPartition, rootTier),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, hasFabric := buildInstanceTopology(tc.node)
			require.Equal(t, tc.node.InstanceID, got.InstanceID)
			require.Equal(t, tc.wantTiers, got.FabricTiers)
			require.Equal(t, tc.wantFabric, hasFabric)
			// Block topology is out of scope for this provider, so it
			// contributes no accelerator domain.
			require.Empty(t, got.XclrDomainID)
			require.Empty(t, got.XclrSubDomainID)
		})
	}
}

func TestRequestedNodeIDs(t *testing.T) {
	testCases := []struct {
		name      string
		instances []topology.ComputeInstances
		want      map[string]struct{}
		errCode   int
	}{
		{
			name: "no instances means every selected node",
		},
		{
			name:      "an empty instance map means every selected node",
			instances: []topology.ComputeInstances{{Region: region}},
		},
		{
			name: "instances become a lookup set",
			instances: []topology.ComputeInstances{{
				Region:    region,
				Instances: map[string]string{"gpu-01": "gpu-01", "gpu-02": "gpu-02"},
			}},
			want: map[string]struct{}{"gpu-01": {}, "gpu-02": {}},
		},
		{
			name: "more than one region is rejected",
			instances: []topology.ComputeInstances{
				{Region: "one", Instances: map[string]string{"gpu-01": "gpu-01"}},
				{Region: "two", Instances: map[string]string{"gpu-02": "gpu-02"}},
			},
			errCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := requestedNodeIDs(tc.instances)
			if tc.errCode != 0 {
				require.NotNil(t, err)
				require.Equal(t, tc.errCode, err.Code())
				return
			}
			require.Nil(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestBuildClusterTopologyFiltersAndFailsEmpty(t *testing.T) {
	nodes := []nodeMetadata{
		gpuNode("gpu-01", partitionA, podA1),
		gpuNode("gpu-02", partitionA, podA1),
		{InstanceID: "cpu-01"},
	}

	topo, err := buildClusterTopology(nodes, nil)
	require.Nil(t, err)
	require.Equal(t, 3, topo.Len())

	topo, err = buildClusterTopology(nodes, map[string]struct{}{"gpu-02": {}})
	require.Nil(t, err)
	require.Equal(t, 1, topo.Len())
	require.Equal(t, "gpu-02", topo.Instances[0].InstanceID)

	_, err = buildClusterTopology(nodes, map[string]struct{}{"absent": {}})
	require.NotNil(t, err)
	require.Equal(t, http.StatusNotFound, err.Code())
}

// TestRenderedTreeTopologyConf pins the file Slurm reads, so a regression in
// tier construction shows up as a topology.conf diff and not only in the graph.
func TestRenderedTreeTopologyConf(t *testing.T) {
	nodes := []nodeMetadata{
		gpuNode("gpu-01", partitionA, podA1),
		gpuNode("gpu-02", partitionA, podA1),
		gpuNode("gpu-03", partitionA, podA2),
		gpuNode("gpu-04", partitionB, podB1),
		{InstanceID: "cpu-01"},
	}

	got := render(t, nodes, &translate.Config{Plugin: topology.TopologyTree})

	require.Contains(t, got, "SwitchName="+podA1+" Nodes=gpu-[01-02]")
	require.Contains(t, got, "SwitchName="+podA2+" Nodes=gpu-03")
	require.Contains(t, got, "SwitchName="+podB1+" Nodes=gpu-04")
	require.Contains(t, got, "SwitchName="+cpuPod+" Nodes=cpu-01")
	require.Contains(t, got, "SwitchName="+partitionA+" Switches="+podA2+","+podA1)
	// Both InfiniBand partitions and the CPU placeholder hang off one root,
	// which is what lets a Slurm job span GPU and CPU nodes.
	require.Contains(t, got,
		"SwitchName="+rootTier+" Switches="+partitionA+","+partitionB+","+cpuPartition)
}

func render(t *testing.T, nodes []nodeMetadata, cfg *translate.Config) string {
	t.Helper()

	topo, httpErr := buildClusterTopology(nodes, nil)
	require.Nil(t, httpErr)

	instances := make(map[string]string, len(nodes))
	for _, node := range nodes {
		instances[node.InstanceID] = node.InstanceID
	}
	graph := topo.ToGraph(NAME, []topology.ComputeInstances{
		{Region: region, Instances: instances},
	}, 0, false)

	nt, err := translate.NewNetworkTopology(graph, cfg)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.Nil(t, nt.Generate(&buf))
	return buf.String()
}
