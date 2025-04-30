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

package oci

import (
	"context"

	v1 "k8s.io/api/core/v1"
)

type baseProvider struct {
}

// Engine support

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *baseProvider) GetComputeInstancesRegion(ctx context.Context) (string, error) {
	return getRegion(ctx)
}

// GetNodeRegion implements k8s.k8sNodeInfo
func (p *baseProvider) GetNodeRegion(node *v1.Node) (string, error) {
	return node.Labels["topology.kubernetes.io/region"], nil
}

// GetNodeInstance implements k8s.k8sNodeInfo
func (p *baseProvider) GetNodeInstance(node *v1.Node) (string, error) {
	return node.Spec.ProviderID, nil
}
