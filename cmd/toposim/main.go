/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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
	"fmt"
	"os"
	"syscall"

	"github.com/oklog/run"
	"k8s.io/klog/v2"

	mod "github.com/NVIDIA/topograph/pkg/model"
	"github.com/NVIDIA/topograph/pkg/toposim"
)

func main() {
	if err := mainInternal(); err != nil {
		klog.Errorf(err.Error())
		os.Exit(1)
	}
}

func mainInternal() error {
	var path string
	var port int
	flag.StringVar(&path, "m", "", "topology model file")
	flag.IntVar(&port, "p", 49025, "gRPC listening port")

	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	if len(path) == 0 || port == 0 {
		return fmt.Errorf("must specify topology model path and listening port")
	}

	model, err := mod.NewModelFromFile(path)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := toposim.NewServer(model, port)

	var g run.Group
	// Signal handler
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	// gRPC endpoint
	g.Add(server.Start, server.Stop)

	return g.Run()
}
