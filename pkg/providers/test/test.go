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
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
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

func Loader(_ context.Context, cfg providers.Config) (providers.Provider, *httperr.Error) {
	var p Params
	if err := config.Decode(cfg.Params, &p); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, fmt.Sprintf("error decoding params: %v", err))
	}
	provider := &Provider{}

	if len(p.ModelPath) == 0 {
		provider.tree, provider.instance2node = translate.GetTreeTestSet(false)
	} else {
		klog.InfoS("Using simulated topology", "model path", p.ModelPath)
		model, err := models.NewModelFromFile(p.ModelPath)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadRequest, err.Error())
		}
		provider.tree, provider.instance2node = model.ToGraph()
	}
	return provider, nil
}

func (p *Provider) GetComputeInstances(_ context.Context) ([]topology.ComputeInstances, *httperr.Error) {
	return []topology.ComputeInstances{
		{
			Instances: p.instance2node,
		},
	}, nil
}

func (p *Provider) GenerateTopologyConfig(_ context.Context, _ *int, _ []topology.ComputeInstances) (*topology.Vertex, *httperr.Error) {
	return p.tree, nil
}
