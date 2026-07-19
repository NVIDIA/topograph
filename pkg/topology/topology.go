/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package topology

import (
	"fmt"
	"strings"
)

const (
	KeyEngine = "engine"

	KeyUID               = "uid"
	KeyNamespace         = "namespace"
	KeyPodSelector       = "podSelector"
	KeyNodeSelector      = "nodeSelector"
	KeyTopologies        = "topologies"
	KeyTopoConfigPath    = "topologyConfigPath"
	KeyTopoConfigmapName = "topologyConfigmapName"
	KeyBlockSizes        = "blockSizes"
	KeyTrimTiers         = "trimTiers"

	KeyPlugin     = "plugin"
	TopologyTree  = "topology/tree"
	TopologyBlock = "topology/block"
	TopologyFlat  = "topology/flat"
	NoTopology    = "no-topology"

	KeyNodeInstance = "topograph.nvidia.com/instance"
	KeyNodeRegion   = "topograph.nvidia.com/region"
	KeyGpuClusterID = "topograph.nvidia.com/cluster-id"

	// NVIDIA GPU Operator node labels
	KeyNvidiaGPUClique  = "nvidia.com/gpu.clique"
	KeyNvidiaGPUProduct = "nvidia.com/gpu.product"

	// Topograph default node labels. Fabric tier zero is closest to the compute
	// node.
	KeyFabricTierPrefix    = "network.topology.nvidia.com/tier-"
	KeyTopologyAccelerator = "network.topology.nvidia.com/accelerator"

	// ConfigMap annotation keys for metadata tracking
	KeyConfigMapEngine            = "topograph.nvidia.com/engine"
	KeyConfigMapTopologyManagedBy = "topograph.nvidia.com/topology-managed-by"
	KeyConfigMapLastUpdated       = "topograph.nvidia.com/last-updated"
	KeyConfigMapPlugin            = "topograph.nvidia.com/plugin"
	KeyConfigMapBlockSizes        = "topograph.nvidia.com/block-sizes"
	KeyConfigMapNamespace         = "topograph.nvidia.com/slurm-namespace"

	//Slinky specific annotations and labels
	KeySlinkyTopologySpec = "topology.slinky.slurm.net/spec"
	KeySlurmNodeName      = "slurm.node.name"
)

// Graph is the canonical scheduler-agnostic topology representation.
// Tiers is the root of the switch hierarchy. Domains maps accelerator/block
// domains to hosts and carries the finalized enumerated domain ID.
type Graph struct {
	Tiers   *Vertex
	Domains DomainMap
	// Instances optionally carries per-instance metadata keyed by instance ID.
	// Engines that do not need instance-oriented output ignore it.
	Instances map[string]Instance
}

// Vertex is a tree node, representing a compute node or a network switch, where
// - Name is a compute node name
// - ID is an CSP defined instance ID of switches and compute nodes
// - Vertices is a list of connected compute nodes or network switches
type Vertex struct {
	Name     string
	ID       string
	Vertices map[string]*Vertex
}

func (v *Vertex) String() string {
	vertices := []string{}
	for _, w := range v.Vertices {
		vertices = append(vertices, w.ID)
	}
	return fmt.Sprintf("ID:%q Name:%q Vertices: %s", v.ID, v.Name, strings.Join(vertices, ","))
}
