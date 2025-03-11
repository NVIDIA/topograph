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
	"strings"
	"testing"
	"time"

	"github.com/agrea/ptr"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/config"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/toposim"
)

const (
	simpleSlurmPayload = `
{
  "provider": {
    "name": "%s"
  },
  "engine": {
    "name": "slurm"
  }
}
`
	simpleSlurmConfig = `SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=Node[201-202,205]
SwitchName=S3 Nodes=Node[304-306]
`

	slurmTreePayload = `
{
  "provider": {
    "name": "%s",
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
        "1101": "n-1101",
        "1102": "n-1102",
        "1201": "n-1201",
        "1202": "n-1202",
        "1301": "n-1301",
        "1302": "n-1302",
        "1401": "n-1401",
        "1402": "n-1402",
        "1500": "n-CPU"
      }
    }
  ]
}
`
	slurmTreeConfig = `SwitchName=sw3 Switches=sw[21-22]
SwitchName=sw21 Switches=sw[11-12]
SwitchName=sw22 Switches=sw[13-14]
SwitchName=sw11 Nodes=n-[1101-1102]
SwitchName=sw12 Nodes=n-[1201-1202]
SwitchName=sw13 Nodes=n-[1301-1302]
SwitchName=sw14 Nodes=n-[1401-1402]
SwitchName=no-topology Nodes=n-CPU
`

	slurmBlockPayload = `
{
  "provider": {
    "name": "%s",
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
`
	slurmBlockConfig = `# block001=nvl-1-1
BlockName=block001 Nodes=n-[1101-1108]
# block002=nvl-1-2
BlockName=block002 Nodes=n-[1201-1208]
# block003=nvl-2-1
BlockName=block003 Nodes=n-[2101-2108]
# block004=nvl-2-2
BlockName=block004 Nodes=n-[2201-2208]
# block005=nvl-3-1
BlockName=block005 Nodes=n-[3101-3108]
# block006=nvl-3-2
BlockName=block006 Nodes=n-[3201-3208]
# block007=nvl-4-1
BlockName=block007 Nodes=n-[4101-4108]
# block008=nvl-4-2
BlockName=block008 Nodes=n-[4201-4208]
# block009=nvl-5-1
BlockName=block009 Nodes=n-[5101-5108]
# block010=nvl-5-2
BlockName=block010 Nodes=n-[5201-5208]
# block011=nvl-6-1
BlockName=block011 Nodes=n-[6101-6108]
# block012=nvl-6-2
BlockName=block012 Nodes=n-[6201-6208]
BlockSizes=8,16,32
`
)

func TestServerLocal(t *testing.T) {
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
		provider string
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
			provider: "test",
			payload:  simpleSlurmPayload,
			expected: simpleSlurmConfig,
		},
		{
			name:     "Case 4: mock AWS request for tree topology",
			endpoint: "generate",
			provider: "aws-sim",
			payload:  slurmTreePayload,
			expected: slurmTreeConfig,
		},
		{
			name:     "Case 5: mock AWS request for block topology",
			endpoint: "generate",
			provider: "aws-sim",
			payload:  slurmBlockPayload,
			expected: slurmBlockConfig,
		},
		{
			name:     "Case 4: mock GCP request for tree topology",
			endpoint: "generate",
			provider: "gcp-sim",
			payload:  slurmTreePayload,
			expected: `SwitchName=sw21 Switches=sw[11-12]
SwitchName=sw22 Switches=sw[13-14]
SwitchName=sw11 Nodes=n-[1101-1102]
SwitchName=sw12 Nodes=n-[1201-1202]
SwitchName=sw13 Nodes=n-[1301-1302]
SwitchName=no-topology Nodes=n-CPU
SwitchName=sw14 Nodes=n-[1401-1402]
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			var body []byte
			switch tc.endpoint {
			case "invalid":
				resp, err = http.Get(baseURL + "/invalid")
			case "healthz":
				resp, err = http.Get(baseURL + "/healthz")
			case "generate":
				// send topology request
				payload := fmt.Sprintf(tc.payload, tc.provider)
				resp, err = http.Post(baseURL+"/v1/generate", "application/json", bytes.NewBuffer([]byte(payload)))
				require.NoError(t, err)
				require.Equal(t, http.StatusAccepted, resp.StatusCode)

				body, err = io.ReadAll(resp.Body)
				require.NoError(t, err)
				out := string(body)
				resp.Body.Close()

				// retrieve topology config
				params := url.Values{}
				params.Add("uid", out)
				fullURL := fmt.Sprintf("%s?%s", baseURL+"/v1/topology", params.Encode())

				for i := range 5 {
					time.Sleep(time.Second)
					resp, err = http.Get(fullURL)
					require.NoError(t, err)

					if resp.StatusCode == http.StatusOK {
						break
					}

					resp.Body.Close()
					if i == 4 {
						t.Errorf("timeout")
					}
				}

			default:
				t.Errorf("unsupported endpoint %s", tc.endpoint)
			}

			require.NoError(t, err)
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, stringToLineMap(tc.expected), stringToLineMap(string(body)))
		})
	}
}

func TestServerRemote(t *testing.T) {
	testCases := []struct {
		name     string
		model    string
		payload  string
		expected string
	}{
		{
			name:     "Case 1: send request for tree topology",
			model:    "../../tests/models/medium.yaml",
			payload:  slurmTreePayload,
			expected: slurmTreeConfig,
		},
		{
			name:     "Case 2: send request for block topology",
			model:    "../../tests/models/large.yaml",
			payload:  slurmBlockPayload,
			expected: slurmBlockConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// read model
			model, err := models.NewModelFromFile(tc.model)
			require.NoError(t, err)

			// init gRPC server
			grpcPort, err := getAvailablePort()
			require.NoError(t, err)

			topo := toposim.NewServer(model, grpcPort)

			defer topo.Stop(nil)
			go func() { _ = topo.Start() }()

			// init http server
			httpPort, err := getAvailablePort()
			require.NoError(t, err)

			cfg := &config.Config{
				RequestAggregationDelay: time.Second,
				HTTP: config.Endpoint{
					Port: httpPort,
				},
				FwdSvcURL: ptr.String(fmt.Sprintf("localhost:%d", grpcPort)),
			}

			srv = initHttpServer(context.TODO(), cfg)
			defer srv.Stop(nil)
			go func() { _ = srv.Start() }()

			// let the servers start
			time.Sleep(time.Second)

			// send topology request
			baseURL := fmt.Sprintf("http://localhost:%d", httpPort)
			payload := fmt.Sprintf(tc.payload, "test")
			resp, err := http.Post(baseURL+"/v1/generate", "application/json", bytes.NewBuffer([]byte(payload)))
			require.NoError(t, err)
			require.Equal(t, http.StatusAccepted, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			out := string(body)
			resp.Body.Close()

			// retrieve topology config
			params := url.Values{}
			params.Add("uid", out)
			fullURL := fmt.Sprintf("%s?%s", baseURL+"/v1/topology", params.Encode())

			for range 5 {
				time.Sleep(2 * time.Second)
				resp, err := http.Get(fullURL)
				require.NoError(t, err)
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					body, err = io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Equal(t, stringToLineMap(tc.expected), stringToLineMap(string(body)))
					return
				}
			}
			t.Errorf("timeout")
		})
	}
}

func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func stringToLineMap(str string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, line := range strings.Split(str, "\n") {
		if len(line) != 0 {
			m[line] = struct{}{}
		}
	}

	return m
}
