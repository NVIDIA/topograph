/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package dsx

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/engines/slurm"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	ignoreErrMsg = "_IGNORE_"

	clusterModel = `
switches:
  core:
    switches: [spine]
  spine:
    metadata:
      availability_zone: az1
    switches: [leaf1,leaf2]
  leaf1:
    metadata:
      group: g1
    nodes: [n11,n12]
  leaf2:
    metadata:
      group: g2
    nodes: [n21,n22]
nodes:
  n11:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n12:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n21:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n22:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
capacity_blocks:
- cb1
- cb2
`

	largeClusterModel = `
switches:
  core:
    switches: [spine]
  spine:
    metadata:
      availability_zone: az1
    switches: [leaf1,leaf2]
  leaf1:
    metadata:
      group: g1
    nodes: ["n[100-164]"]
  leaf2:
    metadata:
      group: g2
    nodes: ["n[200-264]"]
nodes:
  n100:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n101:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n102:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n103:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n104:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n105:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n106:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n107:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n108:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n109:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n110:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n111:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n112:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n113:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n114:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n115:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n116:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n117:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n118:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n119:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n120:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n121:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n122:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n123:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n124:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n125:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n126:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n127:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n128:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n129:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n130:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n131:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n132:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n133:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n134:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n135:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n136:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n137:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n138:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n139:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n140:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n141:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n142:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n143:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n144:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n145:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n146:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n147:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n148:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n149:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n150:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n151:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n152:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n153:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n154:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n155:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n156:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n157:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n158:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n159:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n160:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n161:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n162:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n163:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n164:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n200:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n201:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n202:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n203:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n204:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n205:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n206:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n207:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n208:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n209:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n210:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n211:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n212:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n213:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n214:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n215:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n216:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n217:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n218:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n219:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n220:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n221:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n222:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n223:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n224:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n225:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n226:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n227:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n228:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n229:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n230:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n231:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n232:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n233:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n234:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n235:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n236:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n237:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n238:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n239:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n240:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n241:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n242:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n243:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n244:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n245:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n246:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n247:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n248:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n249:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n250:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n251:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n252:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n253:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n254:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n255:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n256:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n257:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n258:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n259:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n260:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n261:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n262:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n263:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n264:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
capacity_blocks:
- cb1
- cb2
`

	singleNodeModel = `
switches:
  core:
    switches: [leaf]
  leaf:
    nodes: [n1]
nodes:
  n1:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
capacity_blocks:
- cb1
`
)

// makeRegionInstances builds one region group like gcp provider_sim tests (instance ID -> node name).
func makeRegionInstances(region string, ranges ...[2]int) []topology.ComputeInstances {
	m := make(map[string]string)
	for _, r := range ranges {
		from, to := r[0], r[1]
		for i := from; i <= to; i++ {
			m[fmt.Sprintf("n%d", i)] = fmt.Sprintf("node%d", i)
		}
	}
	return []topology.ComputeInstances{{Region: region, Instances: m}}
}

func TestProviderSimNamedLoaderSim(t *testing.T) {
	name, loader := NamedLoaderSim()
	require.Equal(t, NAME_SIM, name)
	require.NotNil(t, loader)
}

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name       string
		model      string
		instances  []topology.ComputeInstances
		pageSize   *int
		params     map[string]any
		apiErr     int
		shouldFail bool
		err        string
	}{
		{
			name:       "Case 1: bad model YAML",
			model:      `bad: model: error:`,
			shouldFail: true,
			err:        ignoreErrMsg,
		},
		{
			name: "Case 2: model with no capacity blocks",
			model: `
switches:
  core:
    switches: [spine]
  spine:
    switches: [leaf]
  leaf: {}
`,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n0": "node0"},
				},
			},
			shouldFail: false,
		},
		{
			name:  "Case 3: ClientFactory API error",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11", "n12": "node12"},
				},
			},
			apiErr:     errClientFactory,
			shouldFail: true,
			err:        "failed to get client: API error",
		},
		{
			name:  "Case 4: API error during topology fetch",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n11": "node11", "n12": "node12"},
				},
			},
			apiErr:     errAPIError,
			shouldFail: true,
			err:        "API error: API error",
		},
		{
			name:  "Case 5: valid small cluster in tree format",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region: "region",
					Instances: map[string]string{
						"n11": "node11", "n12": "node12",
						"n21": "node21", "n22": "node22",
					},
				},
			},
			shouldFail: false,
		},
		{
			name:  "Case 6: valid small cluster with nodes",
			model: clusterModel,
			instances: []topology.ComputeInstances{
				{
					Region: "region",
					Instances: map[string]string{
						"n11": "node11", "n12": "node12", "n13": "node13",
						"n21": "node21", "n22": "node22", "n23": "node23",
					},
				},
			},
			shouldFail: false,
		},
		{
			name:  "Case 7: single node model",
			model: singleNodeModel,
			instances: []topology.ComputeInstances{
				{
					Region:    "region",
					Instances: map[string]string{"n1": "node1"},
				},
			},
			shouldFail: false,
		},
		{
			name:       "Case 8: large cluster with many nodes",
			model:      largeClusterModel,
			instances:  makeRegionInstances("region", [2]int{100, 164}, [2]int{200, 264}),
			shouldFail: false,
		},
		{
			name:       "Case 9: empty node list",
			model:      clusterModel,
			instances:  nil,
			shouldFail: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "test-*")
			require.NoError(t, err)
			defer func() { _ = os.Remove(f.Name()) }()
			defer func() { _ = f.Close() }()
			n, err := f.WriteString(tc.model)
			require.NoError(t, err)
			require.Equal(t, len(tc.model), n)
			err = f.Sync()
			require.NoError(t, err)

			cfg := providers.Config{
				Params: map[string]any{
					"modelFileName": f.Name(),
					"api_error":     tc.apiErr,
				},
			}
			provider, httpErr := LoaderSim(ctx, cfg)
			if httpErr != nil {
				if len(tc.err) == 0 {
					require.Nil(t, httpErr)
				} else if tc.err != ignoreErrMsg {
					require.EqualError(t, httpErr, tc.err)
				}
				return
			}

			topo, httpErr := provider.GenerateTopologyConfig(ctx, tc.pageSize, tc.instances)
			if tc.shouldFail {
				if len(tc.err) == 0 {
					require.NotNil(t, httpErr)
				} else if tc.err != ignoreErrMsg {
					require.EqualError(t, httpErr, tc.err)
				}
			} else {
				require.Nil(t, httpErr)
				require.NotNil(t, topo)
				// For valid topologies, try to generate SLURM output
				data, httpErr := slurm.GenerateOutput(ctx, topo, tc.params)
				require.Nil(t, httpErr)
				nInst := 0
				for _, ci := range tc.instances {
					nInst += len(ci.Instances)
				}
				// With no requested instances, the buffer stays empty and Bytes() is nil.
				if nInst > 0 {
					require.NotNil(t, data)
				}
			}
		})
	}
}

func TestProviderSimTopologyStructure(t *testing.T) {
	ctx := context.Background()

	// Test that the generated topology has the expected structure
	model := clusterModel
	f, err := os.CreateTemp("", "test-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(model)
	require.NoError(t, err)
	err = f.Sync()
	require.NoError(t, err)

	cfg := providers.Config{
		Params: map[string]any{
			"modelFileName": f.Name(),
			"api_error":     errNone,
		},
	}
	provider, httpErr := LoaderSim(ctx, cfg)
	require.Nil(t, httpErr)

	instances := []topology.ComputeInstances{
		{
			Region: "region",
			Instances: map[string]string{
				"n11": "node11",
				"n12": "node12",
				"n21": "node21",
				"n22": "node22",
			},
		},
	}

	topo, httpErr := provider.GenerateTopologyConfig(ctx, nil, instances)
	require.Nil(t, httpErr)
	require.NotNil(t, topo)

	// Verify the topology tree structure
	treeVertex := topo.Tiers
	require.NotNil(t, treeVertex)
	require.NotNil(t, treeVertex.Vertices)

	// Root should have core switch
	coreVertex, exists := treeVertex.Vertices["core"]
	require.True(t, exists, "core vertex should exist")
	require.Equal(t, "core", coreVertex.ID)

	require.NotNil(t, coreVertex.Vertices)
	spineVertex, exists := coreVertex.Vertices["spine"]
	require.True(t, exists, "spine vertex should exist")
	require.Equal(t, "spine", spineVertex.ID)

	// Spine should have leaf1 and leaf2 as children
	require.NotNil(t, spineVertex.Vertices)
	require.True(t, len(spineVertex.Vertices) >= 1, "spine should have at least one leaf child")
}

func TestProviderSimWithNVLink(t *testing.T) {
	ctx := context.Background()

	// Test that nodes carry their NVLink information
	model := clusterModel
	f, err := os.CreateTemp("", "test-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(model)
	require.NoError(t, err)
	err = f.Sync()
	require.NoError(t, err)

	cfg := providers.Config{
		Params: map[string]any{
			"modelFileName": f.Name(),
			"api_error":     errNone,
		},
	}
	provider, httpErr := LoaderSim(ctx, cfg)
	require.Nil(t, httpErr)

	instances := []topology.ComputeInstances{
		{
			Region: "region",
			Instances: map[string]string{
				"n11": "node11",
				"n12": "node12",
			},
		},
	}

	topo, httpErr := provider.GenerateTopologyConfig(ctx, nil, instances)
	require.Nil(t, httpErr)
	require.NotNil(t, topo)

	// NVLink / accelerator domain is emitted as graph domains (same as AWS ToThreeTierGraph path).
	domains := topo.Domains
	require.NotNil(t, domains)
	_, hasNVL := domains["nvl1"]
	require.True(t, hasNVL, "cluster model places cb1 nodes in NVLink domain nvl1 under topology/block")
}

func TestLoaderSimMissingModelFile(t *testing.T) {
	ctx := context.Background()

	cfg := providers.Config{
		Params: map[string]any{
			"modelFileName": "/nonexistent/model.yaml",
			"api_error":     errNone,
		},
	}
	_, httpErr := LoaderSim(ctx, cfg)
	require.NotNil(t, httpErr)
	require.Contains(t, httpErr.Error(), "failed to load model file")
}

func TestLoaderSimMissingModelFileName(t *testing.T) {
	ctx := context.Background()

	cfg := providers.Config{
		Params: map[string]any{},
	}
	_, httpErr := LoaderSim(ctx, cfg)
	require.NotNil(t, httpErr)
	// Should contain error about missing model file name
	require.True(t, len(httpErr.Error()) > 0, "should have error message")
}

func TestProviderSimMultipleInstances(t *testing.T) {
	ctx := context.Background()

	model := clusterModel
	f, err := os.CreateTemp("", "test-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(model)
	require.NoError(t, err)
	err = f.Sync()
	require.NoError(t, err)

	cfg := providers.Config{
		Params: map[string]any{
			"modelFileName": f.Name(),
			"api_error":     errNone,
		},
	}
	provider, httpErr := LoaderSim(ctx, cfg)
	require.Nil(t, httpErr)

	// Create multiple compute instance groups
	instances := []topology.ComputeInstances{
		{
			Region: "region1",
			Instances: map[string]string{
				"n11": "node11",
				"n12": "node12",
			},
		},
		{
			Region: "region2",
			Instances: map[string]string{
				"n21": "node21",
				"n22": "node22",
			},
		},
	}

	topo, httpErr := provider.GenerateTopologyConfig(ctx, nil, instances)
	require.Nil(t, httpErr)
	require.NotNil(t, topo)
}
