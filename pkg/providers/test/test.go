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

package test

import (
	"context"
	"fmt"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
	"k8s.io/klog/v2"
)

const NAME = "test"

type Provider struct {
	tree          *topology.Vertex
	instance2node map[string]string
}

type Params struct {
	ModelPath string `mapstructure:"model_path"`
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	return New(config)
}

func New(cfg providers.Config) (*Provider, error) {
	var p Params
	if err := config.Decode(cfg.Params, &p); err != nil {
		return nil, fmt.Errorf("error decoding params: %w", err)
	}
	provider := &Provider{}

	if len(p.ModelPath) == 0 {
		provider.tree, provider.instance2node = translate.GetTreeTestSet(false)
	} else {
		klog.InfoS("Using simulated topology", "model path", p.ModelPath)
		model, err := models.NewModelFromFile(p.ModelPath)
		if err != nil {
			return nil, err // Wrapped by models.NewModelFromFile
		}
		provider.tree, provider.instance2node = model.ToTree()
	}
	return provider, nil
}

func (p *Provider) GetComputeInstances(_ context.Context) ([]topology.ComputeInstances, error) {
	return []topology.ComputeInstances{
		{
			Instances: p.instance2node,
		},
	}, nil
}

func (p *Provider) GenerateTopologyConfig(_ context.Context, _ int, _ []topology.ComputeInstances) (*topology.Vertex, error) {
	return p.tree, nil
}
