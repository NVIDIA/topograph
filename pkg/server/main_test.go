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
	// TestServerIntegration calls http.DefaultClient.CloseIdleConnections() in its
	// t.Cleanup, which signals the transport to close keep-alive connections.
	// However, persistConn goroutines do not exit synchronously — they drain their
	// final select iteration after the close signal. Goleak fires in that async
	// window, so the filters below remain necessary even with CloseIdleConnections().
	// Tracked in https://github.com/NVIDIA/topograph/issues/493.
	goleak.VerifyTestMain(m,
		goleak.IgnoreCurrent(),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)
}
