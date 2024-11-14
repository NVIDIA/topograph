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
	"errors"
	"fmt"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "test"

type TestEngine struct{}

var ErrEnvironmentUnsupported = errors.New("test engine does not support GetComputeInstances")

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config engines.Config) (engines.Engine, error) {
	return New()
}

func New() (*TestEngine, error) {
	return &TestEngine{}, nil
}

func (eng *TestEngine) GetComputeInstances(ctx context.Context, environment engines.Environment) ([]topology.ComputeInstances, error) {
	return nil, ErrEnvironmentUnsupported
}

func (eng *TestEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	if params == nil {
		params = make(map[string]any)
	}

	var p slurm.Params
	if err := config.Decode(params, &p); err != nil {
		return nil, err
	}

	if len(tree.Metadata) == 0 {
		return nil, fmt.Errorf("metadata for test engine not set")
	}

	if len(p.Plugin) != 0 {
		tree.Metadata[topology.KeyPlugin] = p.Plugin
	}
	if len(p.BlockSizes) != 0 {
		tree.Metadata[topology.KeyBlockSizes] = p.BlockSizes
	}
	return slurm.GenerateOutputParams(ctx, tree, &p)
}
