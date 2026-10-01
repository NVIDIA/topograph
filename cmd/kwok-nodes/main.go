/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/dsx-ai-factory/topograph/internal/kwok"
	"github.com/dsx-ai-factory/topograph/internal/version"
	"github.com/dsx-ai-factory/topograph/pkg/models"
)

type options struct {
	modelFile  string
	outputFile string
	version    bool
	capacity   kwok.Capacity
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		if err == pflag.ErrHelp {
			os.Exit(0)
		}
		fmt.Println("Error parsing flags:", err)
		os.Exit(1)
	}

	if opts.version {
		fmt.Println("Version:", version.Version)
		os.Exit(0)
	}

	if err := mainInternal(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() (options, error) {
	opts := options{
		capacity: kwok.DefaultCapacity(),
	}

	fs := pflag.NewFlagSet("kwok-nodes", pflag.ContinueOnError)
	fs.StringVarP(&opts.modelFile, "model", "m", "", "model file to load; basenames resolve from tests/models (ex: small-tree.yaml); external model files can be used by providing the file path (ex: /myPath/model.yaml)")
	fs.StringVarP(&opts.outputFile, "output", "o", "-", "output manifest path; use - for stdout")
	fs.StringVar(&opts.capacity.CPU, "cpu", opts.capacity.CPU, "node CPU capacity")
	fs.StringVar(&opts.capacity.Memory, "memory", opts.capacity.Memory, "node memory capacity")
	fs.StringVarP(&opts.capacity.Pods, "pods", "p", opts.capacity.Pods, "node pod capacity")
	fs.StringVar(&opts.capacity.EphemeralStorage, "ephemeral-storage", opts.capacity.EphemeralStorage, "node ephemeral-storage capacity")
	fs.IntVarP(&opts.capacity.GPUs, "gpus", "g", opts.capacity.GPUs, "GPU capacity per node; 0 omits GPU capacity")
	fs.StringVar(&opts.capacity.GPUResourceName, "gpu-resource-name", opts.capacity.GPUResourceName, "extended resource name for GPU capacity")
	fs.BoolVar(&opts.version, "version", false, "show the version")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return options{}, err
	}

	return opts, nil
}

func mainInternal(opts options) error {
	if opts.modelFile == "" {
		return fmt.Errorf("missing required --model")
	}

	model, err := models.NewModelFromFile(opts.modelFile)
	if err != nil {
		return err
	}

	nodes, err := kwok.NodesFromModel(model, opts.capacity)
	if err != nil {
		return err
	}

	data, err := kwok.MarshalNodeManifest(nodes)
	if err != nil {
		return err
	}

	if opts.outputFile == "" || opts.outputFile == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}

	return os.WriteFile(opts.outputFile, data, 0o644)
}
