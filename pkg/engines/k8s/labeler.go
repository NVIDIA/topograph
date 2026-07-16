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

package k8s

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"strings"

	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	DefaultLabelAccelerator = "network.topology.nvidia.com/accelerator"
	DefaultLabelLeaf        = "network.topology.nvidia.com/leaf"
	DefaultLabelSpine       = "network.topology.nvidia.com/spine"
	DefaultLabelCore        = "network.topology.nvidia.com/core"
)

var (
	labelAccelerator, labelLeaf, labelSpine, labelCore string
)

func InitLabels(accelerator, leaf, spine, core string) {
	labelAccelerator = accelerator
	labelLeaf = leaf
	labelSpine = spine
	labelCore = core
}

type labelKeySet map[string]struct{}

type nodeLabelPlan struct {
	Desired        map[string]string
	ManagedKeys    labelKeySet
	acceleratorKey string
}

type nodeLabelPlans map[string]*nodeLabelPlan

type nodeLabelReconciler interface {
	reconcileNodeLabelPlans(context.Context, nodeLabelPlans) error
}

type topologyLabeler struct {
	mapper         map[string]string
	acceleratorKey string
	tierKeys       [3]string
}

func newTopologyLabeler() *topologyLabeler {
	return &topologyLabeler{
		mapper:         make(map[string]string),
		acceleratorKey: labelAccelerator,
		tierKeys:       [3]string{labelLeaf, labelSpine, labelCore},
	}
}

func (l *topologyLabeler) applyNodeLabels(ctx context.Context, graph *topology.Graph, labeler nodeLabelReconciler) error {
	plans, err := l.buildNodeLabelPlans(graph)
	if err != nil || len(plans) == 0 {
		return err
	}
	return labeler.reconcileNodeLabelPlans(ctx, plans)
}

func (l *topologyLabeler) buildNodeLabelPlans(graph *topology.Graph) (nodeLabelPlans, error) {
	plans := make(nodeLabelPlans)
	if graph == nil {
		return plans, nil
	}

	if err := l.getDomainLabels(graph.Domains, plans); err != nil {
		return nil, err
	}

	if treeRoot := graph.Tiers; treeRoot != nil {
		layers := []string{}
		if treeRoot.ID != "" {
			layers = append(layers, treeRoot.ID)
		}
		if err := l.getTierLabels(treeRoot, plans, layers); err != nil {
			return nil, err
		}
	}
	for nodeName, plan := range plans {
		if len(plan.ManagedKeys) == 0 {
			delete(plans, nodeName)
		}
	}

	return plans, nil
}

func (l *topologyLabeler) getDomainLabels(domains topology.DomainMap, plans nodeLabelPlans) error {
	if !validLabelKey(l.acceleratorKey) {
		return nil
	}

	domainByNode := make(map[string]string)
	for _, domainName := range slices.Sorted(maps.Keys(domains)) {
		domain := domains[domainName]
		for _, nodeName := range slices.Sorted(maps.Keys(domain)) {
			if nodeName == "" {
				return fmt.Errorf("accelerator domain %q contains an empty node name", domainName)
			}
			if previous, ok := domainByNode[nodeName]; ok && previous != domainName {
				return fmt.Errorf("multiple accelerator labels %s, %s for node %s", previous, domainName, nodeName)
			}
			domainByNode[nodeName] = domainName

			plan := getOrCreatePlan(plans, nodeName)
			plan.acceleratorKey = l.acceleratorKey
			addManagedKey(plan, l.acceleratorKey)
			if err := l.addDesiredLabel(nodeName, plan, l.acceleratorKey, domainName); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *topologyLabeler) getTierLabels(v *topology.Vertex, plans nodeLabelPlans, layers []string) error {
	if v == nil {
		return fmt.Errorf("topology contains a nil vertex")
	}

	if len(v.Vertices) == 0 { // compute node
		// An empty synthetic root represents an empty or partial topology, not a
		// Kubernetes Node.
		if v.Name == "" {
			return nil
		}
		if len(layers) != 0 && v.ID != layers[0] {
			return fmt.Errorf("instance ID mismatch: expected %s, got %s", v.ID, layers[0])
		}

		plan := getOrCreatePlan(plans, v.Name)
		for _, key := range l.tierKeys {
			addManagedKey(plan, key)
		}
		for i, sw := range layers[1:] {
			if sw == "" || i >= len(l.tierKeys) {
				break
			}
			if err := l.addDesiredLabel(v.Name, plan, l.tierKeys[i], sw); err != nil {
				return err
			}
		}
		return nil
	}

	for _, vertexID := range slices.Sorted(maps.Keys(v.Vertices)) {
		w := v.Vertices[vertexID]
		if w == nil {
			return fmt.Errorf("topology vertex %q contains a nil child", v.ID)
		}
		if err := l.getTierLabels(w, plans, append([]string{w.ID}, layers...)); err != nil {
			return err
		}
	}

	return nil
}

func getOrCreatePlan(plans nodeLabelPlans, nodeName string) *nodeLabelPlan {
	plan, ok := plans[nodeName]
	if ok {
		return plan
	}
	plan = &nodeLabelPlan{
		Desired:     make(map[string]string),
		ManagedKeys: make(labelKeySet),
	}
	plans[nodeName] = plan
	return plan
}

func validLabelKey(key string) bool {
	return strings.TrimSpace(key) != ""
}

func addManagedKey(plan *nodeLabelPlan, key string) {
	if validLabelKey(key) {
		plan.ManagedKeys[key] = struct{}{}
	}
}

func (l *topologyLabeler) addDesiredLabel(nodeName string, plan *nodeLabelPlan, key, value string) error {
	if !validLabelKey(key) {
		return nil
	}
	value = l.checkLabel(value)
	if previous, ok := plan.Desired[key]; ok && previous != value {
		return fmt.Errorf(
			"conflicting desired values %q and %q for label %q on node %q",
			previous,
			value,
			key,
			nodeName,
		)
	}
	plan.Desired[key] = value
	return nil
}

// checkLabel checks the length of the label value.
// If more than 63 characters (Kubernetes limit), it will replace it with hash
func (l *topologyLabeler) checkLabel(val string) string {
	v, ok := l.mapper[val]
	if ok {
		return v
	}

	if len(val) <= 63 {
		v = val
	} else {
		h := fnv.New64a()
		h.Write([]byte(val))
		v = fmt.Sprintf("x%x", h.Sum64())
	}

	l.mapper[val] = v
	return v
}
