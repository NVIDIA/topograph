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

package utils

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"k8s.io/klog/v2"
)

var (
	// retries specifies number of retries
	retries = 3

	//retryHttpCodes specifies on which errors to retry the request
	retryHttpCodes = map[int]bool{
		http.StatusRequestTimeout:     true,
		http.StatusTooManyRequests:    true,
		http.StatusBadGateway:         true,
		http.StatusServiceUnavailable: true,
		http.StatusGatewayTimeout:     true,
	}
)

type HttpRequestFunc func() (*http.Request, error)

// HttpRequest sends HTTP requests and returns HTTP response
func HttpRequest(f HttpRequestFunc) (*http.Response, []byte, error) {
	req, err := f()
	if err != nil {
		return nil, nil, err
	}
	klog.V(4).Infof("Sending HTTP request %s", req.URL.String())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read HTTP response: %v", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, body, nil
	}

	return resp, body, fmt.Errorf("HTTP %d %s: %s", resp.StatusCode, resp.Status, string(body))
}

// HttpRequestWithRetries sends HTTP requests and returns HTTP response; retries if needed
func HttpRequestWithRetries(f HttpRequestFunc) (resp *http.Response, body []byte, err error) {
	klog.V(4).Infof("Sending HTTP request with retries")
	for r := 1; r <= retries; r++ {
		resp, body, err = HttpRequest(f)
		if err == nil || !retryHttpCodes[resp.StatusCode] {
			break
		}
		wait := time.Duration(int(math.Pow(2, float64(r))) * time.Now().Second())
		klog.Infof("Request error: %v. Retrying in %s\n", err, wait.String())
		time.Sleep(wait)
	}

	return
}
