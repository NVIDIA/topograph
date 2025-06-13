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

package k8s

import (
	"bytes"
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const NAME = "k8s"

type K8sEngine struct {
	config *rest.Config
	client *kubernetes.Clientset
}

type Params struct {
	Method             string `mapstructure:"method"`
	ConfigPath         string `mapstructure:"topology_config_path"`
	ConfigMapName      string `mapstructure:"topology_configmap_name"`
	ConfigMapNamespace string `mapstructure:"topology_configmap_namespace"`
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config engines.Config) (engines.Engine, error) {
	return New()
}

func New() (*K8sEngine, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &K8sEngine{
		config: config,
		client: client,
	}, nil
}

func getParameters(params map[string]any) (*Params, error) {
	p := &Params{}
	if err := config.Decode(params, p); err != nil {
		return nil, err
	}

	if len(p.Method) == 0 {
		p.Method = topology.MethodLabels
	}

	switch p.Method {
	case topology.MethodLabels:
		// nop
	case topology.MethodSlurm:
		for key, val := range map[string]string{
			topology.KeyTopoConfigPath:         p.ConfigPath,
			topology.KeyTopoConfigmapName:      p.ConfigMapName,
			topology.KeyTopoConfigmapNamespace: p.ConfigMapNamespace,
		} {
			if len(val) == 0 {
				return nil, fmt.Errorf("must specify engine parameter %q with %s method", key, p.Method)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported method %q", p.Method)
	}

	return p, nil
}

func (eng *K8sEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	p, err := getParameters(params)
	if err != nil {
		return nil, err
	}

	if p.Method == topology.MethodSlurm {
		buf := &bytes.Buffer{}
		if err = translate.Write(buf, tree); err != nil {
			return nil, err
		}
		err = eng.UpdateTopologyConfigmap(ctx, p.ConfigMapName, p.ConfigMapNamespace, map[string]string{p.ConfigPath: buf.String()})
		if err != nil {
			return nil, err
		}
	} else if err := NewTopologyLabeler().ApplyNodeLabels(ctx, tree, eng); err != nil {
		return nil, err
	}

	return []byte("OK\n"), nil
}
