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
	NetworkLayers []string          `json:"network_layers" yaml:"network_layers,omitempty"`
	Labels        map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	NetLayers     []string          `yaml:"-" json:"-"`
}

func (inst Instance) AcceleratorID() string {
	return AcceleratorID(inst.Labels)
}

func AcceleratorID(labels map[string]string) string {
	if accelerator := labels[KeyTopologyAccelerator]; accelerator != "" {
		return accelerator
	}
	return labels[KeyNvidiaGPUClique]
}

// String summarizes an instance for logging (simulation / derived fields).
func (inst *Instance) String() string {
	return fmt.Sprintf("Instance: %s Labels: %v NetLayers: %v",
		inst.ID, inst.Labels, inst.NetLayers)
}

// CloneForTopology returns a copy suitable for instance export: network layers
// are filled and simulation-only fields are cleared.
func (inst *Instance) CloneForTopology() Instance {
	return Instance{
		ID:            inst.ID,
		NetworkLayers: append([]string(nil), inst.NetLayers...),
		Labels:        cloneStringMap(inst.Labels),
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for k, v := range values {
		clone[k] = v
	}
	return clone
}
