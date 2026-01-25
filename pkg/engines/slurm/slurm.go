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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/files"
	"github.com/NVIDIA/topograph/internal/httperr"
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
	DynamicNodes     []string             `mapstructure:"dynamicNodes"`
	MinBlocks        int                  `mapstructure:"minBlocks"`
	Topologies       map[string]*Topology `mapstructure:"topologies,omitempty"`
}

type Topology struct {
	Partition    string   `mapstructure:"partition"`
	Plugin       string   `mapstructure:"plugin"`
	BlockSizes   []int    `mapstructure:"blockSizes"`
	DynamicNodes []string `mapstructure:"dynamicNodes"`
	MinBlocks    int      `mapstructure:"minBlocks"`
	Nodes        []string `mapstructure:"nodes"`
	Default      bool     `mapstructure:"clusterDefault"`
}

type Params struct {
	BaseParams     `mapstructure:",squash"`
	TopoConfigPath string `mapstructure:"topologyConfigPath"`
	Reconfigure    bool   `mapstructure:"reconfigure"`
}

type TopologyNodeFinder struct {
	GetPartitionNodes func(context.Context, string, []any) (string, error)
	Params            []any
}

type instanceMapper interface {
	// Instances2NodeMap receives a list of SLURM node names and returns a map of
	// the service provider assigned compute instance IDs to the node names
	Instances2NodeMap(context.Context, []string) (map[string]string, error)
	// GetInstancesRegions receives a list of SLURM node names and returns a map
	// of node names to their deployed regions
	GetInstancesRegions(context.Context, []string) (map[string]string, error)
}

var partitionNodesRe *regexp.Regexp

func init() {
	partitionNodesRe = regexp.MustCompile(`\sNodes=([^\s]+)`)
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(_ context.Context, _ engines.Config) (engines.Engine, *httperr.Error) {
	return &SlurmEngine{}, nil
}

func (eng *SlurmEngine) GetComputeInstances(ctx context.Context, environment engines.Environment) ([]topology.ComputeInstances, *httperr.Error) {
	instanceMapper, ok := environment.(instanceMapper)
	if !ok {
		return nil, httperr.NewError(http.StatusBadRequest, "environment must implement instanceMapper")
	}

	nodes, err := GetNodeList(ctx)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	i2n, err := instanceMapper.Instances2NodeMap(ctx, nodes)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}
	klog.V(4).Infof("Detected instance map: %v", i2n)

	nodeRegions, err := instanceMapper.GetInstancesRegions(ctx, nodes)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	return aggregateComputeInstances(i2n, nodeRegions), nil
}

func aggregateComputeInstances(i2n, nodeRegions map[string]string) []topology.ComputeInstances {
	// regions maps region name to the corresponding index in "cis"
	regions := make(map[string]int)
	cis := []topology.ComputeInstances{}

	for instance, node := range i2n {
		region, ok := nodeRegions[node]
		if !ok {
			klog.Warningf("Failed to find region for node %s", node)
			continue
		}
		indx, ok := regions[region]
		if !ok {
			indx = len(regions)
			regions[region] = indx
			cis = append(cis, topology.ComputeInstances{
				Region:    region,
				Instances: map[string]string{instance: node},
			})
		} else {
			cis[indx].Instances[instance] = node
		}
	}
	klog.V(4).Infof("Detected regions: %v", regions)

	return cis
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

func getPartitionNodes(ctx context.Context, partition string, _ []any) (string, error) {
	args := []string{"show", "partition", partition}
	stdout, err := exec.Exec(ctx, "scontrol", args, nil)
	if err != nil {
		return "", err
	}
	out := stdout.String()
	return out, nil
}

func GetPartitionNodes(ctx context.Context, partition string, f *TopologyNodeFinder) ([]string, error) {
	if len(partition) == 0 {
		return nil, fmt.Errorf("missing partition name")
	}
	out, err := f.GetPartitionNodes(ctx, partition, f.Params)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("GetPartitionNodes: %s", out)
	return parsePartitionNodes(partition, out)
}

func parsePartitionNodes(partition string, data string) ([]string, error) {
	match := partitionNodesRe.FindStringSubmatch(data)
	if len(match) > 1 {
		return cluset.Compact(cluset.ExpandList(match[1])), nil
	}

	return nil, fmt.Errorf("partition %q has no nodes", partition)
}

func (eng *SlurmEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, *httperr.Error) {
	return GenerateOutput(ctx, tree, params)
}

func GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, *httperr.Error) {
	p, err := getParams(params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	return GenerateOutputParams(ctx, tree, p)
}

func GenerateOutputParams(ctx context.Context, root *topology.Vertex, params *Params) ([]byte, *httperr.Error) {
	// apply legacy default plugin value
	if len(params.Plugin) == 0 && len(params.Topologies) == 0 {
		params.Plugin = topology.TopologyTree
	}

	cfg, err := GetTranslateConfig(ctx, &params.BaseParams, &TopologyNodeFinder{GetPartitionNodes: getPartitionNodes})
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	nt, err := translate.NewNetworkTopology(root, cfg)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	path := params.TopoConfigPath
	buf := &bytes.Buffer{}
	if len(path) != 0 {
		if _, err := fmt.Fprintf(buf, TopologyHeader, params.Plugin); err != nil {
			return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	if httpErr := nt.Generate(buf); httpErr != nil {
		return nil, httpErr
	}

	data := buf.Bytes()

	if len(path) == 0 {
		klog.Info("Returning topology config")
		return data, nil
	}

	klog.Infof("Writing topology config in %q", path)
	if err = files.Create(path, data); err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}
	if params.Reconfigure {
		if err = reconfigure(ctx); err != nil {
			return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	return []byte("OK\n"), nil
}

func GetTranslateConfig(ctx context.Context, params *BaseParams, f *TopologyNodeFinder) (*translate.Config, error) {
	cfg := &translate.Config{
		Plugin:       params.Plugin,
		BlockSizes:   getBlockSizes(params.BlockSizes),
		DynamicNodes: params.DynamicNodes,
		MinBlocks:    params.MinBlocks,
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
				BlockSizes:     sect.BlockSizes,
				DynamicNodes:   sect.DynamicNodes,
				MinBlocks:      sect.MinBlocks,
				ClusterDefault: sect.Default,
			}
			klog.InfoS("Adding partition topology", "name", topo, "plugin", sect.Plugin, "default", sect.Default, "partition", sect.Partition)
			if len(sect.Nodes) != 0 {
				klog.V(4).Infof("%s %q provides nodes %v", sect.Plugin, topo, sect.Nodes)
				spec.Nodes = sect.Nodes
			} else {
				if sect.Default && sect.Plugin == topology.TopologyFlat && len(sect.Partition) == 0 {
					klog.V(4).Infof("skip node discovery for default %s %q", sect.Plugin, topo)
				} else if nodes, err := GetPartitionNodes(ctx, sect.Partition, f); err == nil {
					klog.V(4).Infof("%s %q discovered nodes %v", sect.Plugin, topo, nodes)
					spec.Nodes = nodes
				} else {
					return nil, err
				}
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
