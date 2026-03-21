/*
 * Copyright (c) 2024-2025, NVIDIA CORPORATION.  All rights reserved.
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
	"flag"
	"fmt"
	"maps"
	"os"
	"strings"

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
	"github.com/NVIDIA/topograph/pkg/providers/nebius"
	"github.com/NVIDIA/topograph/pkg/providers/oci"
)

func main() {
	var provider string
	var ver bool
	var sets []string
	pflag.StringVar(&provider, "provider", "", "API provider")
	pflag.BoolVar(&ver, "version", false, "show the version")
	pflag.StringArrayVar(&sets, "set", []string{}, "extra key=value parameters")

	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	defer klog.Flush()

	if ver {
		fmt.Println("Version:", version.Version)
		os.Exit(0)
	}

	if err := mainInternal(provider, sets); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func mainInternal(provider string, sets []string) error {
	klog.InfoS("Starting node-data-broker", "provider", provider, "extras", sets)

	extras, err := getExtras(sets)
	if err != nil {
		return err
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to load in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	ctx := context.TODO()
	nodeName := os.Getenv("NODE_NAME")

	annotations, err := getAnnotations(ctx, clientset, config, provider, nodeName, extras)
	if err != nil {
		return err
	}
	klog.Infof("adding annotations %v in node %s for provider %s", annotations, nodeName, provider)

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", nodeName, err)
	}

	mergeNodeAnnotations(node, annotations)

	_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node: %v", err)
	}

	return nil
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

func getAnnotations(ctx context.Context, client *kubernetes.Clientset, config *rest.Config, provider, nodeName string, extras map[string]string) (map[string]string, error) {
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
