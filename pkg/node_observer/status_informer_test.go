/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewStatusInformer(t *testing.T) {
	ctx := context.TODO()
	trigger := &Trigger{
		NodeSelector: map[string]string{"key": "val"},
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"key": "val"},
		},
	}
	informer, err := NewStatusInformer(ctx, nil, trigger, nil)
	require.NoError(t, err)
	require.NotNil(t, informer.nodeFactory)
	require.NotNil(t, informer.podFactory)
}
