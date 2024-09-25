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

package state_observer

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
)

type Controller struct {
	ctx          context.Context
	client       kubernetes.Interface
	cfg          *Config
	nodeInformer *NodeInformer
}

func NewController(ctx context.Context, client kubernetes.Interface, cfg *Config) (*Controller, error) {
	req, err := http.NewRequest("POST", cfg.TopologyGeneratorURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	q := req.URL.Query()
	q.Add(common.KeyProvider, cfg.Provider)
	q.Add(common.KeyEngine, cfg.Engine)
	q.Add(common.KeyTopoConfigPath, cfg.TopologyConfigmap.Filename)
	q.Add(common.KeyTopoConfigmapName, cfg.TopologyConfigmap.Name)
	q.Add(common.KeyTopoConfigmapNamespace, cfg.TopologyConfigmap.Namespace)
	req.URL.RawQuery = q.Encode()

	return &Controller{
		ctx:          ctx,
		client:       client,
		cfg:          cfg,
		nodeInformer: NewNodeInformer(ctx, client, cfg.NodeLabels, req),
	}, nil
}

func (c *Controller) Start() error {
	klog.Infof("Starting state observer")

	return c.nodeInformer.Start()
}

func (c *Controller) Stop(err error) {
	klog.Infof("Stopping state observer")
	c.nodeInformer.Stop(err)
}
