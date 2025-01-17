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

package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/NVIDIA/topograph/pkg/config"
	"github.com/stretchr/testify/require"
)

func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func TestServer(t *testing.T) {
	port, err := getAvailablePort()
	require.NoError(t, err)

	cfg := &config.Config{
		HTTP: config.Endpoint{
			Port: port,
		},
		RequestAggregationDelay: time.Second,
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	srv = initHttpServer(context.TODO(), cfg)
	defer srv.Stop(nil)
	go func() { _ = srv.Start() }()

	// let the server start
	time.Sleep(time.Second)

	testCases := []struct {
		name     string
		endpoint string
		payload  string
		expected string
	}{
		{
			name:     "Case 1: test invalid endpoint",
			endpoint: "invalid",
			expected: "404 page not found\n",
		},
		{
			name:     "Case 2: test healthz endpoint",
			endpoint: "healthz",
			expected: "OK\n",
		},
		{
			name:     "Case 3: send test request for tree topology",
			endpoint: "generate",
			payload: `
{
  "provider": {
    "name": "test"
  },
  "engine": {
    "name": "slurm"
  }
}
`,
			expected: `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=Node[201-202],Node205
SwitchName=S3 Nodes=Node[304-306]
`,
		},
		{
			name:     "Case 4: mock AWS request for tree topology",
			endpoint: "generate",
			payload: `
{
  "provider": {
    "name": "aws-sim",
    "params": {
      "model_path": "../../tests/models/medium.yaml"
    }
  },
  "engine": {
    "name": "slurm"
  },
  "nodes": [
    {
      "region": "R1",
      "instances": {
        "n11-1": "n11-1",
        "n11-2": "n11-2",
        "n12-1": "n12-1",
        "n12-2": "n12-2",
        "n13-1": "n13-1",
        "n13-2": "n13-2",
        "n14-1": "n14-1",
        "n14-2": "n14-2"
      }
    }
  ]
}
`,
			expected: `SwitchName=sw3 Switches=sw[21-22]
SwitchName=sw21 Switches=sw[11-12]
SwitchName=sw22 Switches=sw[13-14]
SwitchName=sw11 Nodes=n11-[1-2]
SwitchName=sw12 Nodes=n12-[1-2]
SwitchName=sw13 Nodes=n13-[1-2]
SwitchName=sw14 Nodes=n14-[1-2]
`,
		},
		{
			name:     "Case 5: mock AWS request for block topology",
			endpoint: "generate",
			payload: `
{
  "provider": {
    "name": "aws-sim",
    "params": {
      "model_path": "../../tests/models/large.yaml"
    }
  },
  "engine": {
    "name": "slurm",
	"params": {
      "plugin": "topology/block",
      "block_sizes": "8,16,32"
    }
  }
}
`,
			expected: `# block001=nvl-1-1
BlockName=block001 Nodes=n1-1-0[1-8]
# block002=nvl-1-2
BlockName=block002 Nodes=n1-2-0[1-8]
# block003=nvl-2-1
BlockName=block003 Nodes=n2-1-0[1-8]
# block004=nvl-2-2
BlockName=block004 Nodes=n2-2-0[1-8]
# block005=nvl-3-1
BlockName=block005 Nodes=n3-1-0[1-8]
# block006=nvl-3-2
BlockName=block006 Nodes=n3-2-0[1-8]
# block007=nvl-4-1
BlockName=block007 Nodes=n4-1-0[1-8]
# block008=nvl-4-2
BlockName=block008 Nodes=n4-2-0[1-8]
# block009=nvl-5-1
BlockName=block009 Nodes=n5-1-0[1-8]
# block010=nvl-5-2
BlockName=block010 Nodes=n5-2-0[1-8]
# block011=nvl-6-1
BlockName=block011 Nodes=n6-1-0[1-8]
# block012=nvl-6-2
BlockName=block012 Nodes=n6-2-0[1-8]
BlockSizes=8,16,32
`,
		},
	}

	for _, tc := range testCases {
		var resp *http.Response
		var body []byte
		switch tc.endpoint {
		case "invalid":
			resp, err = http.Get(baseURL + "/invalid")
		case "healthz":
			resp, err = http.Get(baseURL + "/healthz")
		case "generate":
			// send topology request
			resp, err = http.Post(baseURL+"/v1/generate", "application/json", bytes.NewBuffer([]byte(tc.payload)))
			require.NoError(t, err)
			require.Equal(t, http.StatusAccepted, resp.StatusCode)

			body, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			out := string(body)
			fmt.Println("response", out)
			resp.Body.Close()

			// wait for topology config generation
			time.Sleep(3 * time.Second)

			// retrieve topology config
			params := url.Values{}
			params.Add("uid", out)

			fullURL := fmt.Sprintf("%s?%s", baseURL+"/v1/topology", params.Encode())
			resp, err = http.Get(fullURL)

		default:
			t.Errorf("unsupported endpoint %s", tc.endpoint)
		}

		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, tc.expected, string(body))
	}
}
