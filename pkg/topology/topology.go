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
	"fmt"
	"strings"
)

const (
	KeyEngine = "engine"

	KeyUID               = "uid"
	KeyNamespace         = "namespace"
	KeyPodLabel          = "pod_label"
	KeyTopoConfigPath    = "topology_config_path"
	KeyTopoConfigmapName = "topology_configmap_name"
	KeyBlockSizes        = "block_sizes"

	KeyPlugin     = "plugin"
	TopologyTree  = "topology/tree"
	TopologyBlock = "topology/block"
	TopologyFlat  = "topology/flat"
	NoTopology    = "no-topology"

	KeyNodeInstance = "topograph.nvidia.com/instance"
	KeyNodeRegion   = "topograph.nvidia.com/region"

	// ConfigMap annotation keys for metadata tracking
	KeyConfigMapEngine            = "topograph.nvidia.com/engine"
	KeyConfigMapTopologyManagedBy = "topograph.nvidia.com/topology-managed-by"
	KeyConfigMapLastUpdated       = "topograph.nvidia.com/last-updated"
	KeyConfigMapPlugin            = "topograph.nvidia.com/plugin"
	KeyConfigMapBlockSizes        = "topograph.nvidia.com/block-sizes"
	KeyConfigMapNamespace         = "topograph.nvidia.com/slurm-namespace"
)

// Vertex is a tree node, representing a compute node or a network switch, where
// - Name is a compute node name
// - ID is an CSP defined instance ID of switches and compute nodes
// - Vertices is a list of connected compute nodes or network switches
type Vertex struct {
	Name     string
	ID       string
	Vertices map[string]*Vertex
	Metadata map[string]string
}

func (v *Vertex) String() string {
	vertices := []string{}
	for _, w := range v.Vertices {
		vertices = append(vertices, w.ID)
	}
	return fmt.Sprintf("ID:%q Name:%q Vertices: %s", v.ID, v.Name, strings.Join(vertices, ","))
}
