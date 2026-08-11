/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/accelerator"
	"github.com/NVIDIA/topograph/pkg/providers"
)

func TestLoaderBMAcceleratorSource(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		err    string
	}{
		{name: "no accelerator discovery by default"},
		{
			name:   "empty accelerator section disables discovery",
			params: map[string]any{"accelerator": map[string]any{}},
		},
		{
			name:   "null accelerator section",
			params: map[string]any{"accelerator": nil},
			err:    "accelerator section must be an object with a source",
		},
		{
			name: "non-empty accelerator section requires source",
			params: map[string]any{"accelerator": map[string]any{
				"nvidiaSmi": map[string]any{"gpuOperatorNamespace": "gpu-operator"},
			}},
			err: "accelerator source must be set",
		},
		{
			name: "nvidia-smi",
			params: map[string]any{"accelerator": map[string]any{
				"source": accelerator.SourceNvidiaSMI,
			}},
		},
		{
			name: "none",
			params: map[string]any{"accelerator": map[string]any{
				"source": accelerator.SourceNone,
			}},
		},
		{
			name: "Kubernetes label is unsupported",
			params: map[string]any{"accelerator": map[string]any{
				"source": accelerator.SourceKubernetesLabel,
				"kubernetesLabel": map[string]any{
					"key": "example.com/domain",
				},
			}},
			err: `accelerator source "kubernetes-label" is not supported by command discovery`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, httpErr := LoaderBM(context.Background(), providers.Config{Params: test.params})
			if test.err != "" {
				require.EqualError(t, httpErr, test.err)
				return
			}
			require.Nil(t, httpErr)
			require.IsType(t, &ProviderBM{}, provider)
		})
	}
}
