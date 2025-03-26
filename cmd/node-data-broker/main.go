/*
 * Copyright (c) 2024-2025, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"flag"
	"os"
	"syscall"

	"github.com/oklog/run"
	"k8s.io/klog/v2"

	server "github.com/NVIDIA/topograph/pkg/node_data_broker"
)

func main() {
	var port int
	flag.IntVar(&port, "p", 8181, "service port")

	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	if err := mainInternal(port); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func mainInternal(port int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := server.NewServer(ctx, port)

	var g run.Group
	// Signal handler
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	// Server endpoint
	g.Add(srv.Start, srv.Stop)

	return g.Run()
}
