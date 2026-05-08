/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package topology

import (
	"encoding/json"
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

type BasicNodeAttributes struct {
	NVLink string `json:"nvlink,omitempty" yaml:"nvlink,omitempty"`
}

// NodeAttributes holds per-instance accelerator metadata.
// YAML simulation models use a flat attributes map (nvlink, status, timestamp, gpus);
// JSON export nests GPU fields under "gpu".
type NodeAttributes struct {
	BasicNodeAttributes `yaml:",inline"`
	Status              string `yaml:"status,omitempty" json:"-"`
	CollectedAt         string `yaml:"timestamp,omitempty" json:"-"`
	GPUs                []GPU  `yaml:"gpus,omitempty" json:"-"`
}

// MarshalJSON encodes attributes in the API shape {"nvlink":"...","gpu":{...}}.
func (a NodeAttributes) MarshalJSON() ([]byte, error) {
	type wrap struct {
		BasicNodeAttributes
		GPU GPUAttribute `json:"gpu"`
	}
	return json.Marshal(wrap{
		BasicNodeAttributes: a.BasicNodeAttributes,
		GPU: GPUAttribute{
			Status:      a.Status,
			CollectedAt: a.CollectedAt,
			GPUs:        a.GPUs,
		},
	})
}

// UnmarshalJSON decodes the API attributes shape.
func (a *NodeAttributes) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var aux struct {
		BasicNodeAttributes
		GPU GPUAttribute `json:"gpu"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	a.BasicNodeAttributes = aux.BasicNodeAttributes
	a.Status = aux.GPU.Status
	a.CollectedAt = aux.GPU.CollectedAt
	a.GPUs = aux.GPU.GPUs
	return nil
}

type GPUAttribute struct {
	Status      string `json:"status"`
	CollectedAt string `json:"collected_at"`
	GPUs        []GPU  `json:"gpus,omitempty"`
}

// GPU is a single GPU in simulation models (YAML) and instance-oriented topology export (JSON).
type GPU struct {
	Index     int    `json:"index" yaml:"index"`
	PCIBusID  string `json:"pci_bus_id" yaml:"pci_bus_id"`
	UUID      string `json:"uuid" yaml:"uuid"`
	Model     string `json:"model" yaml:"model"`
	MemoryMiB int    `json:"memory_mib" yaml:"memory_mib"`
}

// String summarizes an instance for logging (simulation / derived fields).
func (inst *Instance) String() string {
	return fmt.Sprintf("Instance: %s Metadata: %v NetLayers: %v Attr: %+v",
		inst.ID, inst.Metadata, inst.NetLayers, inst.Attributes)
}

// CloneForTopology returns a copy suitable for instance export: network layers are
// filled and simulation-only fields (capacity block, metadata) are cleared.
func (inst *Instance) CloneForTopology() Instance {
	gpus := append([]GPU(nil), inst.Attributes.GPUs...)
	return Instance{
		ID:            inst.ID,
		Type:          inst.Type,
		NetworkLayers: append([]string(nil), inst.NetLayers...),
		Attributes: NodeAttributes{
			BasicNodeAttributes: inst.Attributes.BasicNodeAttributes,
			Status:              inst.Attributes.Status,
			CollectedAt:         inst.Attributes.CollectedAt,
			GPUs:                gpus,
		},
	}
}
