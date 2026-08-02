/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
type physicalNICResolver func(string) (string, error)

const sysClassNetPath = "/sys/class/net"

type ProviderK8S struct {
	client kubernetes.Interface
	params *K8SParams
}

type K8SParams struct {
	NodeSelector   map[string]string `mapstructure:"nodeSelector"`
	Interfaces     []string          `mapstructure:"interfaces"`
	InterfaceRegex string            `mapstructure:"interfaceRegex"`
	RailID         string            `mapstructure:"railID"`

	nodeListOpt *metav1.ListOptions
	selector    neighborSelector
}

// NodeData is the LLDP data collected by the node-data-broker. NICRails is nil
// when rail discovery is not configured and otherwise maps physical NIC device
// names from /sys/class/net/<interface>/device to their derived rail IDs.
type NodeData struct {
	Annotations map[string]string
	NICRails    map[string][]string
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
	selector, err := newNeighborSelector(p.Interfaces, p.InterfaceRegex, p.RailID)
	if err != nil {
		return nil, err
	}
	p.selector = selector
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

func GetNodeAnnotations(ctx context.Context, nodeName string, providerParams map[string]any) (map[string]string, error) {
	data, err := GetNodeData(ctx, nodeName, providerParams)
	if err != nil {
		return nil, err
	}
	return data.Annotations, nil
}

func GetNodeData(ctx context.Context, nodeName string, providerParams map[string]any) (*NodeData, error) {
	params, err := getK8SParameters(providerParams)
	if err != nil {
		return nil, fmt.Errorf("invalid LLDP parameters: %w", err)
	}
	return getNodeData(ctx, nodeName, params.selector, exec.Exec, resolvePhysicalNICName)
}

func getNodeData(ctx context.Context, nodeName string, selector neighborSelector, run localRunner, resolveNIC physicalNICResolver) (*NodeData, error) {
	hostname := strings.TrimSpace(nodeName)
	if hostname == "" {
		return nil, fmt.Errorf("nodeName not provided")
	}
	data := &NodeData{
		Annotations: map[string]string{
			topology.KeyNodeInstance: hostname,
			topology.KeyNodeRegion:   "local",
		},
	}
	if selector.railIDTemplate != "" {
		data.NICRails = make(map[string][]string)
	}
	stdout, err := run(ctx, lldpctlExecutable, []string{"-f", "json"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query the lldpd socket: %w", err)
	}
	neighbors, err := parseNeighbors(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	neighbor, interfaceRails, err := selectNeighborAndInterfaceRails(neighbors, selector)
	if errors.Is(err, errNoLLDPNeighbor) {
		klog.Warningf("node %q: %v", hostname, err)
		data.Annotations[topology.KeyLLDPChassisID] = ""
		return data, nil
	}
	if err != nil {
		return nil, err
	}

	data.Annotations[topology.KeyLLDPChassisID] = neighbor.chassisKey()
	if selector.railIDTemplate != "" {
		data.NICRails, err = mapInterfacesToPhysicalNICs(interfaceRails, resolveNIC)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func resolvePhysicalNICName(interfaceName string) (string, error) {
	return resolvePhysicalNICNameAt(sysClassNetPath, interfaceName)
}

func resolvePhysicalNICNameAt(sysClassNetDir, interfaceName string) (string, error) {
	interfaceName = strings.TrimSpace(interfaceName)
	if interfaceName == "" || filepath.Base(interfaceName) != interfaceName {
		return "", fmt.Errorf("invalid network interface name %q", interfaceName)
	}
	devicePath := filepath.Join(sysClassNetDir, interfaceName, "device")
	deviceTarget, err := os.Readlink(devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to read physical NIC device link for interface %q from %q: %w", interfaceName, devicePath, err)
	}
	physicalNIC := strings.TrimSpace(filepath.Base(deviceTarget))
	if physicalNIC == "" || physicalNIC == "." || physicalNIC == string(filepath.Separator) {
		return "", fmt.Errorf("interface %q has an invalid physical NIC device link %q", interfaceName, deviceTarget)
	}
	return physicalNIC, nil
}

func mapInterfacesToPhysicalNICs(interfaceRails map[string][]string, resolveNIC physicalNICResolver) (map[string][]string, error) {
	if len(interfaceRails) == 0 {
		return map[string][]string{}, nil
	}
	if resolveNIC == nil {
		return nil, fmt.Errorf("physical NIC resolver not configured")
	}

	interfaces := make([]string, 0, len(interfaceRails))
	for interfaceName := range interfaceRails {
		interfaces = append(interfaces, interfaceName)
	}
	sort.Strings(interfaces)

	nicRailSets := make(map[string]map[string]bool)
	for _, interfaceName := range interfaces {
		physicalNIC, err := resolveNIC(interfaceName)
		if err != nil {
			return nil, err
		}
		physicalNIC = strings.TrimSpace(physicalNIC)
		if physicalNIC == "" {
			return nil, fmt.Errorf("interface %q resolved to an empty physical NIC name", interfaceName)
		}
		if nicRailSets[physicalNIC] == nil {
			nicRailSets[physicalNIC] = make(map[string]bool)
		}
		for _, railID := range interfaceRails[interfaceName] {
			nicRailSets[physicalNIC][railID] = true
		}
	}

	nicRails := make(map[string][]string, len(nicRailSets))
	for physicalNIC, railSet := range nicRailSets {
		rails := make([]string, 0, len(railSet))
		for railID := range railSet {
			rails = append(rails, railID)
		}
		sort.Strings(rails)
		nicRails[physicalNIC] = rails
	}
	return nicRails, nil
}
