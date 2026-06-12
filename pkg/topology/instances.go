/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package topology

import (
	"fmt"
)

// Instances is the top-level JSON envelope for instance-oriented topology export.
type Instances struct {
	Instances []Instance `json:"instances"`
}

// Instance describes one compute instance and its discovered network / accelerator context.
// YAML and JSON use the same field names for this exported shape.
type Instance struct {
	ID            string            `json:"id" yaml:"id,omitempty"`
	Type          string            `json:"type" yaml:"type,omitempty"`
	NetworkLayers []string          `json:"network_layers" yaml:"network_layers,omitempty"`
	Attributes    NodeAttributes    `json:"attributes" yaml:"attributes"`
	CapacityBlock string            `json:"capacity_block,omitempty" yaml:"capacity_block,omitempty"`
	Metadata      map[string]string `yaml:"-" json:"-"`
	NetLayers     []string          `yaml:"-" json:"-"`
}

// NodeAttributes holds per-instance accelerator metadata.
type NodeAttributes struct {
	NVLink string `json:"nvlink,omitempty" yaml:"nvlink,omitempty"`
}

// String summarizes an instance for logging (simulation / derived fields).
func (inst *Instance) String() string {
	return fmt.Sprintf("Instance: %s Metadata: %v NetLayers: %v Attr: %+v",
		inst.ID, inst.Metadata, inst.NetLayers, inst.Attributes)
}

// CloneForTopology returns a copy suitable for instance export: network layers are
// filled and simulation-only fields (capacity block, metadata) are cleared.
func (inst *Instance) CloneForTopology() Instance {
	return Instance{
		ID:            inst.ID,
		Type:          inst.Type,
		NetworkLayers: append([]string(nil), inst.NetLayers...),
		Attributes:    inst.Attributes,
	}
}
