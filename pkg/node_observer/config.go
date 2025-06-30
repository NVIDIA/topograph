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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	TopologyGeneratorURL string            `yaml:"topology_generator_url"`
	NodeLabels           map[string]string `yaml:"node_labels"`
	Provider             string            `yaml:"provider"`
	Engine               string            `yaml:"engine"`
	Params               map[string]any    `yaml:"params"`
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

	if len(cfg.TopologyGeneratorURL) == 0 {
		return nil, fmt.Errorf("must specify topology_generator_url")
	}

	return cfg, nil
}
