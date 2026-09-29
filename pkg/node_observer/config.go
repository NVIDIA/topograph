/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"fmt"
	"maps"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	kubernetesprovider "github.com/dsx-ai-factory/topograph/pkg/providers/kubernetes"
	"github.com/dsx-ai-factory/topograph/pkg/topology"
)

const (
	defaultRetryDelay             = 5 * time.Minute
	defaultBrokerRetryDelay       = 10 * time.Second
	defaultAPIServerContainerName = "topograph"
)

type Config struct {
	GenerateTopologyURL string            `yaml:"generateTopologyUrl"`
	Trigger             Trigger           `yaml:"trigger"`
	APIServer           APIServer         `yaml:"apiServer,omitempty"`
	Provider            topology.Provider `yaml:"provider"`
	Engine              topology.Engine   `yaml:"engine"`
	RetryDelay          metav1.Duration   `yaml:"retryDelay"`
}

type Trigger struct {
	nodeTopologyLabels []string
	NodeSelector       map[string]string     `yaml:"nodeSelector,omitempty"`
	PodSelector        *metav1.LabelSelector `yaml:"podSelector,omitempty"`
}

type APIServer struct {
	Namespace     string                `yaml:"namespace,omitempty"`
	PodSelector   *metav1.LabelSelector `yaml:"podSelector,omitempty"`
	ContainerName string                `yaml:"containerName,omitempty"`
}

func NewConfigFromFile(fname string) (*Config, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", fname, err)
	}

	if len(cfg.GenerateTopologyURL) == 0 {
		return nil, fmt.Errorf("must specify generateTopologyUrl")
	}

	if cfg.APIServer.PodSelector != nil && len(cfg.APIServer.ContainerName) == 0 {
		cfg.APIServer.ContainerName = defaultAPIServerContainerName
	}

	if len(cfg.Trigger.NodeSelector) == 0 && cfg.Trigger.PodSelector == nil && cfg.APIServer.PodSelector == nil && cfg.Provider.Name != kubernetesprovider.NAME {
		return nil, fmt.Errorf("must specify nodeSelector and/or podSelector in trigger, or apiServer.podSelector")
	}

	if _, err := cfg.nodeTrigger(); err != nil {
		return nil, err
	}

	if cfg.RetryDelay.Duration == 0 {
		cfg.RetryDelay.Duration = defaultRetryDelay
	}
	return cfg, nil
}

func (cfg *Config) nodeTrigger() (*Trigger, error) {
	trigger := cfg.Trigger
	if cfg.Provider.Name == kubernetesprovider.NAME {
		params, err := kubernetesprovider.ParseParams(cfg.Provider.Params)
		if err != nil {
			return nil, err
		}
		if len(trigger.NodeSelector) != 0 && !maps.Equal(trigger.NodeSelector, params.NodeSelector) {
			return nil, fmt.Errorf("trigger.nodeSelector must match provider.params.nodeSelector for the kubernetes provider")
		}
		trigger.NodeSelector = params.NodeSelector
		trigger.nodeTopologyLabels = params.TopologyLabels
	}
	return &trigger, nil
}
