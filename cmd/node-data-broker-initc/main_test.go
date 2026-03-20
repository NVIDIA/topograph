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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetExtras(t *testing.T) {
	tests := []struct {
		name   string
		sets   []string
		extras map[string]string
		err    string
	}{
		{
			name:   "Case 1: empty input",
			sets:   []string{},
			extras: map[string]string{},
		},
		{
			name:   "Case 2: single valid key=value",
			sets:   []string{"a=b"},
			extras: map[string]string{"a": "b"},
		},
		{
			name:   "Case 3: multiple valid key=value",
			sets:   []string{"a=b", "c=d"},
			extras: map[string]string{"a": "b", "c": "d"},
		},
		{
			name: "Case 4: invalid format",
			sets: []string{"foo"},
			err:  `invalid value "foo" for '--set': expected format '<key>=<value>'`,
		},
		{
			name: "Case 5: empty key",
			sets: []string{"=bar"},
			err:  `invalid value "=bar" for '--set': key/value cannot be empty`,
		},
		{
			name: "Case 6: empty value",
			sets: []string{"foo="},
			err:  `invalid value "foo=" for '--set': key/value cannot be empty`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extras, err := getExtras(tt.sets)
			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.extras, extras)
			}
		})
	}
}

func TestGetAnnotations(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		name     string
		provider string
		err      string
	}{
		{
			name: "Case 1: empty provider",
			err:  "must set provider",
		},
		{
			name:     "Case 2: invalid provider",
			provider: "invalid",
			err:      `unsupported provider "invalid"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getAnnotations(ctx, nil, nil, tt.provider, "", nil)
			require.EqualError(t, err, tt.err)
		})
	}
}

func TestMergeNodeAnnotations(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
		in   map[string]string
		out  map[string]string
	}{
		{
			name: "Case 1: no labels",
			node: &corev1.Node{},
			out:  map[string]string{},
		},
		{
			name: "Case 2: copy",
			node: &corev1.Node{},
			in:   map[string]string{"a": "1", "b": "2"},
			out:  map[string]string{"a": "1", "b": "2"},
		},
		{
			name: "Case 3: merge",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"a": "1", "b": "2", "c": "x"},
					Annotations: map[string]string{"a": "1", "b": "2", "c": "x"},
				},
			},
			in:  map[string]string{"c": "3", "d": "4"},
			out: map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeNodeAnnotations(tt.node, tt.in)
			require.Equal(t, tt.out, tt.node.Annotations)
		})
	}
}
