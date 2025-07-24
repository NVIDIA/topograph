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
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/cluset"
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

type BaseParams struct {
	Plugin           string               `mapstructure:"plugin"`
	BlockSizes       string               `mapstructure:"block_sizes"`
	FakeNodesEnabled bool                 `mapstructure:"fakeNodesEnabled"`
	FakeNodePool     string               `mapstructure:"fake_node_pool"`
	Topologies       map[string]*Topology `mapstructure:"topologies,omitempty"`
}

type Topology struct {
	Plugin     string   `mapstructure:"plugin"`
	BlockSizes string   `mapstructure:"block_sizes"`
	Nodes      []string `mapstructure:"nodes"`
	Default    bool     `mapstructure:"cluster_default"`
}

type Params struct {
	BaseParams     `mapstructure:",squash"`
	TopoConfigPath string `mapstructure:"topology_config_path"`
	Reconfigure    bool   `mapstructure:"reconfigure"`
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
	args := []string{"show", "partition", "fake"}
	stdout, err := exec.Exec(ctx, "scontrol", args, nil)
	if err != nil {
		return "", err
	}
	out := stdout.String()
	klog.V(4).Infof("stdout: %s", out)

	return parseFakeNodes(out)
}

func parseFakeNodes(data string) (string, error) {
	prefix := "Nodes="
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); strings.HasPrefix(line, prefix) {
			return line[len(prefix):], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan fake nodes partition: %v", err)
	}

	return "", fmt.Errorf("fake partition has no nodes")
}

func GetTopologyNodes(ctx context.Context, topo string) ([]string, error) {
	args := []string{"show", "topology", topo}
	stdout, err := exec.Exec(ctx, "scontrol", args, nil)
	if err != nil {
		return nil, err
	}
	out := stdout.String()
	klog.V(4).Infof("stdout: %s", out)

	return parseTopologyNodes(out)
}

func parseTopologyNodes(data string) ([]string, error) {
	linePrefix := "BlockName="
	pairPrefix := "Nodes="
	nodes := []string{}
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); strings.HasPrefix(line, linePrefix) {
			pairs := strings.Split(line, " ")
			for _, pair := range pairs {
				if str := strings.TrimSpace(pair); strings.HasPrefix(str, pairPrefix) {
					nodes = append(nodes, str[6:])
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan fake nodes partition: %v", err)
	}
	return cluset.Compact(cluset.Expand(nodes)), nil
}

func (eng *SlurmEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	return GenerateOutput(ctx, tree, params)
}

func GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	p, err := getParams(params)
	if err != nil {
		return nil, err
	}

	return GenerateOutputParams(ctx, tree, p)
}

func GenerateOutputParams(ctx context.Context, root *topology.Vertex, params *Params) ([]byte, error) {
	// apply legacy default plugin value
	if len(params.Plugin) == 0 && len(params.Topologies) == 0 {
		params.Plugin = topology.TopologyTree
	}

	cfg, err := GetTranslateConfig(ctx, &params.BaseParams)
	if err != nil {
		return nil, err
	}

	nt, err := translate.NewNetworkTopology(root, cfg)
	if err != nil {
		return nil, err
	}

	path := params.TopoConfigPath
	buf := &bytes.Buffer{}
	if len(path) != 0 {
		if _, err := fmt.Fprintf(buf, TopologyHeader, params.Plugin); err != nil {
			return nil, err
		}
	}

	err = nt.Generate(buf)
	if err != nil {
		return nil, err
	}

	data := buf.Bytes()

	if len(path) == 0 {
		klog.Info("Returning topology config")
		return data, nil
	}

	klog.Infof("Writing topology config in %q", path)
	if err = files.Create(path, data); err != nil {
		return nil, err
	}
	if params.Reconfigure {
		if err = reconfigure(ctx); err != nil {
			return nil, err
		}
	}

	return []byte("OK\n"), nil
}

func GetTranslateConfig(ctx context.Context, params *BaseParams) (*translate.Config, error) {
	cfg := &translate.Config{
		Plugin:     params.Plugin,
		BlockSizes: getBlockSizes(params.BlockSizes),
	}

	// set fake nodes
	if params.Plugin == topology.TopologyBlock && params.FakeNodesEnabled {
		var fakeNodes string
		var err error
		if len(params.FakeNodePool) > 0 {
			fakeNodes = params.FakeNodePool
		} else {
			fakeNodes, err = GetFakeNodes(ctx)
			if err != nil {
				return nil, err
			}
		}
		cfg.FakeNodePool = fakeNodes
	}

	// set per-partition topologies
	if len(params.Topologies) != 0 {
		cfg.Topologies = make(map[string]*translate.TopologySpec)
		for topo, sect := range params.Topologies {
			spec := &translate.TopologySpec{
				Plugin:         sect.Plugin,
				BlockSizes:     getBlockSizes(sect.BlockSizes),
				ClusterDefault: sect.Default,
			}

			if len(sect.Nodes) != 0 {
				spec.Nodes = sect.Nodes
			} else if nodes, err := GetTopologyNodes(ctx, topo); err == nil {
				spec.Nodes = nodes
			} else {
				return nil, err
			}
			cfg.Topologies[topo] = spec
		}
	}

	return cfg, nil
}

func getParams(params map[string]any) (*Params, error) {
	var p Params
	err := config.Decode(params, &p)
	return &p, err
}

func getBlockSizes(str string) []int {
	if len(str) == 0 {
		return nil
	}
	parts := strings.Split(str, ",")
	blockSizes := make([]int, 0, len(parts))
	for _, part := range parts {
		sz, err := strconv.Atoi(part)
		if err != nil {
			metrics.AddValidationError("BlockSize parsing error")
			klog.Warningf("Failed to parse blockSize %v: %v. Ignoring admin blockSizes.", part, err)
			return nil
		}
		blockSizes = append(blockSizes, sz)
	}
	return blockSizes
}

func reconfigure(ctx context.Context) error {
	stdout, err := exec.Exec(ctx, "scontrol", []string{"reconfigure"}, nil)
	if err != nil {
		return err
	}

	klog.V(4).Infof("stdout: %s", stdout.String())

	return nil
}
