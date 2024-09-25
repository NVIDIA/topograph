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

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPayload(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		payload *Payload
		print   string
		err     string
	}{
		{
			name:    "Case 1: no input",
			payload: &Payload{},
			print: `Payload:
  Nodes: []
`,
		},
		{
			name: "Case 2: bad input",
			input: `{
  "nodes": 5
}
`,
			err: "failed to parse payload: json: cannot unmarshal number into Go struct field Payload.nodes of type []common.ComputeInstances",
		},
		{
			name: "Case 3: invalid creds",
			input: `
{
  "nodes": [
    {
      "region": "region1",
      "instances": {
        "instance1": "node1",
        "instance2": "node2",
        "instance3": "node3"
      }
    }
  ],
  "creds": {
    "aws": {
      "access_key_id": "id",
      "token": "token"
    }
  }
}
`,
			err: "invalid payload: must provide access_key_id and secret_access_key for AWS",
		},
		{
			name: "Case 4: valid input",
			input: `
{
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
  ],
  "creds": {
    "aws": {
      "access_key_id": "id",
      "secret_access_key": "secret"
    }
  }
}
`,
			payload: &Payload{
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
				Creds: &Credentials{
					AWS: &AWSCredentials{
						AccessKeyId:     "id",
						SecretAccessKey: "secret",
					},
				},
			},
			print: `Payload:
  Nodes: [{region1 map[instance1:node1 instance2:node2 instance3:node3]} {region2 map[instance4:node4 instance5:node5 instance6:node6]}]
  Credentials:
    AWS: AccessKeyID=*** SecretAccessKey=*** SessionToken=
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := GetPayload([]byte(tc.input))
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
