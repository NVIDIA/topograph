/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package topology

import (
	"encoding/json"
	"fmt"
)

const (
	InstanceCacheKey = "gpu/v1/%s" // %s is the instance ID
)

// Instances is the top-level JSON envelope for instance-oriented topology export.
type Instances struct {
	Instances []Node `json:"instances"`
}

// Node describes one compute instance and its discovered network / accelerator context.
// The same type is used for simulation models (YAML) and API export (JSON); fields that
// apply only to one encoding are tagged accordingly.
type Node struct {
	ID            string            `json:"id" yaml:"name,omitempty"`
	InstanceType  string            `json:"instance_type" yaml:"type,omitempty"`
	Provider      string            `json:"provider" yaml:"provider,omitempty"`
	Region        string            `json:"region" yaml:"region,omitempty"`
	NetworkLayers []string          `json:"network_layers" yaml:"network_layers,omitempty"`
	NVLinkDomain  string            `json:"nvlink_domain" yaml:"-"`
	Attributes    NodeAttributes    `json:"attributes" yaml:"attributes"`
	CapacityBlock string            `json:"capacity_block,omitempty" yaml:"capacity_block_id,omitempty"`
	Metadata      map[string]string `yaml:"-" json:"-"`
	NetLayers     []string          `yaml:"-" json:"-"`
}

type BasicNodeAttributes struct {
	NVLink string `yaml:"nvlink,omitempty"`
}

// NodeAttributes holds per-instance accelerator metadata.
// YAML simulation models use a flat attributes map (nvlink, status, timestamp, gpus);
// JSON export nests GPU fields under "gpu".
type NodeAttributes struct {
	BasicNodeAttributes `yaml:",inline" json:"-"`
	Status              string `yaml:"status,omitempty" json:"-"`
	CollectedAt         string `yaml:"timestamp,omitempty" json:"-"`
	GPUs                []GPU  `yaml:"gpus,omitempty" json:"-"`
}

// MarshalJSON encodes attributes in the API shape {"gpu":{...}}.
func (a NodeAttributes) MarshalJSON() ([]byte, error) {
	type wrap struct {
		GPU GpuAttribute `json:"gpu"`
	}
	return json.Marshal(wrap{GPU: GpuAttribute{
		Status:      a.Status,
		CollectedAt: a.CollectedAt,
		GPUs:        a.GPUs,
	}})
}

// UnmarshalJSON decodes the API attributes shape.
func (a *NodeAttributes) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var aux struct {
		GPU GpuAttribute `json:"gpu"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	a.Status = aux.GPU.Status
	a.CollectedAt = aux.GPU.CollectedAt
	a.GPUs = aux.GPU.GPUs
	return nil
}

type GpuAttribute struct {
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

func CacheKey(instanceID string) string {
	return fmt.Sprintf(InstanceCacheKey, instanceID)
}

// String summarizes an instance for logging (simulation / derived fields).
func (inst *Node) String() string {
	return fmt.Sprintf("Instance: %s Metadata: %v NetLayers: %v Attr: %+v",
		inst.ID, inst.Metadata, inst.NetLayers, inst.Attributes)
}

// CloneForProvider returns a copy suitable for instance export: provider/region/layers
// and NVLinkDomain are filled; simulation-only fields (capacity block, metadata) are cleared.
func (inst *Node) CloneForProvider(provider string) Node {
	gpus := append([]GPU(nil), inst.Attributes.GPUs...)
	return Node{
		ID:            inst.ID,
		InstanceType:  inst.InstanceType,
		Provider:      provider,
		Region:        inst.Metadata["region"],
		NetworkLayers: append([]string(nil), inst.NetLayers...),
		NVLinkDomain:  inst.Attributes.NVLink,
		Attributes: NodeAttributes{
			Status:      inst.Attributes.Status,
			CollectedAt: inst.Attributes.CollectedAt,
			GPUs:        gpus,
		},
	}
}
