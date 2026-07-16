/*
 * Copyright 2025-2026 NVIDIA CORPORATION
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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNewStatusInformer(t *testing.T) {
	ctx := context.TODO()
	trigger := &Trigger{
		NodeSelector: map[string]string{"key": "val"},
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"key": "val"},
		},
		PeriodicInterval: metav1.Duration{Duration: time.Minute},
	}
	apiServer := &APIServer{
		Namespace: "topograph",
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "topograph"},
		},
		ContainerName: "topograph",
	}
	informer, err := NewStatusInformer(ctx, nil, trigger, apiServer, "topograph-node-data-broker", "topograph", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, informer.nodeFactory)
	require.NotNil(t, informer.podFactory)
	require.NotNil(t, informer.apiFactory)
	require.NotNil(t, informer.brokerFactory)
	require.Equal(t, "topograph", informer.apiServerContainerName)
	require.Equal(t, time.Minute, informer.periodicInterval)
}

func TestNewStatusInformerRejectsNegativePeriodicInterval(t *testing.T) {
	trigger := &Trigger{PeriodicInterval: metav1.Duration{Duration: -time.Minute}}
	_, err := NewStatusInformer(context.TODO(), nil, trigger, nil, "", "", 0, nil)
	require.EqualError(t, err, "trigger.periodicInterval must not be negative")
}

func TestNewStatusInformerBrokerGateRequiresNameAndNamespace(t *testing.T) {
	testCases := []struct {
		name            string
		brokerName      string
		brokerNamespace string
	}{
		{name: "neither is set"},
		{name: "name only", brokerName: "topograph-node-data-broker"},
		{name: "namespace only", brokerNamespace: "topograph"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			informer, err := NewStatusInformer(context.TODO(), nil, nil, nil, tc.brokerName, tc.brokerNamespace, 0, nil)
			require.NoError(t, err)
			require.Nil(t, informer.brokerFactory)
		})
	}
}

func TestBrokerDaemonSetReady(t *testing.T) {
	testCases := []struct {
		name    string
		desired int32
		ready   int32
		want    bool
	}{
		{name: "all desired replicas ready", desired: 3, ready: 3, want: true},
		{name: "replicas still becoming ready", desired: 3, ready: 2, want: false},
		{name: "no replicas desired", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			daemonSet := &appsv1.DaemonSet{Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: tc.desired,
				NumberReady:            tc.ready,
			}}
			require.Equal(t, tc.want, isBrokerDaemonSetReady(daemonSet))
		})
	}
}

func TestBrokerDaemonSetReadyRejectsZeroDesiredReplicas(t *testing.T) {
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      "topograph-node-data-broker",
		Namespace: "topograph",
	}}

	ready, err := brokerDaemonSetReady(daemonSet)
	require.False(t, ready)
	require.EqualError(t, err, "node-data-broker DaemonSet topograph/topograph-node-data-broker has 0 desired replicas; check its node selector, affinity, and tolerations")
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

func TestStatusInformerStartBlocksUntilStop(t *testing.T) {
	s, err := NewStatusInformer(context.Background(), nil, nil, nil, "", "", 0, nil)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()

	select {
	case err := <-done:
		require.Failf(t, "Start returned before Stop", "unexpected error: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	s.Stop(nil)
	require.NoError(t, <-done)
}

func TestPeriodicTriggerDisabledWhenMissingOrZero(t *testing.T) {
	testCases := []struct {
		name    string
		trigger *Trigger
	}{
		{name: "missing"},
		{name: "zero", trigger: &Trigger{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			called := make(chan struct{}, 1)
			reqFunc := func() (*http.Request, *httperr.Error) {
				called <- struct{}{}
				return nil, nil
			}
			s, err := NewStatusInformer(ctx, nil, tc.trigger, nil, "", "", 0, reqFunc)
			require.NoError(t, err)
			s.reqExecFunc = reqExecFunc
			done := runStatusInformer(s)

			select {
			case <-called:
				require.Fail(t, "periodic trigger fired while disabled")
			case <-time.After(40 * time.Millisecond):
			}

			s.Stop(nil)
			require.NoError(t, <-done)
		})
	}
}

func TestPeriodicTriggerWaitsForFullInterval(t *testing.T) {
	const interval = 80 * time.Millisecond
	called := make(chan struct{}, 1)
	reqFunc := func() (*http.Request, *httperr.Error) {
		called <- struct{}{}
		return nil, nil
	}
	s := newPeriodicStatusInformer(t, interval, reqFunc)
	done := runStatusInformer(s)

	select {
	case <-called:
		require.Fail(t, "periodic trigger fired before a full interval elapsed")
	case <-time.After(interval / 3):
	}

	select {
	case <-called:
	case <-time.After(5 * interval):
		require.Fail(t, "periodic trigger did not fire")
	}

	s.Stop(nil)
	require.NoError(t, <-done)
}

func TestPeriodicTriggerStopsWithInformer(t *testing.T) {
	const interval = 20 * time.Millisecond
	var calls int32
	called := make(chan struct{}, 10)
	reqFunc := func() (*http.Request, *httperr.Error) {
		atomic.AddInt32(&calls, 1)
		called <- struct{}{}
		return nil, nil
	}
	s := newPeriodicStatusInformer(t, interval, reqFunc)
	done := runStatusInformer(s)

	select {
	case <-called:
	case <-time.After(5 * interval):
		require.Fail(t, "periodic trigger did not fire")
	}

	s.Stop(nil)
	require.NoError(t, <-done)
	callsAfterStop := atomic.LoadInt32(&calls)
	time.Sleep(3 * interval)
	require.Equal(t, callsAfterStop, atomic.LoadInt32(&calls))
}

func TestStopDropsAQueuedRequest(t *testing.T) {
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})
	reqFunc := func() (*http.Request, *httperr.Error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
			<-release
		}
		return nil, nil
	}
	s, err := NewStatusInformer(context.Background(), nil, nil, nil, "", "", 0, reqFunc)
	require.NoError(t, err)
	s.reqExecFunc = reqExecFunc
	done := runStatusInformer(s)

	s.sendRequest()
	<-started
	s.sendRequest()

	stopped := make(chan struct{})
	go func() {
		s.Stop(nil)
		close(stopped)
	}()
	<-s.ctx.Done()
	close(release)

	select {
	case <-stopped:
	case <-time.After(time.Second):
		require.Fail(t, "Stop did not wait for the active request")
	}
	require.NoError(t, <-done)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestPeriodicTriggerStopsWithContext(t *testing.T) {
	const interval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	called := make(chan struct{}, 10)
	reqFunc := func() (*http.Request, *httperr.Error) {
		atomic.AddInt32(&calls, 1)
		called <- struct{}{}
		return nil, nil
	}
	trigger := &Trigger{PeriodicInterval: metav1.Duration{Duration: interval}}
	s, err := NewStatusInformer(ctx, nil, trigger, nil, "", "", 0, reqFunc)
	require.NoError(t, err)
	s.reqExecFunc = reqExecFunc
	done := runStatusInformer(s)

	select {
	case <-called:
	case <-time.After(5 * interval):
		require.Fail(t, "periodic trigger did not fire")
	}

	cancel()
	require.NoError(t, <-done)
	callsAfterCancel := atomic.LoadInt32(&calls)
	time.Sleep(3 * interval)
	require.Equal(t, callsAfterCancel, atomic.LoadInt32(&calls))
	s.Stop(nil)
}

func TestPeriodicTriggerContextCancelsHTTPRetry(t *testing.T) {
	const interval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	called := make(chan struct{}, 1)
	reqFunc := func() (*http.Request, *httperr.Error) {
		atomic.AddInt32(&calls, 1)
		called <- struct{}{}
		return nil, httperr.NewError(http.StatusServiceUnavailable, "retry later")
	}
	trigger := &Trigger{PeriodicInterval: metav1.Duration{Duration: interval}}
	s, err := NewStatusInformer(ctx, nil, trigger, nil, "", "", 0, reqFunc)
	require.NoError(t, err)
	done := runStatusInformer(s)

	select {
	case <-called:
	case <-time.After(5 * interval):
		require.Fail(t, "periodic trigger did not fire")
	}
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "HTTP retry did not stop after context cancellation")
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
	s.Stop(nil)
}

func TestPeriodicTriggerUsesBrokerReadyGate(t *testing.T) {
	const interval = 20 * time.Millisecond
	const brokerName = "topograph-node-data-broker"
	const brokerNamespace = "topograph"

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: brokerName, Namespace: brokerNamespace},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
		},
	}
	client := k8sfake.NewSimpleClientset(daemonSet)
	called := make(chan struct{}, 1)
	reqFunc := func() (*http.Request, *httperr.Error) {
		called <- struct{}{}
		return nil, nil
	}
	trigger := &Trigger{PeriodicInterval: metav1.Duration{Duration: interval}}
	s, err := NewStatusInformer(context.Background(), client, trigger, nil, brokerName, brokerNamespace, 0, reqFunc)
	require.NoError(t, err)
	s.reqExecFunc = reqExecFunc
	require.NoError(t, s.startBrokerInformer())
	select {
	case <-s.queue:
	default:
	}
	done := runStatusInformer(s)

	select {
	case <-called:
		require.Fail(t, "periodic trigger bypassed the broker readiness gate")
	case <-time.After(3 * interval):
	}

	readyDaemonSet := daemonSet.DeepCopy()
	readyDaemonSet.Status.NumberReady = 1
	brokerInformer := s.brokerFactory.Apps().V1().DaemonSets().Informer()
	require.NoError(t, brokerInformer.GetStore().Update(readyDaemonSet))

	select {
	case <-called:
	case <-time.After(5 * interval):
		require.Fail(t, "periodic trigger did not proceed after the broker became ready")
	}

	s.Stop(nil)
	require.NoError(t, <-done)
}

func TestPeriodicTriggerUsesRequestQueue(t *testing.T) {
	const interval = 10 * time.Millisecond
	called := make(chan struct{}, 1)
	reqFunc := func() (*http.Request, *httperr.Error) {
		called <- struct{}{}
		return nil, nil
	}
	s := newPeriodicStatusInformer(t, interval, reqFunc)
	// A nil queue disables the worker's queue case and makes sendRequest drop
	// the event, proving that a ticker event cannot call process directly.
	s.queue = nil
	done := runStatusInformer(s)

	select {
	case <-called:
		require.Fail(t, "periodic trigger bypassed the request queue")
	case <-time.After(3 * interval):
	}

	s.Stop(nil)
	require.NoError(t, <-done)
}

func TestPeriodicTriggerDoesNotRunConcurrentRequests(t *testing.T) {
	const interval = 5 * time.Millisecond
	var active int32
	var maxActive int32
	completed := make(chan struct{}, 10)
	reqFunc := func() (*http.Request, *httperr.Error) {
		current := atomic.AddInt32(&active, 1)
		for {
			maximum := atomic.LoadInt32(&maxActive)
			if current <= maximum || atomic.CompareAndSwapInt32(&maxActive, maximum, current) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		completed <- struct{}{}
		return nil, nil
	}
	s := newPeriodicStatusInformer(t, interval, reqFunc)
	done := runStatusInformer(s)

	for range 3 {
		select {
		case <-completed:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for periodic request")
		}
	}

	s.Stop(nil)
	require.NoError(t, <-done)
	require.Equal(t, int32(1), atomic.LoadInt32(&maxActive))
}

func TestPeriodicTickerIsIndependentOfRetryTimer(t *testing.T) {
	const interval = 20 * time.Millisecond
	var calls int32
	called := make(chan struct{}, 2)
	reqFunc := func() (*http.Request, *httperr.Error) {
		call := atomic.AddInt32(&calls, 1)
		called <- struct{}{}
		if call == 1 {
			return nil, httperr.NewError(http.StatusBadRequest, "retry later")
		}
		return nil, nil
	}
	s := newPeriodicStatusInformer(t, interval, reqFunc)
	s.retryDelay = time.Hour
	done := runStatusInformer(s)

	for range 2 {
		select {
		case <-called:
		case <-time.After(5 * interval):
			require.Fail(t, "periodic ticker was blocked by the retry timer")
		}
	}

	s.Stop(nil)
	require.NoError(t, <-done)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func newPeriodicStatusInformer(t *testing.T, interval time.Duration, reqFunc httpreq.RequestFunc) *StatusInformer {
	t.Helper()
	trigger := &Trigger{PeriodicInterval: metav1.Duration{Duration: interval}}
	s, err := NewStatusInformer(context.Background(), nil, trigger, nil, "", "", 0, reqFunc)
	require.NoError(t, err)
	s.reqExecFunc = reqExecFunc
	return s
}

func runStatusInformer(s *StatusInformer) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- s.run()
	}()
	return done
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
