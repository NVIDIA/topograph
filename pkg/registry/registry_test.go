/*
 * Copyright 2025-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package registry

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviders(t *testing.T) {
	known := []string{
		"aws", "aws-sim",
		"infiniband-bm", "infiniband-k8s",
		"dra",
		"gcp", "gcp-sim",
		"oci", "oci-imds", "oci-sim",
		"nebius", "nebius-sim",
		"netq",
		"lambdai", "lambdai-sim",
		"dsx-sim",
		"nscale", "nscale-sim",
		"test",
	}

	for _, name := range known {
		t.Run(name, func(t *testing.T) {
			loader, err := Providers.Get(name)
			require.Nil(t, err, "expected %q to be registered", name)
			require.NotNil(t, loader)
		})
	}

	t.Run("unknown provider returns 400", func(t *testing.T) {
		_, err := Providers.Get("nonexistent")
		require.NotNil(t, err)
		require.Equal(t, http.StatusBadRequest, err.Code())
	})
}

func TestEngines(t *testing.T) {
	known := []string{"k8s", "nfd", "graph", "slurm", "slinky"}

	for _, name := range known {
		t.Run(name, func(t *testing.T) {
			loader, err := Engines.Get(name)
			require.Nil(t, err, "expected %q to be registered", name)
			require.NotNil(t, loader)
		})
	}

	t.Run("unknown engine returns 400", func(t *testing.T) {
		_, err := Engines.Get("nonexistent")
		require.NotNil(t, err)
		require.Equal(t, http.StatusBadRequest, err.Code())
	})
}
