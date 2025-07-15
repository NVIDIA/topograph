/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNodeInformer(t *testing.T) {
	ctx := context.TODO()
	trigger := &Trigger{
		NodeLabels: map[string]string{"key": "val"},
		PodLabels:  map[string]string{"key": "val"},
	}
	informer := NewNodeInformer(ctx, nil, trigger, nil)
	require.NotNil(t, informer.nodeFactory)
	require.NotNil(t, informer.podFactory)

	f := informer.eventHandler("tested")
	require.NotNil(t, f)
}
