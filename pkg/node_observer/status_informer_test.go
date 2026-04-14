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
	informer, err := NewStatusInformer(ctx, nil, trigger, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, informer.nodeFactory)
	require.NotNil(t, informer.podFactory)
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
