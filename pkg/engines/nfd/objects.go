/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package nfd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/api/nfd/v1alpha1"

	k8sengine "github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	nfdAPIGroup   = "nfd.k8s-sigs.io"
	nfdAPIVersion = "v1alpha1"
	nfdFeatureSet = "topograph.network"
	nfdSystemName = "system.name"
	nfdNodeName   = "nodename"

	nfdNodeFeatureKind      = "NodeFeature"
	nfdNodeFeatureGroupKind = "NodeFeatureGroup"
	topologyTypeAccelerator = "accelerator"
	topologyTypeLeaf        = "leaf"
	topologyTypeSpine       = "spine"
	topologyTypeCore        = "core"

	labelNFDNodeName = "nfd.node.kubernetes.io/node-name"
	labelManagedBy   = "app.kubernetes.io/managed-by"
	labelEngine      = "topograph.nvidia.com/engine"
	labelResource    = "topograph.nvidia.com/resource"
	labelGroupType   = "topograph.nvidia.com/group-type"

	managedByTopograph         = "topograph"
	resourceNodeFeature        = "nodefeature"
	resourceNodeFeatureGroup   = "nodefeaturegroup"
	annotationNodeName         = "topograph.nvidia.com/node-name"
	annotationTopologyLabelKey = "topograph.nvidia.com/label-key"
	annotationTopologyValue    = "topograph.nvidia.com/label-value"
)

var (
	nodeFeatureGVR = schema.GroupVersionResource{
		Group:    nfdAPIGroup,
		Version:  nfdAPIVersion,
		Resource: "nodefeatures",
	}
	nodeFeatureGroupGVR = schema.GroupVersionResource{
		Group:    nfdAPIGroup,
		Version:  nfdAPIVersion,
		Resource: "nodefeaturegroups",
	}
)

// buildNFDObjects converts node topology labels into per-node features and
// groups for each distinct topology value.
func buildNFDObjects(nodeLabels k8sengine.NodeLabelMap, gpuCliqueValues map[string]string) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	keys := k8sengine.CurrentTopologyLabelKeys()
	configuredLabels := [...]struct {
		kind string
		key  string
	}{
		{kind: topologyTypeAccelerator, key: keys.Accelerator},
		{kind: topologyTypeLeaf, key: keys.Leaf},
		{kind: topologyTypeSpine, key: keys.Spine},
		{kind: topologyTypeCore, key: keys.Core},
	}
	groupValues := make(map[string]map[string]string)
	nodeFeatures := make([]*unstructured.Unstructured, 0, len(nodeLabels))

	for _, nodeName := range slices.Sorted(maps.Keys(nodeLabels)) {
		if errs := validation.IsValidLabelValue(nodeName); len(errs) != 0 {
			return nil, nil, fmt.Errorf("node name %q cannot be used as %q label value: %s",
				nodeName, labelNFDNodeName, strings.Join(errs, "; "))
		}

		labels := nodeLabels[nodeName]
		elements := make(map[string]string, len(configuredLabels))
		for _, label := range configuredLabels {
			labelKey := label.key
			value := ""
			if label.kind == topologyTypeAccelerator {
				if gpuCliqueValue := gpuCliqueValues[nodeName]; gpuCliqueValue != "" {
					labelKey = topology.KeyNvidiaGPUClique
					value = gpuCliqueValue
				}
			}
			if labelKey == "" {
				continue
			}
			if value == "" {
				value = strings.TrimSpace(labels[labelKey])
			}
			if value == "" {
				continue
			}

			elements[label.kind] = value
			if _, ok := groupValues[label.kind]; !ok {
				groupValues[label.kind] = make(map[string]string)
			}
			groupValues[label.kind][value] = labelKey
		}
		if len(elements) == 0 {
			continue
		}

		nodeFeature, err := makeNodeFeature(nodeName, elements)
		if err != nil {
			return nil, nil, err
		}
		nodeFeatures = append(nodeFeatures, nodeFeature)
	}

	nodeFeatureGroups := make([]*unstructured.Unstructured, 0)
	for _, kind := range topologyTypeOrder {
		valuesByLabelKey, ok := groupValues[kind]
		if !ok {
			continue
		}
		for _, value := range slices.Sorted(maps.Keys(valuesByLabelKey)) {
			nodeFeatureGroup, err := makeNodeFeatureGroup(kind, value, valuesByLabelKey[value])
			if err != nil {
				return nil, nil, err
			}
			nodeFeatureGroups = append(nodeFeatureGroups, nodeFeatureGroup)
		}
	}

	return nodeFeatures, nodeFeatureGroups, nil
}

// makeNodeFeature builds the NFD feature data published for one node.
func makeNodeFeature(nodeName string, elements map[string]string) (*unstructured.Unstructured, error) {
	obj := &nfdv1alpha1.NodeFeature{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nfdAPIGroup + "/" + nfdAPIVersion,
			Kind:       nfdNodeFeatureKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: stableObjectName("topograph-node", nodeName),
			Labels: map[string]string{
				labelNFDNodeName: nodeName,
				labelManagedBy:   managedByTopograph,
				labelEngine:      NAME,
				labelResource:    resourceNodeFeature,
			},
			Annotations: map[string]string{
				annotationNodeName: nodeName,
			},
		},
		Spec: nfdv1alpha1.NodeFeatureSpec{
			Features: nfdv1alpha1.Features{
				Attributes: map[string]nfdv1alpha1.AttributeFeatureSet{
					nfdSystemName: {
						Elements: map[string]string{nfdNodeName: nodeName},
					},
					nfdFeatureSet: {
						Elements: elements,
					},
				},
			},
		},
	}

	return toUnstructured(obj)
}

// makeNodeFeatureGroup builds an NFD group that matches one topology value.
func makeNodeFeatureGroup(kind, value, labelKey string) (*unstructured.Unstructured, error) {
	matchExpressions := nfdv1alpha1.MatchExpressionSet{
		kind: {
			Op:    nfdv1alpha1.MatchIn,
			Value: nfdv1alpha1.MatchValue{value},
		},
	}
	obj := &nfdv1alpha1.NodeFeatureGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nfdAPIGroup + "/" + nfdAPIVersion,
			Kind:       nfdNodeFeatureGroupKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: stableObjectName("topograph-"+kind, value),
			Labels: map[string]string{
				labelManagedBy: managedByTopograph,
				labelEngine:    NAME,
				labelResource:  resourceNodeFeatureGroup,
				labelGroupType: kind,
			},
			Annotations: map[string]string{
				annotationTopologyLabelKey: labelKey,
				annotationTopologyValue:    value,
			},
		},
		Spec: nfdv1alpha1.NodeFeatureGroupSpec{
			Rules: []nfdv1alpha1.GroupRule{
				{
					Name: fmt.Sprintf("%s equals %s", kind, value),
					MatchFeatures: nfdv1alpha1.FeatureMatcher{
						{
							Feature:          nfdFeatureSet,
							MatchExpressions: &matchExpressions,
						},
					},
				},
			},
		},
	}

	return toUnstructured(obj)
}

// toUnstructured converts a typed NFD API object for use with the dynamic client.
func toUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("convert NFD object to unstructured: %w", err)
	}
	return &unstructured.Unstructured{Object: object}, nil
}

// applyObjects creates or updates node features and feature groups concurrently.
func (eng *NfdEngine) applyObjects(
	ctx context.Context,
	nodeFeatures []*unstructured.Unstructured,
	nodeFeatureGroups []*unstructured.Unstructured,
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		for _, obj := range nodeFeatures {
			if err := eng.upsertObject(ctx, nodeFeatureGVR, obj); err != nil {
				return err
			}
		}
		return nil
	})
	g.Go(func() error {
		for _, obj := range nodeFeatureGroups {
			if err := eng.upsertObject(ctx, nodeFeatureGroupGVR, obj); err != nil {
				return err
			}
		}
		return nil
	})
	return g.Wait()
}

// cleanupObjects removes managed node features and feature groups that are no
// longer part of the generated topology.
func (eng *NfdEngine) cleanupObjects(
	ctx context.Context,
	nodeFeatures []*unstructured.Unstructured,
	nodeFeatureGroups []*unstructured.Unstructured,
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return eng.cleanupResource(ctx, nodeFeatureGVR, resourceNodeFeature, objectNames(nodeFeatures))
	})
	g.Go(func() error {
		return eng.cleanupResource(ctx, nodeFeatureGroupGVR, resourceNodeFeatureGroup, objectNames(nodeFeatureGroups))
	})
	return g.Wait()
}

// upsertObject patches an existing object or creates it when it does not exist.
func (eng *NfdEngine) upsertObject(ctx context.Context, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	resource := eng.resource(gvr)
	data, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels":      obj.GetLabels(),
			"annotations": obj.GetAnnotations(),
		},
		"spec": obj.Object["spec"],
	})
	if err != nil {
		return err
	}

	_, err = resource.Patch(ctx, obj.GetName(), types.MergePatchType, data, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, obj.DeepCopy(), metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			_, err = resource.Patch(ctx, obj.GetName(), types.MergePatchType, data, metav1.PatchOptions{})
		}
	}

	return err
}

// cleanupResource deletes managed objects whose names are absent from desired.
func (eng *NfdEngine) cleanupResource(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	resourceKind string,
	desired map[string]struct{},
) error {
	selector := labels.Set{
		labelManagedBy: managedByTopograph,
		labelEngine:    NAME,
		labelResource:  resourceKind,
	}.String()
	res := eng.resource(gvr)
	list, err := res.List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}

	for i := range list.Items {
		name := list.Items[i].GetName()
		if _, ok := desired[name]; ok {
			continue
		}
		if err := res.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// resource returns the dynamic resource client for the configured scope.
func (eng *NfdEngine) resource(gvr schema.GroupVersionResource) dynamic.ResourceInterface {
	resource := eng.dynamicClient.Resource(gvr)
	if eng.params != nil && eng.params.Namespace != "" {
		return resource.Namespace(eng.params.Namespace)
	}
	return resource
}

// objectNames returns the names of objects as a set.
func objectNames(objects []*unstructured.Unstructured) map[string]struct{} {
	names := make(map[string]struct{}, len(objects))
	for _, obj := range objects {
		names[obj.GetName()] = struct{}{}
	}
	return names
}

var topologyTypeOrder = []string{
	topologyTypeAccelerator,
	topologyTypeLeaf,
	topologyTypeSpine,
	topologyTypeCore,
}

// stableObjectName produces a deterministic DNS label from a prefix and value.
func stableObjectName(prefix, value string) string {
	prefix = dnsLabelPart(prefix)
	if prefix == "" {
		prefix = "topograph"
	}

	hash := shortHash(value)
	part := dnsLabelPart(value)
	if part == "" {
		part = "value"
	}

	maxPartLen := 63 - len(prefix) - len(hash) - 2
	if maxPartLen < 1 {
		maxPartLen = 1
	}
	if len(part) > maxPartLen {
		part = strings.Trim(part[:maxPartLen], "-")
		if part == "" {
			part = "value"
		}
	}

	return fmt.Sprintf("%s-%s-%s", prefix, part, hash)
}

// shortHash returns the first eight hexadecimal characters of a SHA-256 hash.
func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

// dnsLabelPart normalizes a value into characters allowed in a DNS label.
func dnsLabelPart(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false

	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
