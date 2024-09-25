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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/translate"
)

type K8sEngine struct {
	kubeClient *kubernetes.Clientset
}

func GetK8sEngine() (*K8sEngine, error) {
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

func (eng *K8sEngine) GenerateOutput(ctx context.Context, tree *common.Vertex, params map[string]string) ([]byte, error) {
	if err := NewTopologyLabeler().ApplyNodeLabels(ctx, tree, eng); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	err := translate.ToSLURM(buf, tree)
	if err != nil {
		return nil, err
	}

	cfg := buf.Bytes()

	filename := params[common.KeyTopoConfigPath]
	cmName := params[common.KeyTopoConfigmapName]
	cmNamespace := params[common.KeyTopoConfigmapNamespace]
	err = eng.UpdateTopologyConfigmap(ctx, cmName, cmNamespace, map[string]string{filename: string(cfg)})
	if err != nil {
		return nil, err
	}

	return []byte("OK\n"), nil
}
