/*
 * Copyright (c) 2024-2026, NVIDIA CORPORATION.  All rights reserved.
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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/version"
	"github.com/NVIDIA/topograph/pkg/providers/aws"
	"github.com/NVIDIA/topograph/pkg/providers/dra"
	"github.com/NVIDIA/topograph/pkg/providers/gcp"
	"github.com/NVIDIA/topograph/pkg/providers/infiniband"
	"github.com/NVIDIA/topograph/pkg/providers/lambdai"
	"github.com/NVIDIA/topograph/pkg/providers/nebius"
	"github.com/NVIDIA/topograph/pkg/providers/oci"
)

const (
	defaultPort            = 8080
	defaultRefreshInterval = 5 * time.Minute
	readHeaderTimeout      = 5 * time.Second
	shutdownTimeout        = 5 * time.Second
)

type nodeBroker struct {
	clientset kubernetes.Interface
	config    *rest.Config
	provider  string
	sets      []string
	nodeName  string
}

func main() {
	var provider string
	var ver bool
	var sets []string
	var port int
	var refreshInterval time.Duration
	pflag.StringVar(&provider, "provider", "", "API provider")
	pflag.BoolVar(&ver, "version", false, "show the version")
	pflag.StringArrayVar(&sets, "set", []string{}, "extra key=value parameters")
	pflag.IntVar(&port, "port", defaultPort, "port for the health HTTP server")
	pflag.DurationVar(&refreshInterval, "refresh-interval", defaultRefreshInterval, "interval between node annotation refreshes after startup (0 disables periodic refresh)")

	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	defer klog.Flush()

	if ver {
		fmt.Println("Version:", version.Version)
		os.Exit(0)
	}

	if err := mainInternal(provider, sets, port, refreshInterval); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func mainInternal(provider string, sets []string, port int, refreshInterval time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clientset, config, err := newInClusterClientset()
	if err != nil {
		return err
	}

	broker := &nodeBroker{
		clientset: clientset,
		config:    config,
		provider:  provider,
		sets:      sets,
		nodeName:  os.Getenv("NODE_NAME"),
	}

	if err := broker.apply(ctx); err != nil {
		return err
	}

	if refreshInterval > 0 {
		go refreshNodeAnnotations(ctx, broker, refreshInterval)
	}

	// Keep the DaemonSet pod Running by serving a health endpoint until the pod
	// is terminated. Periodic annotation refresh runs in the background.
	return serveHealth(ctx, port)
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

type annotationApplier func(ctx context.Context) error

// refreshNodeAnnotations re-applies node annotations on a fixed interval until
// the context is cancelled. Failures are logged but do not terminate the pod.
func refreshNodeAnnotations(ctx context.Context, broker *nodeBroker, interval time.Duration) {
	runRefreshLoop(ctx, interval, func(ctx context.Context) error {
		return broker.apply(ctx)
	})
}

func runRefreshLoop(ctx context.Context, interval time.Duration, apply annotationApplier) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := apply(ctx); err != nil {
				klog.ErrorS(err, "periodic node annotation refresh failed")
			}
		}
	}
}

func (b *nodeBroker) apply(ctx context.Context) error {
	klog.InfoS("Applying node annotations", "provider", b.provider, "extras", b.sets)

	extras, err := getExtras(b.sets)
	if err != nil {
		return err
	}

	annotations, err := getAnnotations(ctx, b.clientset, b.config, b.provider, b.nodeName, extras)
	if err != nil {
		return err
	}
	klog.Infof("adding annotations %v in node %s for provider %s", annotations, b.nodeName, b.provider)

	node, err := b.clientset.CoreV1().Nodes().Get(ctx, b.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", b.nodeName, err)
	}

	mergeNodeAnnotations(node, annotations)

	_, err = b.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node: %v", err)
	}

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

func getExtras(sets []string) (map[string]string, error) {
	extras := make(map[string]string)
	for _, kv := range sets {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			key, val := parts[0], parts[1]
			if len(key) == 0 || len(val) == 0 {
				return nil, fmt.Errorf("invalid value %q for '--set': key/value cannot be empty", kv)
			}
			extras[key] = val
		} else {
			return nil, fmt.Errorf("invalid value %q for '--set': expected format '<key>=<value>'", kv)
		}
	}

	return extras, nil
}

func getAnnotations(ctx context.Context, client kubernetes.Interface, config *rest.Config, provider, nodeName string, extras map[string]string) (map[string]string, error) {
	switch provider {
	case aws.NAME:
		return aws.GetNodeAnnotations(ctx)
	case gcp.NAME:
		return gcp.GetNodeAnnotations(ctx)
	case oci.NAME:
		return oci.GetNodeAnnotations(ctx)
	case nebius.NAME:
		return nebius.GetNodeAnnotations(ctx)
	case dra.NAME:
		return dra.GetNodeAnnotations(ctx, nodeName)
	case infiniband.NAME_K8S:
		return infiniband.GetNodeAnnotations(ctx, client, config, nodeName, extras)
	case lambdai.NAME:
		return lambdai.GetNodeAnnotations(ctx, client, nodeName)
	case "":
		return nil, fmt.Errorf("must set provider")
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

func mergeNodeAnnotations(node *corev1.Node, annotations map[string]string) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	maps.Copy(node.Annotations, annotations)
}
