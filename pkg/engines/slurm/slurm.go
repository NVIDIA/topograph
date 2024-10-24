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
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/translate"
	"github.com/NVIDIA/topograph/pkg/utils"
)

const TopologyHeader = `
###############################################################
# Slurm's network topology configuration file for use with the
# %s plugin
###############################################################
`

type SlurmEngine struct{}

func GetNodeList(ctx context.Context) ([]string, error) {
	stdout, err := utils.Exec(ctx, "scontrol", []string{"show", "nodes", "-o"}, nil)
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

func (eng *SlurmEngine) GenerateOutput(ctx context.Context, tree *common.Vertex, params map[string]string) ([]byte, error) {
	return GenerateOutput(ctx, tree, params)
}

func GenerateOutput(ctx context.Context, tree *common.Vertex, params map[string]string) ([]byte, error) {
	buf := &bytes.Buffer{}
	path := params[common.KeyTopoConfigPath]

	if len(path) != 0 {
		var plugin string
		if len(tree.Metadata) != 0 {
			plugin = tree.Metadata[common.KeyPlugin]
		}
		if len(plugin) == 0 {
			plugin = common.ValTopologyTree
		}
		if _, err := buf.WriteString(fmt.Sprintf(TopologyHeader, plugin)); err != nil {
			return nil, err
		}
	}

	err := translate.ToSLURM(buf, tree)
	if err != nil {
		return nil, err
	}

	cfg := buf.Bytes()

	if len(path) == 0 {
		return cfg, nil
	}

	klog.Infof("Writing topology config in %q", path)
	if err = utils.CreateFile(path, cfg); err != nil {
		return nil, err
	}
	if _, ok := params[common.KeySkipReload]; ok {
		klog.Infof("Skip SLURM reconfiguration")
	} else {
		if err = reconfigure(ctx); err != nil {
			return nil, err
		}
	}
	return []byte("OK\n"), nil
}

func reconfigure(ctx context.Context) error {
	stdout, err := utils.Exec(ctx, "scontrol", []string{"reconfigure"}, nil)
	if err != nil {
		return err
	}
	klog.V(4).Infof("stdout: %s", stdout.String())
	return nil
}
