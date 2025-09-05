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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
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
	slurm.BaseParams `mapstructure:",squash"`
	Namespace        string               `mapstructure:"namespace"`
	LabelSelector    metav1.LabelSelector `mapstructure:"podSelector"`
	ConfigPath       string               `mapstructure:"topologyConfigPath"`
	ConfigMapName    string               `mapstructure:"topologyConfigmapName"`

	// derived fields
	podSelector string
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

	selector, err := metav1.LabelSelectorAsSelector(&p.LabelSelector)
	if err != nil {
		return nil, err
	}
	p.podSelector = selector.String()

	for key, val := range map[string]string{
		topology.KeyNamespace:         p.Namespace,
		topology.KeyPodSelector:       p.podSelector,
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
	nodes, err := k8s.GetNodes(ctx, eng.client)
	if err != nil {
		return nil, err
	}

	opt := metav1.ListOptions{
		LabelSelector: eng.params.podSelector,
	}
	pods, err := eng.client.CoreV1().Pods(eng.params.Namespace).List(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to list SLURM pods in the cluster: %v", err)
	}

	klog.V(4).Infof("Found %d pods in %q namespace with selector %q", len(pods.Items), eng.params.Namespace, eng.params.podSelector)

	// map k8s host name to SLURM host name
	nodeMap := make(map[string]string)
	for _, pod := range pods.Items {
		host, ok := pod.Labels["slurm.node.name"]
		if !ok {
			host = pod.Spec.Hostname
		}
		klog.V(4).Infof("Mapping k8s node %s to SLURM node %s", pod.Spec.NodeName, host)
		nodeMap[pod.Spec.NodeName] = host
	}

	return getComputeInstances(nodes, nodeMap)
}

func getComputeInstances(nodes *corev1.NodeList, nodeMap map[string]string) ([]topology.ComputeInstances, error) {
	regions := make(map[string]map[string]string)
	regionNames := []string{}
	for _, node := range nodes.Items {
		hostName, ok := nodeMap[node.Name]
		if !ok {
			klog.V(4).Infof("Cannot resolve k8s node %q", node.Name)
			continue
		}
		klog.V(4).InfoS("Adding compute instance", "host", hostName, "node", node.Name)
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
		regions[region][instance] = hostName
	}

	cis := make([]topology.ComputeInstances, 0, len(regions))
	for _, region := range regionNames {
		cis = append(cis, topology.ComputeInstances{Region: region, Instances: regions[region]})
	}

	return cis, nil
}

// generateConfigMapAnnotations creates metadata annotations for ConfigMaps
func (eng *SlinkyEngine) generateConfigMapAnnotations() map[string]string {
	annotations := map[string]string{
		topology.KeyConfigMapEngine:            NAME,
		topology.KeyConfigMapTopologyManagedBy: "topograph",
		topology.KeyConfigMapLastUpdated:       time.Now().Format(time.RFC3339),
		topology.KeyConfigMapNamespace:         eng.params.Namespace,
	}

	// Add plugin-specific annotations if available
	if len(eng.params.Plugin) != 0 {
		annotations[topology.KeyConfigMapPlugin] = eng.params.Plugin
	}
	if len(eng.params.BlockSizes) != 0 {
		annotations[topology.KeyConfigMapBlockSizes] = eng.params.BlockSizes
	}

	return annotations
}

func (eng *SlinkyEngine) GenerateOutput(ctx context.Context, root *topology.Vertex, _ map[string]any) ([]byte, error) {
	p := eng.params

	topologyNodeFinder := &slurm.TopologyNodeFinder{
		GetTopologyNodes: eng.getTopologyNodes,
		Params:           []any{p.Namespace},
	}
	cfg, err := slurm.GetTranslateConfig(ctx, &p.BaseParams, topologyNodeFinder)
	if err != nil {
		return nil, err
	}

	nt, err := translate.NewNetworkTopology(root, cfg)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	err = nt.Generate(buf)
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

	annotations := eng.generateConfigMapAnnotations()
	verb := "get"
	cmClient := eng.client.CoreV1().ConfigMaps(namespace)
	cm, err := cmClient.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		verb = "update"
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		maps.Copy(cm.Data, data)

		// Apply annotations to existing ConfigMap
		if cm.ObjectMeta.Annotations == nil {
			cm.ObjectMeta.Annotations = make(map[string]string)
		}
		maps.Copy(cm.ObjectMeta.Annotations, annotations)

		_, err = cmClient.Update(ctx, cm, metav1.UpdateOptions{})
	} else if errors.IsNotFound(err) {
		verb = "create"
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: annotations,
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

func (eng *SlinkyEngine) getTopologyNodes(ctx context.Context, topo string, params []any) (string, error) {
	if len(params) != 1 {
		return "", fmt.Errorf("getTopologyNodes expects a namespace as a parameter")
	}
	namespace, ok := params[0].(string)
	if !ok {
		return "", fmt.Errorf("getTopologyNodes expects a string parameter")
	}

	labels := map[string]string{"app.kubernetes.io/component": "login"}
	pods, err := k8s.GetPodsByLabels(ctx, eng.client, namespace, labels)
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods with labels %v", labels)
	}

	cmd := []string{"scontrol", "show", "topology", topo}
	return k8s.ExecInPod(ctx, eng.client, eng.config, pods.Items[0].Name, pods.Items[0].Namespace, cmd)
}
