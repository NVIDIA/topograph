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

package slinky

import (
	"bytes"
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const NAME = "slinky"

type SlinkyEngine struct {
	config *rest.Config
	client *kubernetes.Clientset
	params *Params
}

type Params struct {
	Namespace     string `mapstructure:"namespace"`
	PodLabel      string `mapstructure:"pod_label"`
	Plugin        string `mapstructure:"plugin"`
	BlockSizes    string `mapstructure:"block_sizes"`
	ConfigPath    string `mapstructure:"topology_config_path"`
	ConfigMapName string `mapstructure:"topology_configmap_name"`
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, params engines.Config) (engines.Engine, error) {
	return New(params)
}

func New(params engines.Config) (*SlinkyEngine, error) {
	p, err := getParameters(params)
	if err != nil {
		return nil, err
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &SlinkyEngine{
		config: config,
		client: client,
		params: p,
	}, nil
}

func getParameters(params engines.Config) (*Params, error) {
	p := &Params{}
	if err := config.Decode(params, p); err != nil {
		return nil, err
	}

	for key, val := range map[string]string{
		topology.KeyNamespace:         p.Namespace,
		topology.KeyPodLabel:          p.PodLabel,
		topology.KeyTopoConfigPath:    p.ConfigPath,
		topology.KeyTopoConfigmapName: p.ConfigMapName,
	} {
		if len(val) == 0 {
			return nil, fmt.Errorf("must specify engine parameter %q", key)
		}
	}

	return p, nil
}

func (eng *SlinkyEngine) GetComputeInstances(ctx context.Context, _ engines.Environment) ([]topology.ComputeInstances, error) {
	nodes, err := eng.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list node in the cluster: %v", err)
	}

	opt := metav1.ListOptions{
		LabelSelector: eng.params.PodLabel,
	}
	pods, err := eng.client.CoreV1().Pods(eng.params.Namespace).List(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to list SLURM pods in the cluster: %v", err)
	}

	// map k8s host name to SLURM host name
	nodeMap := make(map[string]string)
	for _, pod := range pods.Items {
		klog.V(4).Infof("Mapping %s to %s", pod.Spec.NodeName, pod.Spec.Hostname)
		nodeMap[pod.Spec.NodeName] = pod.Spec.Hostname
	}

	return getComputeInstances(nodes, nodeMap)
}

func getComputeInstances(nodes *corev1.NodeList, nodeMap map[string]string) ([]topology.ComputeInstances, error) {
	regions := make(map[string]map[string]string)
	regionNames := []string{}
	for _, node := range nodes.Items {
		instance, ok := node.Annotations[topology.KeyNodeInstance]
		if !ok {
			return nil, fmt.Errorf("missing %q annotation in node %s", topology.KeyNodeInstance, node.Name)
		}
		region, ok := node.Annotations[topology.KeyNodeRegion]
		if !ok {
			return nil, fmt.Errorf("missing %q annotation in node %s", topology.KeyNodeRegion, node.Name)
		}
		if _, ok = regions[region]; !ok {
			regions[region] = make(map[string]string)
			regionNames = append(regionNames, region)
		}
		regions[region][instance] = nodeMap[node.Name]
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for _, region := range regionNames {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: regions[region]})
	}

	return cis, nil
}

func (eng *SlinkyEngine) GenerateOutput(ctx context.Context, tree *topology.Vertex, params map[string]any) ([]byte, error) {
	p := eng.params

	if tree.Metadata == nil {
		tree.Metadata = make(map[string]string)
	}
	if len(p.Plugin) != 0 {
		tree.Metadata[topology.KeyPlugin] = p.Plugin
	}
	if len(p.BlockSizes) != 0 {
		tree.Metadata[topology.KeyBlockSizes] = p.BlockSizes
	}

	buf := &bytes.Buffer{}
	err := translate.Write(buf, tree)
	if err != nil {
		return nil, err
	}
	err = eng.UpdateTopologyConfigmap(ctx, p.ConfigMapName, p.Namespace, map[string]string{p.ConfigPath: buf.String()})
	if err != nil {
		return nil, err
	}

	return []byte("OK\n"), nil
}

func (eng *SlinkyEngine) UpdateTopologyConfigmap(ctx context.Context, name, namespace string, data map[string]string) error {
	klog.Infof("Updating topology config %s/%s", namespace, name)

	verb := "get"
	cmClient := eng.client.CoreV1().ConfigMaps(namespace)
	cm, err := cmClient.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		verb = "update"
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		maps.Copy(cm.Data, data)
		_, err = cmClient.Update(ctx, cm, metav1.UpdateOptions{})
	} else if errors.IsNotFound(err) {
		verb = "create"
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: data,
		}
		_, err = cmClient.Create(ctx, cm, metav1.CreateOptions{})
	}

	if err != nil {
		return fmt.Errorf("failed to %s configmap %s/%s: %v",
			verb, namespace, name, err)
	}

	klog.Infof("Successfully %sd configmap %s/%s", verb, namespace, name)
	return nil
}
