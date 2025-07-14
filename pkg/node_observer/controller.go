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

package node_observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type Controller struct {
	ctx          context.Context
	client       kubernetes.Interface
	nodeInformer *NodeInformer
}

func NewController(ctx context.Context, client kubernetes.Interface, cfg *Config) *Controller {
	var f httpreq.RequestFunc = func() (*http.Request, error) {
		payload := topology.NewRequest(cfg.Provider, nil, cfg.Engine, cfg.Params)
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to parse payload: %v", err)
		}
		req, err := http.NewRequest("POST", cfg.TopologyGeneratorURL, bytes.NewBuffer(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	return &Controller{
		ctx:          ctx,
		client:       client,
		nodeInformer: NewNodeInformer(ctx, client, &cfg.Trigger, f),
	}
}

func (c *Controller) Start() error {
	klog.Infof("Starting state observer")
	return c.nodeInformer.Start()
}

func (c *Controller) Stop(err error) {
	klog.Infof("Stopping state observer")
	c.nodeInformer.Stop(err)
}
