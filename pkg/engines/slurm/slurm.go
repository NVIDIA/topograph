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

package slurm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/files"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const TopologyHeader = `
###############################################################
# Slurm's network topology configuration file for use with the
# %s plugin
###############################################################
`

const NAME = "slurm"

type SlurmEngine struct{}

type Params struct {
	Plugin         string `mapstructure:"plugin"`
	TopoConfigPath string `mapstructure:"topology_config_path"`
	BlockSizes     string `mapstructure:"block_sizes"`
	SkipReload     string `mapstructure:"skip_reload"` // TODO: Should this be a bool
}

type instanceMapper interface {
	Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error)
	GetComputeInstancesRegion() (string, error)
}

var ErrEnvironmentUnsupported = errors.New("environment must implement instanceMapper")

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config engines.Config) (engines.Engine, error) {
	return New()
}

func New() (*SlurmEngine, error) {
	return &SlurmEngine{}, nil
}

func (eng *SlurmEngine) GetComputeInstances(ctx context.Context, environment engines.Environment) ([]topology.ComputeInstances, error) {
	instanceMapper, ok := environment.(instanceMapper)
	if !ok {
		return nil, ErrEnvironmentUnsupported
	}

	nodes, err := GetNodeList(ctx)
	if err != nil {
		return nil, err
	}

	i2n, err := instanceMapper.Instances2NodeMap(ctx, nodes)
	if err != nil {
		return nil, err
	}

	region, err := instanceMapper.GetComputeInstancesRegion()
	if err != nil {
		return nil, err
	}

	return []topology.ComputeInstances{{
		Region:    region,
		Instances: i2n,
	}}, nil
}

func GetNodeList(ctx context.Context) ([]string, error) {
	stdout, err := exec.Exec(ctx, "scontrol", []string{"show", "nodes", "-o"}, nil)
	if err != nil {
		return nil, err
	}

	klog.V(4).Infof("stdout: %s", stdout.String())

	nodes := []string{}
	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), " ")
		if len(arr) > 0 {
			line := arr[0]
			if strings.HasPrefix(line, "NodeName=") {
				nodes = append(nodes, line[9:])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed scan output: %v", err)
	}

	return nodes, nil
}

func (eng *SlurmEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	return GenerateOutput(ctx, tree, params)
}

func GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	var p Params
	if err := config.Decode(params, &p); err != nil {
		return nil, err
	}

	return GenerateOutputParams(ctx, tree, &p)
}

func GenerateOutputParams(ctx context.Context, tree *topology.Vertex, params *Params) ([]byte, error) {
	buf := &bytes.Buffer{}
	path := params.TopoConfigPath

	if len(path) != 0 {
		var plugin string
		if len(tree.Metadata) != 0 {
			plugin = tree.Metadata[topology.KeyPlugin]
		}
		if len(plugin) == 0 {
			plugin = topology.TopologyTree
		}
		if _, err := buf.WriteString(fmt.Sprintf(TopologyHeader, plugin)); err != nil {
			return nil, err
		}
	}

	blockSize := params.BlockSizes

	if len(blockSize) != 0 {
		tree.Metadata[topology.KeyBlockSizes] = blockSize
	}

	err := translate.ToGraph(buf, tree)
	if err != nil {
		return nil, err
	}

	cfg := buf.Bytes()

	if len(path) == 0 {
		return cfg, nil
	}

	klog.Infof("Writing topology config in %q", path)
	if err = files.Create(path, cfg); err != nil {
		return nil, err
	}
	if len(params.SkipReload) > 0 {
		klog.Infof("Skip SLURM reconfiguration")
	} else {
		if err = reconfigure(ctx); err != nil {
			return nil, err
		}
	}

	return []byte("OK\n"), nil
}

func reconfigure(ctx context.Context) error {
	stdout, err := exec.Exec(ctx, "scontrol", []string{"reconfigure"}, nil)
	if err != nil {
		return err
	}

	klog.V(4).Infof("stdout: %s", stdout.String())

	return nil
}
