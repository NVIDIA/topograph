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

package cw

import (
	"context"
	"fmt"
	"os/exec"

	v1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/topograph/pkg/ib"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "cw"

type Provider struct{}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	return New()
}

func New() (*Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, fmt.Errorf("CW does not support mult-region topology requests")
	}

	cmd := exec.CommandContext(ctx, "ibnetdiscover")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return ib.GenerateTopologyConfig(output)
}

// Engine support

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion() (string, error) {
	return "", nil
}

// GetNodeRegion implements k8s.k8sNodeInfo
func (p *Provider) GetNodeRegion(node *v1.Node) (string, error) {
	return node.Labels["topology.kubernetes.io/region"], nil
}

// GetNodeInstance implements k8s.k8sNodeInfo
func (p *Provider) GetNodeInstance(node *v1.Node) (string, error) {
	return node.Labels["kubernetes.io/hostname"], nil
}
