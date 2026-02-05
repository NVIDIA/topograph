/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/NVIDIA/topograph/pkg/config"
	"github.com/NVIDIA/topograph/pkg/test"
	"github.com/stretchr/testify/require"
)

const (
	slurmSmallConfig = `
SwitchName=S1 Switches=S[2-3]
SwitchName=S2 Nodes=I[21-22,25]
SwitchName=S3 Nodes=I[34-36]
`
)

func TestServerIntegration(t *testing.T) {

	port, err := test.GetAvailablePort()
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
		filename       string
		expected       string
		generateMethod string
		timeout        time.Duration
	}{
		{
			filename: "../../tests/integration/payload-error-500-after-retries.json",
			timeout:  1 * time.Minute,
		},
		{
			filename:       "../../tests/integration/payload-invalid-http-method.json",
			generateMethod: "GET",
		},
		{
			filename: "../../tests/integration/payload-invalid-user-input.json",
		},
		{
			filename: "../../tests/integration/payload-repeated-202.json",
		},
		{
			filename: "../../tests/integration/payload-request-id-not-found.json",
		},
		{
			filename: "../../tests/integration/payload-valid-topology.json",
			expected: slurmSmallConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			payloadFile, err := os.Open(tc.filename)
			require.NoError(t, err)
			defer payloadFile.Close()

			payload, err := io.ReadAll(payloadFile)
			require.NoError(t, err)
			if tc.timeout <= 0 {
				tc.timeout = 10 * time.Second
			}

			testIntegration(t, baseURL, string(payload), tc.expected, tc.generateMethod, tc.timeout)
		})
	}
}

func testIntegration(t *testing.T, baseURL, payload, expected, generateMethod string, timeout time.Duration) {

	start, delay := time.Now(), 2*time.Second

	// parse payload to get the request details
	tp, err := ParseTestPayload([]byte(payload))
	require.NoError(t, err)

	//set generate method if not set
	if len(generateMethod) == 0 {
		generateMethod = "POST"
	}

	//validate input parameters
	require.Equal(t, "test", tp.Provider.Name)

	//construct generate request
	req, err := http.NewRequest(generateMethod, baseURL+"/v1/generate", bytes.NewBuffer([]byte(payload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// send generate request and validate response code
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, tp.Provider.Params.GenerateResponseCode, resp.StatusCode)
	if resp.StatusCode != http.StatusAccepted {
		return
	}

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out := string(body)
	resp.Body.Close()

	// retrieve topology config
	params := url.Values{}
	params.Add("uid", out)
	fullURL := fmt.Sprintf("%s?%s", baseURL+"/v1/topology", params.Encode())

	for time.Since(start) < timeout {
		time.Sleep(delay)
		resp, err = http.Get(fullURL)
		require.NoError(t, err)

		if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			continue
		}

		if resp.StatusCode == tp.Provider.Params.TopologyResponseCode {
			break
		} else {
			resp.Body.Close()
		}
	}

	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, tp.Provider.Params.TopologyResponseCode, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, stringToLineMap(expected), stringToLineMap(string(body)))
}

func ParseTestPayload(data []byte) (*TestPayload, error) {
	var tp TestPayload
	if err := json.Unmarshal(data, &tp); err != nil {
		return nil, fmt.Errorf("failed to parse test payload: %v", err)
	}

	return &tp, nil
}

type TestPayload struct {
	Provider TestProvider `json:"provider"`
}

type TestProvider struct {
	Name   string `json:"name"`
	Params struct {
		ModelPath            string `json:"model_path,omitempty"`
		TestcaseName         string `json:"testcaseName,omitempty"`
		Description          string `json:"description,omitempty"`
		GenerateResponseCode int    `json:"generateResponseCode,omitempty"`
		TopologyResponseCode int    `json:"topologyResponseCode,omitempty"`
		ModelFileName        string `json:"modelFileName,omitempty"`
		ErrorMessage         string `json:"errorMessage,omitempty"`
	} `json:"params"`
}
