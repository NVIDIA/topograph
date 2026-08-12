/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package logs

import "k8s.io/klog/v2"

type Verbose struct {
	verbose klog.Verbose
}

func V(level klog.Level) Verbose {
	return Verbose{verbose: klog.V(level)}
}

func (v Verbose) Enabled() bool {
	return v.verbose.Enabled()
}

func (v Verbose) Info(args ...any) {
	if v.Enabled() {
		v.verbose.Info(args...)
	}
}

func (v Verbose) Infof(format string, args ...any) {
	if v.Enabled() {
		v.verbose.Infof(format, args...)
	}
}

func (v Verbose) InfoS(msg string, keysAndValues ...any) {
	if v.Enabled() {
		v.verbose.InfoS(msg, keysAndValues...)
	}
}

func Info(args ...any) {
	klog.Info(args...)
}

func Infof(format string, args ...any) {
	klog.Infof(format, args...)
}

func InfoS(msg string, keysAndValues ...any) {
	klog.InfoS(msg, keysAndValues...)
}
