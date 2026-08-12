/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package logs

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

type rack struct {
	id int
}

func (r *rack) String() string {
	return fmt.Sprintf("Rack-%d", r.id)
}

func TestInfo(t *testing.T) {
	tests := []struct {
		name      string
		verbosity int
		verbose   bool
		level     klog.Level
		args      []any
		want      string
	}{
		{
			name:      "global message",
			verbosity: 0,
			args:      []any{"global message"},
			want:      "global message",
		},
		{
			name:      "requested level above verbosity",
			verbosity: 4,
			verbose:   true,
			level:     5,
			args:      []any{"should not be logged"},
		},
		{
			name:      "verbose message",
			verbosity: 4,
			verbose:   true,
			level:     4,
			args:      []any{"node ", "worker-1", " is on ", &rack{id: 3}},
			want:      "node worker-1 is on Rack-3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := configureKlog(t, test.verbosity)

			if test.verbose {
				V(test.level).Info(test.args...)
			} else {
				Info(test.args...)
			}

			assertLogOutput(t, "Info", output, test.want)
		})
	}
}

func TestInfof(t *testing.T) {
	tests := []struct {
		name      string
		verbosity int
		level     klog.Level
		format    string
		args      []any
		want      string
	}{
		{
			name:      "global formatted message",
			verbosity: 0,
			format:    "global %s",
			args:      []any{"message"},
			want:      "global message",
		},
		{
			name:      "requested level above verbosity",
			verbosity: 4,
			level:     5,
			format:    "should not be logged",
		},
		{
			name:      "verbose formatted message",
			verbosity: 4,
			level:     4,
			format:    "node %s has %d GPUs on rack %s",
			args:      []any{"worker-1", 8, &rack{id: 3}},
			want:      "node worker-1 has 8 GPUs on rack Rack-3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := configureKlog(t, test.verbosity)

			if test.verbosity != 0 {
				V(test.level).Infof(test.format, test.args...)
			} else {
				Infof(test.format, test.args...)
			}

			assertLogOutput(t, "Infof", output, test.want)
		})
	}
}

func TestInfoS(t *testing.T) {
	tests := []struct {
		name          string
		verbosity     int
		level         klog.Level
		msg           string
		keysAndValues []any
		want          string
	}{
		{
			name:          "global structured message",
			verbosity:     0,
			msg:           "global message",
			keysAndValues: []any{"node", "worker-1"},
			want:          `"global message" node="worker-1"`,
		},
		{
			name:      "requested level above verbosity",
			verbosity: 4,
			level:     5,
			msg:       "should not be logged",
		},
		{
			name:          "verbose structured message",
			verbosity:     4,
			level:         4,
			msg:           "node discovered",
			keysAndValues: []any{"node", "worker-1", "rack", &rack{id: 3}},
			want:          `"node discovered" node="worker-1" rack="Rack-3"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := configureKlog(t, test.verbosity)

			if test.verbosity != 0 {
				V(test.level).InfoS(test.msg, test.keysAndValues...)
			} else {
				InfoS(test.msg, test.keysAndValues...)
			}

			assertLogOutput(t, "InfoS", output, test.want)
		})
	}
}

func assertLogOutput(t *testing.T, method string, output *bytes.Buffer, want string) {
	t.Helper()

	if want == "" && output.Len() != 0 {
		t.Fatalf("%s() unexpectedly logged: %q", method, output.String())
	}
	if want != "" && !strings.Contains(output.String(), want) {
		t.Fatalf("%s() output %q does not contain %q", method, output.String(), want)
	}
}

func configureKlog(t *testing.T, verbosity int) *bytes.Buffer {
	t.Helper()

	state := klog.CaptureState()
	t.Cleanup(state.Restore)

	output := &bytes.Buffer{}
	klog.LogToStderr(false)
	klog.SetOutput(output)

	flags := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	klog.InitFlags(flags)
	if err := flags.Set("vmodule", ""); err != nil {
		t.Fatalf("clear klog vmodule: %v", err)
	}
	if err := flags.Set("v", strconv.Itoa(verbosity)); err != nil {
		t.Fatalf("set klog verbosity: %v", err)
	}

	return output
}
