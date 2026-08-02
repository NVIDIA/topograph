/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/NVIDIA/topograph/pkg/providers/infiniband"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestGetAnnotations(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		name     string
		provider string
		err      string
	}{
		{
			name: "Case 1: empty provider",
			err:  "must set provider",
		},
		{
			name:     "Case 2: invalid provider",
			provider: "invalid",
			err:      `unsupported provider "invalid"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := &nodeBroker{
				config: nodeDataBrokerConfig{Provider: topology.Provider{Name: tt.provider}},
			}
			_, err := broker.getAnnotations(ctx)
			require.EqualError(t, err, tt.err)
		})
	}

	t.Run("invalid accelerator section", func(t *testing.T) {
		broker := &nodeBroker{
			config: nodeDataBrokerConfig{
				Provider: topology.Provider{
					Name: infiniband.NAME_K8S,
					Params: map[string]any{
						"accelerator": "invalid",
					},
				},
			},
		}
		_, err := broker.getAnnotations(ctx)
		require.EqualError(t, err, "accelerator section must be an object with a source")
	})

	t.Run("null accelerator section", func(t *testing.T) {
		broker := &nodeBroker{
			config: nodeDataBrokerConfig{
				Provider: topology.Provider{
					Name: infiniband.NAME_K8S,
					Params: map[string]any{
						"accelerator": nil,
					},
				},
			},
		}
		_, err := broker.getAnnotations(ctx)
		require.EqualError(t, err, "accelerator section must be an object with a source")
	})

	t.Run("empty accelerator section disables discovery", func(t *testing.T) {
		broker := &nodeBroker{
			nodeName: "node-1",
			config: nodeDataBrokerConfig{
				Provider: topology.Provider{
					Name: infiniband.NAME_K8S,
					Params: map[string]any{
						"accelerator": map[string]any{},
					},
				},
			},
		}
		annotations, err := broker.getAnnotations(ctx)
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			topology.KeyNodeInstance: "node-1",
			topology.KeyNodeRegion:   "local",
		}, annotations)
	})
}

func TestNewNodeDataBrokerConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "node-data-broker-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
provider:
  name: infiniband-k8s
  params:
    accelerator:
      source: none
healthzPort: 18080
nicRailsConfigMap:
  name: topograph-nic-rails
  namespace: topograph
`), 0o600))

	config, err := newNodeDataBrokerConfig(configPath)
	require.NoError(t, err)
	require.Equal(t, infiniband.NAME_K8S, config.Provider.Name)
	require.Equal(t, map[string]any{
		"accelerator": map[string]any{"source": "none"},
	}, config.Provider.Params)
	require.Equal(t, 18080, config.HealthzPort)
	require.Equal(t, &configMapReference{
		Name:      "topograph-nic-rails",
		Namespace: "topograph",
	}, config.NICRailsConfigMap)

	_, err = newNodeDataBrokerConfig(filepath.Join(dir, "missing.yaml"))
	require.ErrorContains(t, err, "failed to read node-data-broker config")

	invalidPath := filepath.Join(dir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidPath, []byte("provider: ["), 0o600))
	_, err = newNodeDataBrokerConfig(invalidPath)
	require.ErrorContains(t, err, "failed to decode node-data-broker config")

	missingProviderPath := filepath.Join(dir, "missing-provider.yaml")
	require.NoError(t, os.WriteFile(missingProviderPath, []byte("healthzPort: 8080"), 0o600))
	_, err = newNodeDataBrokerConfig(missingProviderPath)
	require.EqualError(t, err, "must specify provider.name")

	missingPortPath := filepath.Join(dir, "missing-port.yaml")
	require.NoError(t, os.WriteFile(missingPortPath, []byte("provider:\n  name: test"), 0o600))
	_, err = newNodeDataBrokerConfig(missingPortPath)
	require.EqualError(t, err, "must specify a positive healthzPort")

	negativePortPath := filepath.Join(dir, "negative-port.yaml")
	require.NoError(t, os.WriteFile(negativePortPath, []byte("provider:\n  name: test\nhealthzPort: -1"), 0o600))
	_, err = newNodeDataBrokerConfig(negativePortPath)
	require.EqualError(t, err, "must specify a positive healthzPort")
}

func TestPatchNICRailsConfigMap(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "topograph-nic-rails",
			Namespace: "topograph",
		},
		Data: map[string]string{
			"node-2": `{"0000:af:00.0":["rail2"]}`,
		},
	})
	broker := &nodeBroker{
		clientset: clientset,
		nodeName:  "node-1",
		config: nodeDataBrokerConfig{NICRailsConfigMap: &configMapReference{
			Name:      "topograph-nic-rails",
			Namespace: "topograph",
		}},
	}

	err := broker.patchNICRailsConfigMap(ctx, map[string][]string{
		"0000:af:00.0": {"rail1"},
		"0000:3b:00.0": {"rail0"},
	})
	require.NoError(t, err)

	configMap, err := clientset.CoreV1().ConfigMaps("topograph").Get(ctx, "topograph-nic-rails", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, `{"0000:3b:00.0":["rail0"],"0000:af:00.0":["rail1"]}`, configMap.Data["node-1"])
	require.Equal(t, `{"0000:af:00.0":["rail2"]}`, configMap.Data["node-2"])

	require.NoError(t, broker.patchNICRailsConfigMap(ctx, map[string][]string{}))
	configMap, err = clientset.CoreV1().ConfigMaps("topograph").Get(ctx, "topograph-nic-rails", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, configMap.Data, "node-1")
	require.Equal(t, `{"0000:af:00.0":["rail2"]}`, configMap.Data["node-2"])
}

func TestPatchNICRailsConfigMapRequiresReference(t *testing.T) {
	broker := &nodeBroker{nodeName: "node-1"}
	err := broker.patchNICRailsConfigMap(context.Background(), map[string][]string{"0000:3b:00.0": {"rail0"}})
	require.EqualError(t, err, "nicRailsConfigMap name and namespace are required when LLDP rail discovery is configured")
}

func TestMergeNodeAnnotations(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
		in   map[string]string
		out  map[string]string
	}{
		{
			name: "Case 1: no annotations",
			node: &corev1.Node{},
			out:  map[string]string{},
		},
		{
			name: "Case 2: copy",
			node: &corev1.Node{},
			in:   map[string]string{"a": "1", "b": "2"},
			out:  map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "Case 3: merge",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"a": "1", "b": "2", "c": "x"},
					Annotations: map[string]string{"a": "1", "b": "2", "c": "x"},
				},
			},
			in:  map[string]string{"c": "3", "d": "4"},
			out: map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"},
		},
		{
			name: "Case 4: empty value deletes annotation",
			node: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				"keep":  "value",
				"stale": "value",
			}}},
			in:  map[string]string{"new": "value", "stale": ""},
			out: map[string]string{"keep": "value", "new": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeNodeAnnotations(tt.node, tt.in)
			require.Equal(t, tt.out, tt.node.Annotations)
		})
	}
}

func TestHealthHandler(t *testing.T) {
	srv := httptest.NewServer(healthHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

func TestServeHealthShutsDownOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		// port 0 lets the OS pick a free ephemeral port.
		done <- serveHealth(ctx, 0)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serveHealth did not return after context cancellation")
	}
}
