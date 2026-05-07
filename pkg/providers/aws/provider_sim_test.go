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

package aws

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/agrea/ptr"
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
    switches: [tor1,tor2]
  tor1:
    metadata:
      group: g1
    nodes: [n11,n12]
  tor2:
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
    switches: [tor1,tor2]
  tor1:
    metadata:
      group: g1
    nodes: ["n[100-199]"]
  tor2:
    metadata:
      group: g2
    nodes: ["n[200-299]"]
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
  n165:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n166:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n167:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n168:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n169:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n170:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n171:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n172:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n173:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n174:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n175:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n176:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n177:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n178:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n179:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n180:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n181:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n182:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n183:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n184:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n185:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n186:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n187:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n188:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n189:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n190:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n191:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n192:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n193:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n194:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n195:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n196:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n197:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n198:
    capacity_block_id: cb1
    attributes:
      nvlink: nvl1
  n199:
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
  n265:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n266:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n267:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n268:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n269:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n270:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n271:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n272:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n273:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n274:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n275:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n276:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n277:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n278:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n279:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n280:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n281:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n282:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n283:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n284:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n285:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n286:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n287:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n288:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n289:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n290:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n291:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n292:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n293:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n294:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n295:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n296:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n297:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n298:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
  n299:
    capacity_block_id: cb2
    attributes:
      nvlink: nvl2
capacity_blocks:
- cb1
- cb2
`
)

func TestProviderSim(t *testing.T) {
	ctx := context.Background()

	type interval struct{ from, to int }

	testCases := []struct {
		name      string
		model     string
		region    string
		intervals []interval
		pageSize  *int
		params    map[string]any
		apiErr    int
		topology  string
		err       string
	}{
		{
			name:  "Case 1: bad model",
			model: `bad: model: error:`,
			err:   ignoreErrMsg,
		},
		{
			name: "Case 2: no ComputeInstances",
			model: `
switches:
  core:
    switches: [spine]
  spine:
    switches: [tor]
  tor:
    nodes: [n11,n12]
nodes:
  n11:
    capacity_block_id: cb
    attributes:
      nvlink: nvl1
  n12:
    capacity_block_id: cb
    attributes:
      nvlink: nvl1
capacity_blocks:
- cb
`,
		},
		{
			name:      "Case 3.1: ClientFactory API error",
			model:     clusterModel,
			region:    "region",
			intervals: []interval{{11, 12}},
			apiErr:    errClientFactory,
			err:       "failed to get client: API error",
		},
		{
			name:      "Case 3.2: DescribeInstanceTopology API error",
			model:     clusterModel,
			region:    "region",
			intervals: []interval{{11, 12}},
			apiErr:    errDescribeInstanceTopology,
			err:       "failed to describe instance topology: API error",
		},
		{
			name:      "Case 4: missing region",
			model:     clusterModel,
			intervals: []interval{{11, 12}},
			err:       `must specify region`,
		},
		{
			name: "Case 5.1: missing availability zone",
			model: `
switches:
  core:
    switches: [spine]
  spine:
    switches: [tor]
  tor:
    nodes: [n11]
nodes:
  n11:
    capacity_block_id: cb
    attributes:
      nvlink: nvl1
capacity_blocks:
- cb
`,
			region:    "region",
			intervals: []interval{{11, 11}},
			err:       `failed to describe instance topology: availability zone not found for instance "n11" in AWS simulation`,
		},
		{
			name: "Case 5.2: missing placement group",
			model: `
switches:
  core:
    switches: [spine]
  spine:
    metadata:
      availability_zone: az1
    switches: [tor]
  tor:
    nodes: [n11]
nodes:
  n11:
    capacity_block_id: cb
    attributes:
      nvlink: nvl1
capacity_blocks:
- cb
`,
			region:    "region",
			intervals: []interval{{11, 11}},
			err:       `failed to describe instance topology: placement group not found for instance "n11" in AWS simulation`,
		},
		{
			name:      "Case 6: valid cluster in tree format",
			model:     clusterModel,
			region:    "region",
			intervals: []interval{{11, 13}, {21, 23}},
			topology: `SwitchName=core Switches=spine
SwitchName=no-topology Nodes=node[13,23]
SwitchName=spine Switches=tor[1-2]
SwitchName=tor1 Nodes=node[11-12]
SwitchName=tor2 Nodes=node[21-22]
`,
		},
		{
			name:      "Case 7: valid cluster in block format",
			model:     clusterModel,
			region:    "region",
			intervals: []interval{{11, 12}, {21, 22}, {31, 32}},
			params:    map[string]any{"plugin": "topology/block"},
			topology: `# block001=nvl1
BlockName=block001 Nodes=node[11-12]
# block002=nvl2
BlockName=block002 Nodes=node[21-22]
BlockSizes=2,4
`,
		},
		{
			name:      "Case 8: valid large cluster in block format",
			model:     largeClusterModel,
			region:    "region",
			intervals: []interval{{101, 164}, {201, 264}},
			pageSize:  ptr.Int(25),
			params:    map[string]any{"plugin": "topology/block"},
			topology: `# block001=nvl1
BlockName=block001 Nodes=node[101-164]
# block002=nvl2
BlockName=block002 Nodes=node[201-264]
BlockSizes=64,128
`,
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

			var instances []topology.ComputeInstances
			if len(tc.intervals) != 0 {
				instances = []topology.ComputeInstances{
					{
						Region:    tc.region,
						Instances: make(map[string]string),
					},
				}
				for _, item := range tc.intervals {
					for i := item.from; i <= item.to; i++ {
						instances[0].Instances[fmt.Sprintf("n%d", i)] = fmt.Sprintf("node%d", i)
					}
				}
			}
			topo, httpErr := provider.GenerateTopologyConfig(ctx, tc.pageSize, instances)
			if len(tc.err) != 0 {
				require.EqualError(t, httpErr, tc.err)
			} else {
				require.Nil(t, httpErr)
				data, httpErr := slurm.GenerateOutput(ctx, topo, tc.params)
				require.Nil(t, httpErr)
				require.Equal(t, tc.topology, string(data))
			}
		})
	}
}
