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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/node_observer"
)

var GitTag string

func main() {
	var c string
	var version bool
	flag.StringVar(&c, "c", "/etc/topograph/node-observer-config.yaml", "config file")
	flag.BoolVar(&version, "version", false, "show the version")

	klog.InitFlags(nil)
	flag.Parse()
	defer klog.Flush()

	if version {
		fmt.Println("Version:", GitTag)
		os.Exit(0)
	}

	if err := mainInternal(c); err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
}

func mainInternal(c string) error {
	cfg, err := node_observer.NewConfigFromFile(c)
	if err != nil {
		return err
	}

	konfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(konfig)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller, err := node_observer.NewController(ctx, kubeClient, cfg)
	if err != nil {
		return err
	}

	var g run.Group
	// Signal handler
	g.Add(run.SignalHandler(ctx, os.Interrupt, syscall.SIGTERM))
	// Controller
	g.Add(controller.Start, controller.Stop)

	return g.Run()
}
