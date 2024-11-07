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

package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/topograph/internal/component"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type Provider interface {
	GenerateTopologyConfig(ctx context.Context, pageSize int, instances []topology.ComputeInstances) (*topology.Vertex, error)
}

type Config struct {
	Creds  map[string]string
	Params map[string]any
}
type NamedLoader = component.NamedLoader[Provider, Config]
type Loader = component.Loader[Provider, Config]
type Registry component.Registry[Provider, Config]

var ErrUnsupportedProvider = errors.New("unsupported provider")

func NewRegistry(namedLoaders ...NamedLoader) Registry {
	return Registry(component.NewRegistry(namedLoaders...))
}

func (r Registry) Get(name string) (Loader, error) {
	loader, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q, %w", name, ErrUnsupportedProvider)
	}

	return loader, nil
}
