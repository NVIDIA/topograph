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

	"github.com/NVIDIA/topograph/pkg/topology"
)

type Labeler interface {
	AddNodeLabels(context.Context, string, map[string]string) error
}

type topologyLabeler struct {
	mapper map[string]string
}

func NewTopologyLabeler() *topologyLabeler {
	return &topologyLabeler{
		mapper: make(map[string]string),
	}
}

func (l *topologyLabeler) ApplyNodeLabels(ctx context.Context, v *topology.Vertex, labeler Labeler) error {
	if v == nil {
		return nil
	}
	levels := []string{}
	if len(v.ID) != 0 {
		levels = append(levels, v.ID)
	}

	return l.applyNodeLabels(ctx, v, labeler, levels)
}

func (l *topologyLabeler) applyNodeLabels(ctx context.Context, v *topology.Vertex, labeler Labeler, levels []string) error {
	if len(v.Vertices) == 0 { // compute node
		if len(levels) != 0 {
			if v.ID != levels[0] {
				return fmt.Errorf("instance ID mismatch: expected %s, got %s", v.ID, levels[0])
			}

			labels := make(map[string]string)
			for i, sw := range levels[1:] {
				if len(sw) == 0 {
					break
				}
				labels[fmt.Sprintf("topology.kubernetes.io/network-level-%d", i+1)] = l.checkLabel(sw)
			}

			if err := labeler.AddNodeLabels(ctx, v.Name, labels); err != nil {
				return err
			}
		}
		return nil
	}

	for _, w := range v.Vertices {
		if err := l.applyNodeLabels(ctx, w, labeler, append([]string{w.ID}, levels...)); err != nil {
			return err
		}
	}

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
