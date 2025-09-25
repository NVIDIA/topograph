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

package config

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/agrea/ptr"
	"github.com/stretchr/testify/require"
)

const (
	credentials = `
accessKeyId: id
secretAccessKey: key
`

	configTemplate = `
http:
  port: 49021
  ssl: true
request_aggregation_delay: 15s
page_size: 50
ssl:
  cert: %s
  key: %s
  ca_cert: %s
credentials_path: %s
env:
  SLURM_CONF: /etc/slurm/config.yaml
  PATH: /a/b/c
`
)

func TestConfig(t *testing.T) {
	file, err := os.CreateTemp("", "test-cfg-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()

	cert, err := os.CreateTemp("", "test-cert-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(cert.Name()) }()
	defer func() { _ = cert.Close() }()

	key, err := os.CreateTemp("", "test-key-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(key.Name()) }()
	defer func() { _ = key.Close() }()

	caCert, err := os.CreateTemp("", "test-ca-cert-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(caCert.Name()) }()
	defer func() { _ = caCert.Close() }()

	creds, err := os.CreateTemp("", "test-creds-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(creds.Name()) }()
	defer func() { _ = creds.Close() }()
	credsPath := creds.Name()

	_, err = creds.WriteString(credentials)
	require.NoError(t, err)

	_, err = file.WriteString(fmt.Sprintf(configTemplate, cert.Name(), key.Name(), caCert.Name(), creds.Name()))
	require.NoError(t, err)

	cfg, err := NewFromFile(file.Name())
	require.NoError(t, err)

	expected := &Config{
		HTTP: Endpoint{
			Port: 49021,
			SSL:  true,
		},
		RequestAggregationDelay: 15 * time.Second,
		PageSize:                ptr.Int(50),
		SSL: &SSL{
			Cert:   cert.Name(),
			Key:    key.Name(),
			CaCert: caCert.Name(),
		},
		CredsPath:   &credsPath,
		Credentials: map[string]string{"accessKeyId": "id", "secretAccessKey": "key"},
		Env: map[string]string{
			"SLURM_CONF": "/etc/slurm/config.yaml",
			"PATH":       "/a/b/c",
		},
	}
	require.Equal(t, expected, cfg)

	var path string
	if path = os.Getenv("PATH"); len(path) == 0 {
		path = "/a/b/c"
	} else {
		path = path + ":" + "/a/b/c"
	}

	err = cfg.UpdateEnv()
	require.NoError(t, err)

	require.Equal(t, "/etc/slurm/config.yaml", os.Getenv("SLURM_CONF"))
	require.Equal(t, path, os.Getenv("PATH"))
}
func TestValidate(t *testing.T) {
	cert, err := os.CreateTemp("", "test-cert-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(cert.Name()) }()
	defer func() { _ = cert.Close() }()

	key, err := os.CreateTemp("", "test-key-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(key.Name()) }()
	defer func() { _ = key.Close() }()

	caCert, err := os.CreateTemp("", "test-ca-cert-*.yml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(caCert.Name()) }()
	defer func() { _ = caCert.Close() }()

	testCases := []struct {
		name string
		cfg  Config
		err  string
	}{
		{
			name: "Case 1: missing port",
			err:  "port is not set",
		},
		{
			name: "Case 2: missing request_aggregation_delay",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},
			},
			err: "request_aggregation_delay is not set",
		},
		{
			name: "Case 3: missing ssl section",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},
				RequestAggregationDelay: time.Second,
			},
			err: "missing ssl section",
		},
		{
			name: "Case 4.1: missing server certificate",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},
				RequestAggregationDelay: time.Second,
				SSL:                     &SSL{},
			},
			err: "missing filename for server certificate",
		},
		{
			name: "Case 4.2: missing server key",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},
				RequestAggregationDelay: time.Second,
				SSL: &SSL{
					Cert: cert.Name(),
				},
			},
			err: "missing filename for server key",
		},
		{
			name: "Case 4.3: missing CA certificate",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},

				RequestAggregationDelay: time.Second,
				SSL: &SSL{
					Cert: cert.Name(),
					Key:  key.Name(),
				},
			},
			err: "missing filename for CA certificate",
		},
		{
			name: "Case 5.1: valid input with cert",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
					SSL:  true,
				},
				RequestAggregationDelay: time.Second,
				SSL: &SSL{
					Cert:   cert.Name(),
					Key:    key.Name(),
					CaCert: caCert.Name(),
				},
			},
		},
		{
			name: "Case 5.2: valid input without cert",
			cfg: Config{
				HTTP: Endpoint{
					Port: 1,
				},
				RequestAggregationDelay: time.Second,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
