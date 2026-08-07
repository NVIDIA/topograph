/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package lldp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/exec"
	"github.com/NVIDIA/topograph/internal/httperr"
	internalk8s "github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type localRunner func(context.Context, string, []string, map[string]string) (*bytes.Buffer, error)

type ProviderK8S struct {
	client kubernetes.Interface
	params *K8SParams
}

type K8SParams struct {
	NodeSelector map[string]string `mapstructure:"nodeSelector"`
	Interfaces   []string          `mapstructure:"interfaces"`

	nodeListOpt *metav1.ListOptions
}

func NamedLoaderK8S() (string, providers.Loader) {
	return NAME_K8S, LoaderK8S
}

func LoaderK8S(_ context.Context, providerConfig providers.Config) (providers.Provider, *httperr.Error) {
	params, err := getK8SParameters(providerConfig.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}
	if err := internalk8s.ConfigureClientRateLimits(cfg); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	return &ProviderK8S{client: client, params: params}, nil
}

func getK8SParameters(params map[string]any) (*K8SParams, error) {
	p := &K8SParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, err
	}
	if err := validateInterfaces(p.Interfaces); err != nil {
		return nil, err
	}
	if len(p.NodeSelector) != 0 {
		p.nodeListOpt = &metav1.ListOptions{LabelSelector: labels.Set(p.NodeSelector).String()}
	}
	return p, nil
}

func (p *ProviderK8S) GenerateTopologyConfig(ctx context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	if len(cis) > 1 {
		return nil, httperr.NewError(http.StatusBadRequest, "on-prem does not support multi-region topology requests")
	}
	nodes, err := internalk8s.GetNodes(ctx, p.client, p.params.nodeListOpt)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	topo := topology.NewClusterTopology()
	for _, node := range nodes.Items {
		instanceID := strings.TrimSpace(node.Annotations[topology.KeyNodeInstance])
		chassisKey := strings.TrimSpace(node.Annotations[topology.KeyLLDPChassisID])
		if instanceID == "" || chassisKey == "" {
			klog.Warningf("Skipping node %q: missing %q or %q annotation", node.Name, topology.KeyNodeInstance, topology.KeyLLDPChassisID)
			continue
		}
		switchID, err := switchID(chassisKey)
		if err != nil {
			return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("node %q: %v", node.Name, err))
		}
		topo.Append(&topology.InstanceTopology{
			InstanceID:  instanceID,
			FabricTiers: topology.ClosestFirstFabricTiers(switchID),
		})
	}
	return topo.ToGraph(NAME_K8S, cis, 0, false), nil
}

func GetNodeAnnotations(ctx context.Context, nodeName string, extras map[string]string) (map[string]string, error) {
	return getNodeAnnotations(ctx, nodeName, extras, exec.Exec)
}

func getNodeAnnotations(ctx context.Context, nodeName string, extras map[string]string, run localRunner) (map[string]string, error) {
	hostname := strings.TrimSpace(nodeName)
	if hostname == "" {
		return nil, fmt.Errorf("nodeName not provided")
	}
	annotations := map[string]string{
		topology.KeyNodeInstance: hostname,
		topology.KeyNodeRegion:   "local",
	}
	interfaces, err := parseInterfaceList(extras["interfaces"])
	if err != nil {
		return nil, fmt.Errorf("invalid LLDP interfaces: %w", err)
	}
	stdout, err := run(ctx, lldpctlExecutable, []string{"-f", "json"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query the lldpd socket: %w", err)
	}
	neighbors, err := parseNeighbors(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	neighbor, err := selectNeighbor(neighbors, interfaces)
	if errors.Is(err, errNoLLDPNeighbor) {
		klog.Warningf("node %q: %v", hostname, err)
		annotations[topology.KeyLLDPChassisID] = ""
		return annotations, nil
	}
	if err != nil {
		return nil, err
	}

	annotations[topology.KeyLLDPChassisID] = neighbor.chassisKey()
	return annotations, nil
}
