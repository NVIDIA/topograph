/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// IgnoreCurrent captures goroutines running before m.Run() (klog flushDaemon)
	// so they do not trigger false positives.
	//
	// Integration tests make HTTP requests via http.DefaultTransport but do not
	// call CloseIdleConnections() on teardown, leaving keep-alive connection
	// goroutines in the transport pool. Tracked in
	// https://github.com/NVIDIA/topograph/issues/493; filter until fixed.
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)
}
