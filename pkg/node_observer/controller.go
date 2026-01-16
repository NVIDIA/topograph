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
	ctx            context.Context
	client         kubernetes.Interface
	statusInformer *StatusInformer
}

func NewController(ctx context.Context, client kubernetes.Interface, cfg *Config) (*Controller, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	payload := topology.NewRequest(cfg.Provider, cfg.Engine)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	f := httpreq.GetRequestFunc(ctx, http.MethodPost, headers, nil, data, cfg.GenerateTopologyURL)
	statusInformer, err := NewStatusInformer(ctx, client, &cfg.Trigger, f)
	if err != nil {
		return nil, err
	}
	return &Controller{
		ctx:            ctx,
		client:         client,
		statusInformer: statusInformer,
	}, nil
}

func (c *Controller) Start() error {
	klog.Infof("Starting state observer")
	return c.statusInformer.Start()
}

func (c *Controller) Stop(err error) {
	klog.Infof("Stopping state observer")
	c.statusInformer.Stop(err)
}
