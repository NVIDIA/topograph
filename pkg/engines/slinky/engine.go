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
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/k8s"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/topology"
	"github.com/NVIDIA/topograph/pkg/translate"
)

const (
	NAME = "slinky"

	ConfigUpdateModeNone         = "none"
	ConfigUpdateModeSkeletonOnly = "skeleton-only"

	dynamicShowPartitionNodes = "\tNodes=NONE"
)

type SlinkyEngine struct {
	config *rest.Config
	client kubernetes.Interface
	params *Params
}

type Params struct {
	slurm.BaseParams `mapstructure:",squash"`
	// Namespace specifies the namespace where Slinky cluster is deployed
	Namespace string `mapstructure:"namespace"`
	// PodSelector specifies slurmd pods
	PodSelector metav1.LabelSelector `mapstructure:"podSelector"`
	// NodeSelector (optional) specifies nodes running slurmd pods
	NodeSelector map[string]string `mapstructure:"nodeSelector"`
	// ConfigMapName specifies the name of the configmap containing topology config
	ConfigMapName string `mapstructure:"topologyConfigmapName"`
	// ConfigPath specifies the topology config filename inside the configmap
	ConfigPath string `mapstructure:"topologyConfigPath"`
	// UseDynamicNodes specifies whether to use dynamic nodes for reporting: true or false
	UseDynamicNodes bool `mapstructure:"useDynamicNodes" default:"false"`
	// ConfigUpdateMode specifies the mode for updating the slurm config: valid values {"none", "skeleton-only"}
	ConfigUpdateMode string `mapstructure:"configUpdateMode,omitempty"`
	// Topologies specifies per-partition topology configuration
	Topologies map[string]*Topology `mapstructure:"topologies,omitempty"`

	// derived fields
	podListOpt  *metav1.ListOptions
	nodeListOpt *metav1.ListOptions
}

// Topology is the slinky-specific per-partition topology config.
// It extends slurm.Topology with PodSelector, which selects the pods whose
// hosts make up the partition. Exactly one of Nodes or PodSelector may be set;
// if neither is set, the engine falls back to "scontrol show partition".
type Topology struct {
	slurm.Topology `mapstructure:",squash"`
	// PodSelector selects the slurmd pods belonging to this partition.
	PodSelector metav1.LabelSelector `mapstructure:"podSelector"`
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(_ context.Context, params engines.Config) (engines.Engine, *httperr.Error) {
	p, err := getParameters(params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, err.Error())
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

	// Validate config update mode
	if len(p.ConfigUpdateMode) != 0 && p.ConfigUpdateMode != ConfigUpdateModeNone && p.ConfigUpdateMode != ConfigUpdateModeSkeletonOnly {
		return nil, fmt.Errorf("invalid configUpdateMode: %s, must be either %s, or %s", p.ConfigUpdateMode, ConfigUpdateModeNone, ConfigUpdateModeSkeletonOnly)
	}

	sel, err := metav1.LabelSelectorAsSelector(&p.PodSelector)
	if err != nil {
		return nil, err
	}
	p.podListOpt = &metav1.ListOptions{
		LabelSelector: sel.String(),
	}

	if len(p.NodeSelector) != 0 {
		p.nodeListOpt = &metav1.ListOptions{
			LabelSelector: labels.Set(p.NodeSelector).String(),
		}
	}

	for key, val := range map[string]string{
		topology.KeyNamespace:         p.Namespace,
		topology.KeyPodSelector:       p.podListOpt.LabelSelector,
		topology.KeyTopoConfigPath:    p.ConfigPath,
		topology.KeyTopoConfigmapName: p.ConfigMapName,
	} {
		if len(val) == 0 {
			return nil, fmt.Errorf("must specify engine parameter %q", key)
		}
	}

	for name, t := range p.Topologies {
		if t == nil {
			return nil, fmt.Errorf("topology %q: nil entry", name)
		}
		if len(t.Nodes) != 0 && !isEmptySelector(&t.PodSelector) {
			return nil, fmt.Errorf("topology %q: cannot set both nodes and podSelector", name)
		}
	}

	return p, nil
}

func isEmptySelector(sel *metav1.LabelSelector) bool {
	return sel == nil || (len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0)
}

func (eng *SlinkyEngine) GetComputeInstances(ctx context.Context, _ engines.Environment) ([]topology.ComputeInstances, *httperr.Error) {

	nodes, nodeMap, err := eng.getClusterNodes(ctx)
	if err != nil {
		return nil, err
	}

	return getComputeInstances(nodes, nodeMap)
}

func (eng *SlinkyEngine) getClusterNodes(ctx context.Context) (*corev1.NodeList, map[string]string, *httperr.Error) {
	nodes, err := k8s.GetNodes(ctx, eng.client, eng.params.nodeListOpt)
	if err != nil {
		return nil, nil, httperr.NewError(http.StatusBadGateway, err.Error())
	}

	pods, err := eng.client.CoreV1().Pods(eng.params.Namespace).List(ctx, *eng.params.podListOpt)
	if err != nil {
		return nil, nil, httperr.NewError(http.StatusBadGateway,
			fmt.Sprintf("failed to list SLURM pods in the cluster: %v", err))
	}

	klog.V(4).Infof("Found %d pods in %q namespace with selector %q", len(pods.Items), eng.params.Namespace, eng.params.podListOpt.LabelSelector)

	// map k8s host name to SLURM host name
	nodeMap := make(map[string]string)
	for _, pod := range pods.Items {
		if !k8s.IsPodReady(&pod) {
			continue
		}
		host, ok := pod.Labels[topology.KeySlurmNodeName]
		if !ok {
			host = pod.Spec.Hostname
		}
		klog.V(4).Infof("Mapping k8s node %s to SLURM node %s", pod.Spec.NodeName, host)
		nodeMap[pod.Spec.NodeName] = host
	}
	return nodes, nodeMap, nil
}

func getComputeInstances(nodes *corev1.NodeList, nodeMap map[string]string) ([]topology.ComputeInstances, *httperr.Error) {
	regions := make(map[string]map[string]string)
	regionNames := []string{}
	for _, node := range nodes.Items {
		hostName, ok := nodeMap[node.Name]
		if !ok {
			klog.V(4).Infof("Cannot resolve k8s node %q", node.Name)
			continue
		}
		instance, ok := node.Annotations[topology.KeyNodeInstance]
		if !ok {
			klog.Warningf("missing %q annotation in node %s", topology.KeyNodeInstance, node.Name)
			continue
		}
		region, ok := node.Annotations[topology.KeyNodeRegion]
		if !ok {
			klog.Warningf("missing %q annotation in node %s", topology.KeyNodeRegion, node.Name)
			continue
		}
		klog.V(4).InfoS("Adding compute instance", "host", hostName, "node", node.Name, "instance", instance, "region", region)
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
		annotations[topology.KeyConfigMapBlockSizes] = intToStr(eng.params.BlockSizes)
	}

	return annotations
}

func (eng *SlinkyEngine) GenerateOutput(ctx context.Context, graph *topology.Graph, _ map[string]any) ([]byte, *httperr.Error) {
	p := eng.params

	resolvedTopologies, err := eng.resolveTopologies(ctx)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	topologyNodeFinder := &slurm.TopologyNodeFinder{
		GetPartitionNodes: eng.getPartitionNodes,
		Params:            []any{p.Namespace},
	}
	cfg, err := slurm.GetTranslateConfig(ctx, &p.BaseParams, resolvedTopologies, topologyNodeFinder)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	nt, err := translate.NewNetworkTopology(graph, cfg)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	// Get desired topology from root topology graph
	buf := &bytes.Buffer{}
	topologies, httpErr := nt.GenerateTopologyConfig(buf, p.ConfigUpdateMode == ConfigUpdateModeSkeletonOnly)
	if httpErr != nil {
		return nil, httpErr
	}
	desiredTopology := buf.String()

	// If the slurm config update mode is not none, update the slurm config
	if p.ConfigUpdateMode != ConfigUpdateModeNone {
		data := map[string]string{p.ConfigPath: desiredTopology}
		if err := eng.UpdateTopologyConfigmap(ctx, p.ConfigMapName, p.Namespace, data); err != nil {
			return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
		}
	}

	// For dynamic mode, perform reconciliation using the latest topology information from the provider (root) and the cluster (nodes and their annotations)
	if p.UseDynamicNodes {
		httpErr := eng.performReconciliation(ctx, nt, topologies)
		if httpErr != nil {
			return nil, httpErr
		}
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

		changed := false
		for key, value := range data {
			if cm.Data[key] != value {
				changed = true
				break
			}
		}

		if !changed {
			klog.Infof("No changes to configmap %s/%s found, skipping update", namespace, name)
			return nil
		}

		// If the config map data is nil, create a new map
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		maps.Copy(cm.Data, data)

		// Apply annotations to existing ConfigMap
		if cm.Annotations == nil {
			cm.Annotations = make(map[string]string)
		}
		maps.Copy(cm.Annotations, annotations)

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

// resolveTopologies converts slinky.Topologies into slurm.Topology entries,
// resolving per-partition pod selectors into concrete node lists. Entries
// with explicit Nodes pass through; entries with neither Nodes nor PodSelector
// are left with empty Nodes so slurm.GetTranslateConfig invokes the scontrol
// fallback.
func (eng *SlinkyEngine) resolveTopologies(ctx context.Context) (map[string]*slurm.Topology, error) {
	if len(eng.params.Topologies) == 0 {
		return nil, nil
	}
	out := make(map[string]*slurm.Topology, len(eng.params.Topologies))
	for name, t := range eng.params.Topologies {
		st := t.Topology
		if len(st.Nodes) == 0 && !isEmptySelector(&t.PodSelector) {
			nodes, err := eng.listPartitionNodes(ctx, &t.PodSelector)
			if err != nil {
				return nil, fmt.Errorf("topology %q: %w", name, err)
			}
			st.Nodes = nodes
		}
		out[name] = &st
	}
	return out, nil
}

// listPartitionNodes lists pods in the engine's namespace matching sel and
// returns the corresponding SLURM node names from the KeySlurmNodeName label
// (falling back to pod.Spec.Hostname). Pods that are not Ready are skipped,
// mirroring the main pod-listing path.
func (eng *SlinkyEngine) listPartitionNodes(ctx context.Context, sel *metav1.LabelSelector) ([]string, error) {
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, err
	}
	pods, err := eng.client.CoreV1().Pods(eng.params.Namespace).List(ctx, metav1.ListOptions{LabelSelector: s.String()})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods with selector %q: %v", s.String(), err)
	}
	names := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if !k8s.IsPodReady(&pod) {
			klog.Warningf("topology: skipped not Ready pod %s/%s (selector %s)", pod.Namespace, pod.Name, s.String())
			continue
		}
		host, ok := pod.Labels[topology.KeySlurmNodeName]
		if !ok {
			host = pod.Spec.Hostname
		}
		if host == "" {
			klog.Warningf("topology: cannot find Slurm hostname for pod %s/%s (selector %s)", pod.Namespace, pod.Name, s.String())
			continue
		}
		names = append(names, host)
	}
	return names, nil
}

func (eng *SlinkyEngine) getPartitionNodes(ctx context.Context, partition string, params []any) (string, error) {
	if eng.params.UseDynamicNodes {
		klog.Infof("Skipping - scontrol show partition - when using useDynamicNodes flag")
		return dynamicShowPartitionNodes, nil
	}

	if len(params) != 1 {
		return "", fmt.Errorf("getPartitionNodes expects a namespace as a parameter")
	}
	namespace, ok := params[0].(string)
	if !ok {
		return "", fmt.Errorf("getPartitionNodes expects a string parameter")
	}

	labels := map[string]string{"app.kubernetes.io/component": "login"}
	pods, err := k8s.GetPodsByLabels(ctx, eng.client, namespace, labels)
	if err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		cmd := []string{"scontrol", "show", "partition", partition}
		buf, err := k8s.ExecInPod(ctx, eng.client, eng.config, pod.Name, pod.Namespace, cmd)
		if err != nil {
			return "", err
		}

		return buf.String(), nil
	}

	return "", fmt.Errorf("no running pods with labels %v", labels)
}

func (eng *SlinkyEngine) performReconciliation(ctx context.Context, nt *translate.NetworkTopology, topologies []*translate.TopologyUnit) *httperr.Error {

	nodes, nodeMap, err := eng.getClusterNodes(ctx)
	if err != nil {
		return err
	}

	// Update node annotations based on the desired topology and the current cluster state.
	// This will trigger Slinky to reconfigure the nodes accordingly.
	for _, node := range nodes.Items {
		slurmName, ok := nodeMap[node.Name]
		if !ok {
			klog.V(4).Infof("Skipping node %s as it does not have a corresponding SLURM name", node.Name)
			continue
		}

		if httpErr := eng.updateNodeAnnotation(ctx, &node, slurmName, nt, topologies); httpErr != nil {
			return httpErr
		}
		klog.V(4).Infof("Successfully updated annotation for node %s (SLURM name: %s)", node.Name, slurmName)

	}

	// Removed nodes: do nothing

	return nil
}

func (eng *SlinkyEngine) updateNodeAnnotation(ctx context.Context, node *corev1.Node, slurmName string, nt *translate.NetworkTopology, topologies []*translate.TopologyUnit) *httperr.Error {

	// Get the topology desiredSpec for the node based on the desired topologies
	desiredSpec, httpErr := nt.GetNodeTopologySpec(slurmName, topologies)
	if httpErr != nil {
		return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get topology spec for node %s: %v", slurmName, httpErr))
	}

	//If the topology spec is empty, no topology information is available for the node from the provider.
	//In this case, we can skip the annotation update to avoid unnecessary node reconfiguration by Slinky.
	if desiredSpec == "" {
		klog.V(4).Infof("Node %s (SLURM name: %s) received no topology spec from the provider, skipping annotation update", node.Name, slurmName)
		return nil
	}

	//Get the node object again to ensure we have the latest resource version for update
	nodeObj, err := eng.client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get node %s: %v", node.Name, err))
	}

	// Update the topology annotation on the node
	if nodeObj.Annotations == nil {
		nodeObj.Annotations = make(map[string]string)
	}

	//Get the current topology spec annotation on the node and compare with the desired spec. If they are the same, skip the update to avoid unnecessary node reconfiguration by Slinky.
	currentSpec, exists := nodeObj.Annotations[topology.KeySlinkyTopologySpec]
	if exists && currentSpec == desiredSpec {
		klog.V(4).Infof("Node %s (SLURM name: %s) topology spec is up to date, skipping annotation update", node.Name, slurmName)
		return nil
	}

	klog.Infof("Updating node %s (SLURM name: %s) topology spec annotation. Current spec: %q, New spec: %q", node.Name, slurmName, currentSpec, desiredSpec)

	//Set the new topology spec annotation on the node. This will trigger Slinky to reconfigure the node according to the new topology.
	nodeObj.Annotations[topology.KeySlinkyTopologySpec] = desiredSpec

	// Update the node object in Kubernetes
	_, err = eng.client.CoreV1().Nodes().Update(ctx, nodeObj, metav1.UpdateOptions{})
	if err != nil {
		return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to update node annotation: %v", err))
	}

	return nil
}

func intToStr(input []int) string {
	strs := make([]string, len(input))
	for i, n := range input {
		strs[i] = strconv.Itoa(n)
	}
	return strings.Join(strs, ",")
}
