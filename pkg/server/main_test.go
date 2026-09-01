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
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}
