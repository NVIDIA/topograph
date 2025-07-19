/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (nt *NetworkTopology) initTree(root *topology.Vertex) {
	tree, ok := root.Vertices[topology.TopologyTree]
	if !ok {
		return
	}

	queue := []*topology.Vertex{tree}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		_, ok := nt.tree[v.ID]
		if !ok {
			nt.tree[v.ID] = []string{}
			nt.vertices[v.ID] = v
			if len(v.Vertices) == 0 {
				nt.nodeInfo[v.Name] = &nodeInfo{instanceID: v.ID}
			}
		}
		for id, w := range v.Vertices {
			nt.tree[v.ID] = append(nt.tree[v.ID], id)
			queue = append(queue, w)
		}
	}

	for _, val := range nt.tree {
		sort.Strings(val)
	}
}

// toTreeTopology generates SLURM cluster topology config in "topology/tree" format
func (nt *NetworkTopology) toTreeTopology(wr io.Writer) error {
	if len(nt.tree) == 0 {
		return fmt.Errorf("missing tree topology")
	}
	queue := []string{""}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		v, ok := nt.vertices[id]
		if !ok {
			return fmt.Errorf("missing vertex with ID %q", id)
		}
		if err := writeVertex(wr, v); err != nil {
			return err
		}
		queue = append(queue, nt.tree[id]...)
	}
	return nil
}

func writeVertex(wr io.Writer, v *topology.Vertex) error {
	if len(v.ID) == 0 {
		return nil
	}

	switches := make([]string, 0, len(v.Vertices))
	nodes := make([]string, 0, len(v.Vertices))
	for _, node := range v.Vertices {
		if node.Name == "" {
			switches = append(switches, node.ID)
		} else {
			nodes = append(nodes, node.Name)
		}
	}

	var comment, name string
	if len(v.Name) == 0 {
		name = v.ID
	} else {
		comment = fmt.Sprintf("# %s=%s\n", v.Name, v.ID)
		name = v.Name
	}

	if len(switches) != 0 {
		_, err := fmt.Fprintf(wr, "%sSwitchName=%s Switches=%s\n", comment, name, strings.Join(cluset.Compact(switches), ","))
		if err != nil {
			return err
		}
	}

	if len(nodes) != 0 {
		_, err := fmt.Fprintf(wr, "%sSwitchName=%s Nodes=%s\n", comment, name, strings.Join(cluset.Compact(nodes), ","))
		if err != nil {
			return err
		}
	}
	return nil
}
