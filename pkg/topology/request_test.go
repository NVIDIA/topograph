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

package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPayload(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		payload *Request
		print   string
		err     string
	}{
		{
			name:    "Case 1: no input",
			payload: &Request{},
			print: `TopologyRequest:
  Provider:
  Credentials: []
  Parameters: []
  Engine:
  Parameters: []
  Nodes:
`,
		},
		{
			name: "Case 2: bad input",
			input: `{
  "nodes": 5
}
`,
			err: "failed to parse payload: json: cannot unmarshal number into Go struct field Request.nodes of type []topology.ComputeInstances",
		},
		{
			name: "Case 3: valid input",
			input: `
{
  "provider": {
    "name": "aws",
	"creds": {
      "accessKeyId": "id",
      "secretAccessKey": "secret"
    },
	"params": {}
  },
  "engine": {
    "name": "slurm",
	"params": {
	  "plugin": "topology/block",
	  "block_sizes": "30,120",
	  "reconfigure": true
	}
  },
  "nodes": [
    {
      "region": "region1",
      "instances": {
        "instance1": "node1",
        "instance2": "node2",
        "instance3": "node3"
      }
    },
    {
      "region": "region2",
      "instances": {
        "instance4": "node4",
        "instance5": "node5",
        "instance6": "node6"
      }
    }
  ]
}
`,
			payload: &Request{
				Provider: Provider{
					Name: "aws",
					Creds: map[string]string{
						"accessKeyId":     "id",
						"secretAccessKey": "secret",
					},
					Params: map[string]any{},
				},
				Engine: Engine{
					Name: "slurm",
					Params: map[string]any{
						KeyPlugin:     TopologyBlock,
						KeyBlockSizes: "30,120",
						"reconfigure": true,
					},
				},
				Nodes: []ComputeInstances{
					{
						Region: "region1",
						Instances: map[string]string{
							"instance1": "node1",
							"instance2": "node2",
							"instance3": "node3",
						},
					},
					{
						Region: "region2",
						Instances: map[string]string{
							"instance4": "node4",
							"instance5": "node5",
							"instance6": "node6",
						},
					},
				},
			},
			print: `TopologyRequest:
  Provider: aws
  Credentials: [accessKeyId:*** secretAccessKey:***]
  Parameters: []
  Engine: slurm
  Parameters: [block_sizes:30,120 plugin:topology/block reconfigure:true]
  Nodes: region1: [instance1:node1 instance2:node2 instance3:node3] region2: [instance4:node4 instance5:node5 instance6:node6]
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := GetTopologyRequest([]byte(tc.input))
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.payload, payload)
				require.Equal(t, tc.print, payload.String())
			}
		})
	}
}

func TestGetNodeNames(t *testing.T) {
	cis := []ComputeInstances{
		{
			Region:    "loc1",
			Instances: map[string]string{"i1": "n1", "i2": "n2"},
		},
		{
			Region:    "loc2",
			Instances: map[string]string{"i3": "n3", "i4": "n4"},
		},
	}

	nodeList := []string{"n1", "n2", "n3", "n4"}
	nodeMap := map[string]bool{"n1": true, "n2": true, "n3": true, "n4": true}
	require.ElementsMatch(t, nodeList, GetNodeNameList(cis))
	require.Equal(t, nodeMap, GetNodeNameMap(cis))
}
