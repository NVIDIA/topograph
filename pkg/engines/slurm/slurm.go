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
	"regexp"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/files"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/metrics"
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
	Plugin           string `mapstructure:"plugin"`
	TopoConfigPath   string `mapstructure:"topology_config_path"`
	BlockSizes       string `mapstructure:"block_sizes"`
	Reconfigure      bool   `mapstructure:"reconfigure"`
	FakeNodesEnabled bool   `mapstructure:"fakeNodesEnabled"`
	FakeNodePool     string `mapstructure:"fake_node_pool"`
}

type instanceMapper interface {
	Instances2NodeMap(context.Context, []string) (map[string]string, error)
	GetComputeInstancesRegion(context.Context) (string, error)
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
	klog.V(4).Infof("Detected instance map: %v", i2n)

	region, err := instanceMapper.GetComputeInstancesRegion(ctx)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("Detected region: %s", region)

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

func GetFakeNodes(ctx context.Context) (string, error) {
	var fakeRange []string
	reFake := regexp.MustCompile(`^Nodes=(.*)\[(\d+)-(\d+)\]`)
	args := []string{"show", "partition", "fake"}
	stdout, err := exec.Exec(ctx, "scontrol", args, nil)
	if err != nil {
		return "", err
	}

	klog.V(4).Infof("stdout: %s", stdout.String())

	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	for scanner.Scan() {
		line := scanner.Text()
		fakeRange = reFake.FindStringSubmatch(line)
		if len(fakeRange) == 4 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan fake nodes partition: %v", err)
	}
	output := fmt.Sprintf("%s,%s,%s", fakeRange[1], fakeRange[2], fakeRange[3]) // fakeNodePrefix, start, end
	return output, nil
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
	path, plugin := params.TopoConfigPath, params.Plugin

	// set and validate plugin
	switch plugin {
	case "":
		plugin = topology.TopologyTree
	case topology.TopologyTree:
		if _, ok := tree.Vertices[topology.TopologyTree]; !ok {
			return nil, fmt.Errorf("missing tree topology")
		}
	case topology.TopologyBlock:
		if _, ok := tree.Vertices[topology.TopologyTree]; !ok {
			return nil, fmt.Errorf("missing tree topology")
		}
		if _, ok := tree.Vertices[topology.TopologyBlock]; !ok {
			return nil, fmt.Errorf("missing block topology")
		}
	default:
		klog.Infof("Unsupported topology plugin %s. Using %s", plugin, topology.TopologyTree)
		plugin = topology.TopologyTree
		metrics.AddValidationError("unsupported plugin")
	}

	if len(path) != 0 {
		if _, err := buf.WriteString(fmt.Sprintf(TopologyHeader, plugin)); err != nil {
			return nil, err
		}
	}

	if len(tree.Metadata) == 0 {
		tree.Metadata = make(map[string]string)
	}

	tree.Metadata[topology.KeyPlugin] = plugin
	if len(params.BlockSizes) != 0 {
		tree.Metadata[topology.KeyBlockSizes] = params.BlockSizes
	}

	if plugin == topology.TopologyBlock && params.FakeNodesEnabled {
		var fakeData string
		var err error
		if len(params.FakeNodePool) > 0 {
			fakeData = params.FakeNodePool
		} else {
			fakeData, err = GetFakeNodes(ctx)
			if err != nil {
				return nil, err
			}
		}
		tree.Metadata[topology.KeyFakeConfig] = fakeData
	}

	err := translate.Write(buf, tree)
	if err != nil {
		return nil, err
	}

	cfg := buf.Bytes()

	if len(path) == 0 {
		klog.Info("Returning topology config")
		return cfg, nil
	}

	klog.Infof("Writing topology config in %q", path)
	if err = files.Create(path, cfg); err != nil {
		return nil, err
	}
	if params.Reconfigure {
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
