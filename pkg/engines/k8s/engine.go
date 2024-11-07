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

	k8s_core_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const NAME = "k8s"

type K8sEngine struct {
	kubeClient *kubernetes.Clientset
}

type Params struct {
	TopoConfigPath         string `mapstructure:"topology_config_path"`
	TopoConfigmapName      string `mapstructure:"topology_configmap_name"`
	TopoConfigmapNamespace string `mapstructure:"topology_configmap_namespace"`
}

type k8sNodeInfo interface {
	GetNodeRegion(node *k8s_core_v1.Node) (string, error)
	GetNodeInstance(node *k8s_core_v1.Node) (string, error)
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

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &K8sEngine{kubeClient: kubeClient}, nil
}

func (eng *K8sEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	if err := NewTopologyLabeler().ApplyNodeLabels(ctx, tree, eng); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	err := translate.ToGraph(buf, tree)
	if err != nil {
		return nil, err
	}

	var p Params
	if err := config.Decode(params, &p); err != nil {
		return nil, err
	}

	cfg := buf.Bytes()

	filename := p.TopoConfigPath
	cmName := p.TopoConfigmapName
	cmNamespace := p.TopoConfigmapNamespace
	err = eng.UpdateTopologyConfigmap(ctx, cmName, cmNamespace, map[string]string{filename: string(cfg)})
	if err != nil {
		return nil, err
	}

	return []byte("OK\n"), nil
}
