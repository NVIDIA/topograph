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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/providers/aws"
	"github.com/NVIDIA/topograph/pkg/providers/baremetal"
	"github.com/NVIDIA/topograph/pkg/providers/gcp"
	"github.com/NVIDIA/topograph/pkg/providers/nebius"
	"github.com/NVIDIA/topograph/pkg/providers/oci"
)

var GitTag string

func main() {
	var provider string
	var version bool
	flag.StringVar(&provider, "provider", "", "API provider")
	flag.BoolVar(&version, "version", false, "show the version")

	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	if version {
		fmt.Println("Version:", GitTag)
		os.Exit(0)
	}

	if err := mainInternal(provider); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func mainInternal(provider string) (err error) {
	ctx := context.TODO()
	hostname := os.Getenv("NODE_NAME")

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to load in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %v", err)
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, hostname, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %q: %v", hostname, err)
	}

	annotations, err := getAnnotations(ctx, clientset, config, provider, hostname)
	if err != nil {
		return err
	}
	mergeNodeAnnotations(node, annotations)

	_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node: %v", err)
	}

	return nil
}

func getAnnotations(ctx context.Context, client *kubernetes.Clientset, config *rest.Config, provider, hostname string) (annotations map[string]string, err error) {
	switch provider {
	case aws.NAME:
		annotations, err = aws.GetNodeAnnotations()
	case gcp.NAME:
		annotations, err = gcp.GetNodeAnnotations()
	case oci.NAME, oci.NAME_IMDS:
		annotations, err = oci.GetNodeAnnotations()
	case nebius.NAME:
		annotations, err = nebius.GetNodeAnnotations()
	case baremetal.NAME_IB:
		annotations, err = baremetal.GetNodeAnnotations(ctx, client, config, hostname)
	case "":
		err = fmt.Errorf("must set provider")
	default:
		err = fmt.Errorf("unsupported provider %q", provider)
	}
	if err == nil {
		klog.Infof("adding annotations %v in node %s for provider %s", annotations, hostname, provider)
	}

	return
}

func mergeNodeAnnotations(node *corev1.Node, annotations map[string]string) {
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	maps.Copy(node.Annotations, annotations)
}
