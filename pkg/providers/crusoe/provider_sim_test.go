/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestLoaderSimBadModel(t *testing.T) {
	_, err := LoaderSim(context.Background(), providers.Config{
		Params: map[string]any{"modelFileName": "no-such-model.yaml"},
	})
	require.NotNil(t, err)
	require.ErrorContains(t, err, "failed to load model file")
}

func TestLoaderSimMissingModel(t *testing.T) {
	_, err := LoaderSim(context.Background(), providers.Config{Params: map[string]any{}})
	require.NotNil(t, err)
	require.ErrorContains(t, err, "no model file name for simulation")
}

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	provider, httpErr := LoaderSim(ctx, providers.Config{
		Params: map[string]any{"modelFileName": "crusoe-small.yaml"},
	})
	require.Nil(t, httpErr)

	sim, ok := provider.(*simProvider)
	require.True(t, ok)

	cis, httpErr := sim.GetComputeInstances(ctx)
	require.Nil(t, httpErr)
	require.Len(t, cis, 1)

	graph, httpErr := provider.GenerateTopologyConfig(ctx, nil, cis)
	require.Nil(t, httpErr)

	data, httpErr := slurm.GenerateOutput(ctx, graph, nil)
	require.Nil(t, httpErr)

	expected := `SwitchName=crusoe Switches=0cf47922-9f9e-4a2e-b187-5c51697e7739,1d0e8b3a-2c47-4f16-9a55-7be3d9c4f082,cpu-partition
SwitchName=0cf47922-9f9e-4a2e-b187-5c51697e7739 Switches=4f6a2e91-8d75-4c30-b1e8-6a9c3f5d7b24,9b1c7d54-3e08-4a92-8f61-2d5a4c7b0e13
SwitchName=1d0e8b3a-2c47-4f16-9a55-7be3d9c4f082 Switches=7c3d9f68-5b21-4e84-a0f3-9e1b8d2c6a57
SwitchName=cpu-partition Switches=cpu-pod
SwitchName=4f6a2e91-8d75-4c30-b1e8-6a9c3f5d7b24 Nodes=gpu-[03-04]
SwitchName=9b1c7d54-3e08-4a92-8f61-2d5a4c7b0e13 Nodes=gpu-[01-02]
SwitchName=7c3d9f68-5b21-4e84-a0f3-9e1b8d2c6a57 Nodes=gpu-[05-06]
SwitchName=cpu-pod Nodes=cpu-[01-02]
`
	require.Equal(t, expected, string(data))
}

func TestProviderSimRejectsMultiRegion(t *testing.T) {
	ctx := context.Background()

	provider, httpErr := LoaderSim(ctx, providers.Config{
		Params: map[string]any{"modelFileName": "crusoe-small.yaml"},
	})
	require.Nil(t, httpErr)

	_, httpErr = provider.GenerateTopologyConfig(ctx, nil, []topology.ComputeInstances{
		{Region: "one", Instances: map[string]string{"gpu-01": "gpu-01"}},
		{Region: "two", Instances: map[string]string{"gpu-02": "gpu-02"}},
	})
	require.NotNil(t, httpErr)
	require.ErrorContains(t, httpErr, "does not support multi-region")
}
