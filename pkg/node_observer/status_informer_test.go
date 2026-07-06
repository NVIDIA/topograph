/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
	apiServer := &APIServer{
		Namespace: "topograph",
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "topograph"},
		},
		ContainerName: "topograph",
	}
	nodeDataBroker := &NodeDataBroker{
		Namespace: "topograph",
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app.kubernetes.io/name": "node-data-broker"},
		},
		ContainerName: "node-data-broker",
	}
	informer, err := NewStatusInformer(ctx, nil, trigger, apiServer, nodeDataBroker, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, informer.nodeFactory)
	require.NotNil(t, informer.podFactory)
	require.NotNil(t, informer.apiFactory)
	require.NotNil(t, informer.brokerFactory)
	require.Equal(t, "topograph", informer.apiServerContainerName)
	require.Equal(t, "node-data-broker", informer.brokerContainerName)
}

func TestAPIServerPodUpdateTriggersOnReadyTransitionAndRestart(t *testing.T) {
	testCases := []struct {
		name      string
		oldPod    *corev1.Pod
		newPod    *corev1.Pod
		triggered bool
	}{
		{
			name:      "not ready after update",
			oldPod:    makeWorkloadPod(false, makeContainerStatus("topograph", false, 0)),
			newPod:    makeWorkloadPod(false, makeContainerStatus("topograph", false, 0)),
			triggered: false,
		},
		{
			name:      "becomes ready",
			oldPod:    makeWorkloadPod(false, makeContainerStatus("topograph", false, 0)),
			newPod:    makeWorkloadPod(true, makeContainerStatus("topograph", true, 0)),
			triggered: true,
		},
		{
			name:      "target container restart count increases while ready",
			oldPod:    makeWorkloadPod(true, makeContainerStatus("topograph", true, 1)),
			newPod:    makeWorkloadPod(true, makeContainerStatus("topograph", true, 2)),
			triggered: true,
		},
		{
			name:      "ready update without restart",
			oldPod:    makeWorkloadPod(true, makeContainerStatus("topograph", true, 1)),
			newPod:    makeWorkloadPod(true, makeContainerStatus("topograph", true, 1)),
			triggered: false,
		},
		{
			name: "sidecar restart does not trigger",
			oldPod: makeWorkloadPod(true,
				makeContainerStatus("topograph", true, 1),
				makeContainerStatus("sidecar", true, 1),
			),
			newPod: makeWorkloadPod(true,
				makeContainerStatus("topograph", true, 1),
				makeContainerStatus("sidecar", true, 2),
			),
			triggered: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.triggered, shouldRequestOnAPIServerUpdate(tc.oldPod, tc.newPod, "topograph"))
		})
	}
}

func TestAPIServerPodReadinessRequiresTargetContainer(t *testing.T) {
	require.True(t, isAPIServerPodReady(
		makeWorkloadPod(true, makeContainerStatus("topograph", true, 0)),
		"topograph",
	))
	require.False(t, isAPIServerPodReady(
		makeWorkloadPod(true, makeContainerStatus("topograph", false, 0)),
		"topograph",
	))
	require.False(t, isAPIServerPodReady(
		makeWorkloadPod(true, makeContainerStatus("sidecar", true, 0)),
		"topograph",
	))
}

func reqExecFunc(f httpreq.RequestFunc, _ bool) ([]byte, *httperr.Error) {
	if _, err := f(); err != nil {
		return nil, err
	}
	return nil, nil
}

func TestSendRequestAndRetry(t *testing.T) {
	var calls int32

	// first two calls fails, third succeeds
	reqFunc := func() (*http.Request, *httperr.Error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1, 2:
			return nil, httperr.NewError(http.StatusInternalServerError, "")
		default:
			return nil, nil
		}
	}

	s := &StatusInformer{
		queue:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		retryDelay:  50 * time.Millisecond,
		reqFunc:     reqFunc,
		reqExecFunc: reqExecFunc,
	}

	// start worker
	go s.run()
	defer close(s.stopCh)

	// trigger request
	s.sendRequest()

	// wait enough time for: fail + delay + fail + delay + success
	time.Sleep(200 * time.Millisecond)

	require.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestDeduplicatesRequests(t *testing.T) {
	var calls int32

	reqFunc := func() (*http.Request, *httperr.Error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	}

	s := &StatusInformer{
		queue:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		retryDelay:  50 * time.Millisecond,
		reqFunc:     reqFunc,
		reqExecFunc: reqExecFunc,
	}

	go s.run()
	defer close(s.stopCh)

	// flood with requests
	for range 5 {
		s.sendRequest()
	}

	time.Sleep(50 * time.Millisecond)

	require.Equal(t, int32(1), atomic.LoadInt32(&calls))

}

func TestRetryCancelledByNewRequest(t *testing.T) {
	var calls int32

	// always fail
	reqFunc := func() (*http.Request, *httperr.Error) {
		atomic.AddInt32(&calls, 1)
		return nil, httperr.NewError(http.StatusInternalServerError, "")
	}

	s := &StatusInformer{
		queue:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		retryDelay:  200 * time.Millisecond,
		reqFunc:     reqFunc,
		reqExecFunc: reqExecFunc,
	}

	go s.run()
	defer close(s.stopCh)

	s.sendRequest()

	time.Sleep(50 * time.Millisecond)

	// send another request before retry fires
	s.sendRequest()

	time.Sleep(100 * time.Millisecond)

	// expected 2 calls:
	// - initial
	// - immediate second (not waiting full retryDelay)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func makeWorkloadPod(ready bool, statuses ...corev1.ContainerStatus) *corev1.Pod {
	conditionStatus := corev1.ConditionFalse
	if ready {
		conditionStatus = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "topograph-abc",
			Namespace: "topograph",
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: conditionStatus,
				},
			},
			ContainerStatuses: statuses,
		},
	}
}

func makeContainerStatus(name string, ready bool, restarts int32) corev1.ContainerStatus {
	status := corev1.ContainerStatus{
		Name:         name,
		Ready:        ready,
		RestartCount: restarts,
	}
	if ready {
		status.State.Running = &corev1.ContainerStateRunning{}
	}
	return status
}
