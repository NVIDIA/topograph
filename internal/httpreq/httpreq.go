/*
 * Copyright 2024-2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package httpreq

import (
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
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

type RequestFunc func() (*http.Request, error)

// DoRequest sends HTTP requests and returns HTTP response
func DoRequest(f RequestFunc, insecureSkipVerify bool) (*http.Response, []byte, *httperr.Error) {
	req, err := f()
	if err != nil {
		return nil, nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}
	klog.V(4).Infof("Sending HTTP request %s", req.URL.String())
	client := &http.Client{}
	if insecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		code := http.StatusBadGateway
		if resp != nil {
			code = resp.StatusCode
		}
		return resp, nil, httperr.NewError(code, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, httperr.NewError(http.StatusInternalServerError, fmt.Sprintf("failed to read HTTP response: %v", err))
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, body, nil
	}

	return resp, body, httperr.NewError(resp.StatusCode, string(body))
}

// DoRequestWithRetries sends HTTP requests and returns HTTP response; retries if needed
func DoRequestWithRetries(f RequestFunc, insecureSkipVerify bool) (resp *http.Response, body []byte, err *httperr.Error) {
	klog.V(4).Infof("Sending HTTP request with retries")
	for r := 1; r <= retries; r++ {
		resp, body, err = DoRequest(f, insecureSkipVerify)
		if err == nil || resp == nil || !retryHttpCodes[resp.StatusCode] {
			break
		}
		wait := time.Duration(int(math.Pow(2, float64(r))) * time.Now().Second())
		klog.Infof("Request error: %v. Retrying in %s\n", err, wait.String())
		time.Sleep(wait)
	}

	return
}

func GetURL(baseURL string, query map[string]string, paths ...string) (string, *httperr.Error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", httperr.NewError(http.StatusBadRequest, err.Error())
	}

	u.Path = path.Join(append([]string{u.Path}, paths...)...)

	if len(query) != 0 {
		q := u.Query()
		for key, val := range query {
			q.Set(key, val)
		}
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}
