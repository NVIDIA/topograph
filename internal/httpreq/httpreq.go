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
	"strconv"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
)

const (
	// maxRetries is the maximum number of retry attempts
	maxRetries = 5

	// baseDelay is the initial delay used for retry backoff
	baseDelay = 500 * time.Millisecond

	// maxRetryAfter is the maximum delay allowed when honoring a Retry-After header
	maxRetryAfter = 5 * time.Minute
)

// ShouldRetry returns true if the given HTTP status code is retryable
func ShouldRetry(status int) bool {
	switch status {
	case
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

func ParseRetryAfter(resp *http.Response) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}

	value := resp.Header.Get("Retry-After")
	if len(value) == 0 {
		return 0, false
	}

	// check if Retry-After is seconds
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		if seconds > int(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}

	// check if Retry-After is an HTTP date
	if t, err := http.ParseTime(value); err == nil {
		if delay := time.Until(t); delay > 0 {
			if delay > maxRetryAfter {
				delay = maxRetryAfter
			}
			return delay, true
		}
	}

	return 0, false
}

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
	attempt := 0
	for {
		attempt++
		resp, body, err = DoRequest(f, insecureSkipVerify)
		if err == nil || attempt == maxRetries || !ShouldRetry(err.Code()) {
			break
		}
		wait := GetNextBackoff(resp, baseDelay, attempt-1)
		klog.Infof("Attempt %d failed with error: %v. Retrying in %s", attempt, err, wait.String())
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

// GetNextBackoff determines the retry delay from Retry-After header or exponential backoff
func GetNextBackoff(resp *http.Response, initialBackoff time.Duration, attempt int) time.Duration {
	wait, valid := ParseRetryAfter(resp)
	if !valid {
		wait = initialBackoff * time.Duration(int(math.Pow(2, float64(attempt))))
	}
	return wait
}
