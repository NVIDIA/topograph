/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	nodeDataBrokerNameEnv      = "NODE_DATA_BROKER_NAME"
	nodeDataBrokerNamespaceEnv = "NODE_DATA_BROKER_NAMESPACE"
)

type Controller struct {
	ctx            context.Context
	cancel         context.CancelFunc
	client         kubernetes.Interface
	statusInformer *StatusInformer
	healthURL      string
}

func NewController(ctx context.Context, client kubernetes.Interface, cfg *Config) (*Controller, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	payload := topology.NewRequest(cfg.Provider, cfg.Engine)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	healthURL, err := healthCheckURL(cfg.GenerateTopologyURL)
	if err != nil {
		return nil, err
	}

	controllerCtx, cancel := context.WithCancel(ctx)
	f := httpreq.GetRequestFunc(controllerCtx, http.MethodPost, headers, nil, data, cfg.GenerateTopologyURL)
	statusInformer, err := NewStatusInformer(
		controllerCtx,
		client,
		&cfg.Trigger,
		&cfg.APIServer,
		os.Getenv(nodeDataBrokerNameEnv),
		os.Getenv(nodeDataBrokerNamespaceEnv),
		cfg.RetryDelay.Duration,
		f,
	)
	if err != nil {
		cancel()
		return nil, err
	}
	return &Controller{
		ctx:            controllerCtx,
		cancel:         cancel,
		client:         client,
		statusInformer: statusInformer,
		healthURL:      healthURL,
	}, nil
}

func (c *Controller) Start() error {
	klog.Infof("Waiting for topograph API to become ready")
	if err := waitForTopograph(c.ctx, c.healthURL, healthCheckInterval, healthCheckTimeout); err != nil {
		return err
	}

	klog.Infof("Starting state observer")
	return c.statusInformer.Start()
}

func (c *Controller) Stop(err error) {
	klog.Infof("Stopping state observer")
	c.cancel()
	c.statusInformer.Stop(err)
}
