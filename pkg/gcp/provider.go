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

package gcp

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/k8s"
	"github.com/NVIDIA/topograph/pkg/slurm"
)

type Provider struct{}

func GetProvider() (*Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) GetCredentials(_ *common.Credentials) (interface{}, error) {
	return nil, nil
}

func (p *Provider) GetComputeInstances(ctx context.Context, engine common.Engine) ([]common.ComputeInstances, error) {
	klog.InfoS("Getting compute instances", "provider", common.ProviderGCP, "engine", engine)

	switch eng := engine.(type) {
	case *slurm.SlurmEngine:
		nodes, err := slurm.GetNodeList(ctx)
		if err != nil {
			return nil, err
		}
		i2n := make(map[string]string)
		for _, node := range nodes {
			i2n[node] = node
		}
		return []common.ComputeInstances{{Instances: i2n}}, nil
	case *k8s.K8sEngine:
		return eng.GetComputeInstances(ctx,
			func(n *v1.Node) string { return n.Labels["topology.kubernetes.io/region"] },
			func(n *v1.Node) string { return n.Labels["kubernetes.io/hostname"] })
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, creds interface{}, _ int, instances []common.ComputeInstances) (*common.Vertex, error) {
	if len(instances) > 1 {
		return nil, fmt.Errorf("GCP does not support mult-region topology requests")
	}

	var instanceToNode map[string]string
	if len(instances) == 1 {
		instanceToNode = instances[0].Instances
	}

	cfg, err := GenerateInstanceTopology(ctx, creds, instanceToNode)
	if err != nil {
		return nil, err
	}

	return cfg.toSLURM()
}
