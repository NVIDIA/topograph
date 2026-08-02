/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/topograph/internal/version"
	"github.com/NVIDIA/topograph/pkg/accelerator"
	"github.com/NVIDIA/topograph/pkg/providers/aws"
	"github.com/NVIDIA/topograph/pkg/providers/dra"
	"github.com/NVIDIA/topograph/pkg/providers/gcp"
	"github.com/NVIDIA/topograph/pkg/providers/infiniband"
	"github.com/NVIDIA/topograph/pkg/providers/lambdai"
	"github.com/NVIDIA/topograph/pkg/providers/lldp"
	"github.com/NVIDIA/topograph/pkg/providers/nebius"
	"github.com/NVIDIA/topograph/pkg/providers/oci"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	defaultConfigPath = "/etc/topograph/node-data-broker-config.yaml"
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

type nodeBroker struct {
	clientset  kubernetes.Interface
	restConfig *rest.Config
	config     nodeDataBrokerConfig
	nodeName   string
}

type nodeDataBrokerConfig struct {
	Provider          topology.Provider   `yaml:"provider"`
	HealthzPort       int                 `yaml:"healthzPort"`
	NICRailsConfigMap *configMapReference `yaml:"nicRailsConfigMap,omitempty"`
}

type configMapReference struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type collectedNodeData struct {
	annotations map[string]string
	nicRails    map[string][]string
}

func main() {
	var ver bool
	var configPath string
	pflag.BoolVar(&ver, "version", false, "show the version")
	pflag.StringVarP(&configPath, "config", "c", defaultConfigPath, "config file")

	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	defer klog.Flush()

	if ver {
		fmt.Println("Version:", version.Version)
		os.Exit(0)
	}

	config, err := newNodeDataBrokerConfig(configPath)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	if err := mainInternal(config); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func newNodeDataBrokerConfig(path string) (nodeDataBrokerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nodeDataBrokerConfig{}, fmt.Errorf("failed to read node-data-broker config %q: %w", path, err)
	}
	var config nodeDataBrokerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nodeDataBrokerConfig{}, fmt.Errorf("failed to decode node-data-broker config %q: %w", path, err)
	}
	if config.Provider.Name == "" {
		return nodeDataBrokerConfig{}, fmt.Errorf("must specify provider.name")
	}
	if config.HealthzPort <= 0 {
		return nodeDataBrokerConfig{}, fmt.Errorf("must specify a positive healthzPort")
	}
	return config, nil
}

func mainInternal(brokerConfig nodeDataBrokerConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clientset, config, err := newInClusterClientset()
	if err != nil {
		return err
	}

	broker := &nodeBroker{
		clientset:  clientset,
		restConfig: config,
		config:     brokerConfig,
		nodeName:   os.Getenv("NODE_NAME"),
	}

	if err := broker.apply(ctx); err != nil {
		return err
	}

	// Keep the DaemonSet pod Running by serving a health endpoint until the pod
	// is terminated.
	return serveHealth(ctx, brokerConfig.HealthzPort)
}

func newInClusterClientset() (kubernetes.Interface, *rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return clientset, config, nil
}

func (b *nodeBroker) apply(ctx context.Context) error {
	klog.InfoS("Applying node topology data", "provider", b.config.Provider.Name)

	data, err := b.getNodeData(ctx)
	if err != nil {
		return err
	}
	klog.Infof("adding annotations %v in node %s for provider %s", data.annotations, b.nodeName, b.config.Provider.Name)

	node, err := b.clientset.CoreV1().Nodes().Get(ctx, b.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", b.nodeName, err)
	}

	mergeNodeAnnotations(node, data.annotations)

	_, err = b.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node: %v", err)
	}
	if data.nicRails != nil {
		if err := b.patchNICRailsConfigMap(ctx, data.nicRails); err != nil {
			return err
		}
	}

	return nil
}

func (b *nodeBroker) patchNICRailsConfigMap(ctx context.Context, nicRails map[string][]string) error {
	ref := b.config.NICRailsConfigMap
	if ref == nil || ref.Name == "" || ref.Namespace == "" {
		return fmt.Errorf("nicRailsConfigMap name and namespace are required when LLDP rail discovery is configured")
	}

	var value *string
	if len(nicRails) != 0 {
		encoded, err := json.Marshal(nicRails)
		if err != nil {
			return fmt.Errorf("failed to encode NIC rail data for node %q: %w", b.nodeName, err)
		}
		text := string(encoded)
		value = &text
	}
	patch, err := json.Marshal(struct {
		Data map[string]*string `json:"data"`
	}{Data: map[string]*string{b.nodeName: value}})
	if err != nil {
		return fmt.Errorf("failed to encode NIC rail ConfigMap patch for node %q: %w", b.nodeName, err)
	}

	_, err = b.clientset.CoreV1().ConfigMaps(ref.Namespace).Patch(
		ctx,
		ref.Name,
		types.MergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch NIC rail ConfigMap %s/%s for node %q: %w", ref.Namespace, ref.Name, b.nodeName, err)
	}
	klog.InfoS("Updated NIC rail ConfigMap", "namespace", ref.Namespace, "name", ref.Name, "node", b.nodeName, "nicRails", nicRails)
	return nil
}

// serveHealth runs a minimal HTTP server exposing /healthz so the DaemonSet
// pod stays Running after node annotations have been applied. It blocks until
// the context is cancelled (SIGTERM/SIGINT), then shuts down gracefully.
func serveHealth(ctx context.Context, port int) error {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           healthHandler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		klog.Infof("Serving health endpoint on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		klog.Info("Shutting down health endpoint")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func healthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (b *nodeBroker) getAnnotations(ctx context.Context) (map[string]string, error) {
	data, err := b.getNodeData(ctx)
	if err != nil {
		return nil, err
	}
	return data.annotations, nil
}

func (b *nodeBroker) getNodeData(ctx context.Context) (*collectedNodeData, error) {
	var annotations map[string]string
	var err error
	switch b.config.Provider.Name {
	case aws.NAME:
		annotations, err = aws.GetNodeAnnotations(ctx)
	case gcp.NAME:
		annotations, err = gcp.GetNodeAnnotations(ctx)
	case oci.NAME:
		annotations, err = oci.GetNodeAnnotations(ctx)
	case nebius.NAME:
		annotations, err = nebius.GetNodeAnnotations(ctx)
	case dra.NAME:
		annotations, err = dra.GetNodeAnnotations(ctx, b.nodeName)
	case infiniband.NAME_K8S:
		section := accelerator.SectionFromProviderParams(b.config.Provider.Params)
		annotations, err = infiniband.GetNodeAnnotations(ctx, b.clientset, b.restConfig, b.nodeName, section)
	case lambdai.NAME:
		annotations, err = lambdai.GetNodeAnnotations(ctx, b.clientset, b.nodeName)
	case lldp.NAME_K8S:
		lldpData, lldpErr := lldp.GetNodeData(ctx, b.nodeName, b.config.Provider.Params)
		if lldpErr != nil {
			return nil, lldpErr
		}
		return &collectedNodeData{annotations: lldpData.Annotations, nicRails: lldpData.NICRails}, nil
	case "":
		return nil, fmt.Errorf("must set provider")
	default:
		return nil, fmt.Errorf("unsupported provider %q", b.config.Provider.Name)
	}
	if err != nil {
		return nil, err
	}
	return &collectedNodeData{annotations: annotations}, nil
}

func mergeNodeAnnotations(node *corev1.Node, annotations map[string]string) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	for key, value := range annotations {
		if value == "" {
			delete(node.Annotations, key)
		} else {
			node.Annotations[key] = value
		}
	}
}
