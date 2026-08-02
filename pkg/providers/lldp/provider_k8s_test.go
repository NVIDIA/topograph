/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lldp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func providersConfig(params map[string]any) providers.Config {
	return providers.Config{Params: params}
}

func TestGetK8SParameters(t *testing.T) {
	params, err := getK8SParameters(map[string]any{
		"interfaces":   []string{"eno1"},
		"nodeSelector": map[string]string{"fabric": "ethernet"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"eno1"}, params.Interfaces)
	require.Equal(t, "fabric=ethernet", params.nodeListOpt.LabelSelector)
}

func TestGetK8SParametersCompilesInterfaceRegexAndRailID(t *testing.T) {
	params, err := getK8SParameters(map[string]any{
		"interfaceRegex": `^eth_r([0-9]+)_p[0-9]+$`,
		"railID":         `rail$1`,
	})
	require.NoError(t, err)
	require.Equal(t, `^eth_r([0-9]+)_p[0-9]+$`, params.selector.interfaceRegexp.String())
	require.Equal(t, `rail$1`, params.selector.railIDTemplate)
}

func TestGetNodeAnnotations(t *testing.T) {
	runner := func(_ context.Context, executable string, args []string, _ map[string]string) (*bytes.Buffer, error) {
		require.Equal(t, "lldpctl", executable)
		require.Equal(t, []string{"-f", "json"}, args)
		return bytes.NewBufferString(singleNeighborJSON), nil
	}

	selector := requireNeighborSelector(t, []string{"eno1"}, "", "")
	data, err := getNodeData(context.Background(), "node-1", selector, runner, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		topology.KeyNodeInstance:  "node-1",
		topology.KeyNodeRegion:    "local",
		topology.KeyLLDPChassisID: "mac:00:11:22:33:44:55",
	}, data.Annotations)
	require.Nil(t, data.NICRails)
}

func TestGetNodeDataDerivesNICRails(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(multipleNeighborsJSON), nil
	}
	selector := requireNeighborSelector(t, nil, `^eno([12])$`, `rail$1`)
	resolveNIC := func(interfaceName string) (string, error) {
		require.Contains(t, []string{"eno1", "eno2"}, interfaceName)
		return "0000:01:00.0", nil
	}

	data, err := getNodeData(context.Background(), "node-1", selector, runner, resolveNIC)
	require.NoError(t, err)
	require.Equal(t, "mac:00:11:22:33:44:55", data.Annotations[topology.KeyLLDPChassisID])
	require.Equal(t, map[string][]string{
		"0000:01:00.0": {"rail1", "rail2"},
	}, data.NICRails)
}

func TestGetNodeAnnotationsClearsMissingNeighbor(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(`{"lldp":{}}`), nil
	}

	data, err := getNodeData(context.Background(), "node-1", neighborSelector{}, runner, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		topology.KeyNodeInstance:  "node-1",
		topology.KeyNodeRegion:    "local",
		topology.KeyLLDPChassisID: "",
	}, data.Annotations)
	require.Nil(t, data.NICRails)
}

func TestGetNodeDataClearsMissingNICRails(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(`{"lldp":{}}`), nil
	}
	selector := requireNeighborSelector(t, nil, `^eno([0-9]+)$`, `rail$1`)

	data, err := getNodeData(context.Background(), "node-1", selector, runner, nil)
	require.NoError(t, err)
	require.NotNil(t, data.NICRails)
	require.Empty(t, data.NICRails)
}

func TestGetNodeAnnotationsRejectsAmbiguity(t *testing.T) {
	runner := func(_ context.Context, _ string, _ []string, _ map[string]string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(multipleNeighborsJSON), nil
	}

	data, err := getNodeData(context.Background(), "node-1", neighborSelector{}, runner, nil)
	require.ErrorContains(t, err, "multiple LLDP switches found")
	require.Nil(t, data)
}

func TestResolvePhysicalNICNameAt(t *testing.T) {
	sysClassNetDir := t.TempDir()
	interfaceDir := filepath.Join(sysClassNetDir, "eth_r0_p0")
	require.NoError(t, os.MkdirAll(interfaceDir, 0o755))
	require.NoError(t, os.Symlink("../../../devices/pci0000:00/0000:3b:00.0", filepath.Join(interfaceDir, "device")))

	physicalNIC, err := resolvePhysicalNICNameAt(sysClassNetDir, "eth_r0_p0")
	require.NoError(t, err)
	require.Equal(t, "0000:3b:00.0", physicalNIC)

	_, err = resolvePhysicalNICNameAt(sysClassNetDir, "missing")
	require.ErrorContains(t, err, `failed to read physical NIC device link for interface "missing"`)

	_, err = resolvePhysicalNICNameAt(sysClassNetDir, "../eth0")
	require.EqualError(t, err, `invalid network interface name "../eth0"`)
}

func TestMapInterfacesToPhysicalNICs(t *testing.T) {
	deviceByInterface := map[string]string{
		"eth_r0_p0": "0000:3b:00.0",
		"eth_r1_p0": "0000:3b:00.0",
		"eth_r2_p0": "0000:af:00.0",
	}
	resolveNIC := func(interfaceName string) (string, error) {
		device, ok := deviceByInterface[interfaceName]
		if !ok {
			return "", fmt.Errorf("unknown interface %q", interfaceName)
		}
		return device, nil
	}

	nicRails, err := mapInterfacesToPhysicalNICs(map[string][]string{
		"eth_r0_p0": {"rail0"},
		"eth_r1_p0": {"rail1", "rail0"},
		"eth_r2_p0": {"rail2"},
	}, resolveNIC)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"0000:3b:00.0": {"rail0", "rail1"},
		"0000:af:00.0": {"rail2"},
	}, nicRails)
}

func TestProviderK8SGenerateTopologyConfig(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "k8s-node-1",
		Labels: map[string]string{"fabric": "ethernet"},
		Annotations: map[string]string{
			topology.KeyNodeInstance:  "instance-1",
			topology.KeyNodeRegion:    "local",
			topology.KeyLLDPChassisID: "mac:00:11:22:33:44:55",
		},
	}}
	provider := &ProviderK8S{
		client: fake.NewSimpleClientset(node),
		params: &K8SParams{nodeListOpt: &metav1.ListOptions{LabelSelector: "fabric=ethernet"}},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"instance-1": "k8s-node-1"}}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, "lldp-001122334455")
	require.Contains(t, graph.Tiers.Vertices["lldp-001122334455"].Vertices, "instance-1")
}

func TestProviderK8SGeneratesNoTopologyWithoutChassisAnnotation(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "k8s-node-1",
		Annotations: map[string]string{
			topology.KeyNodeInstance: "instance-1",
			topology.KeyNodeRegion:   "local",
		},
	}}
	provider := &ProviderK8S{
		client: fake.NewSimpleClientset(node),
		params: &K8SParams{},
	}
	cis := []topology.ComputeInstances{{Region: "local", Instances: map[string]string{"instance-1": "k8s-node-1"}}}

	graph, httpErr := provider.GenerateTopologyConfig(context.Background(), nil, cis)
	require.Nil(t, httpErr)
	require.Contains(t, graph.Tiers.Vertices, topology.NoTopology)
	require.Contains(t, graph.Tiers.Vertices[topology.NoTopology].Vertices, "instance-1")
}
