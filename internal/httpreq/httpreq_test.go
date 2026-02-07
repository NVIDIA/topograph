/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package httpreq

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/internal/httperr"
)

func TestShouldRetry(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		retry  bool
	}{
		{
			name:   "request timeout",
			status: http.StatusRequestTimeout, // 408
			retry:  true,
		},
		{
			name:   "too many requests",
			status: http.StatusTooManyRequests, // 429
			retry:  true,
		},
		{
			name:   "internal server error",
			status: http.StatusInternalServerError, // 500
			retry:  true,
		},
		{
			name:   "bad gateway",
			status: http.StatusBadGateway, // 502
			retry:  true,
		},
		{
			name:   "service unavailable",
			status: http.StatusServiceUnavailable, // 503
			retry:  true,
		},
		{
			name:   "gateway timeout",
			status: http.StatusGatewayTimeout, // 504
			retry:  true,
		},
		{
			name:   "ok",
			status: http.StatusOK, // 200
			retry:  false,
		},
		{
			name:   "bad request",
			status: http.StatusBadRequest, // 400
			retry:  false,
		},
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized, // 401
			retry:  false,
		},
		{
			name:   "not found",
			status: http.StatusNotFound, // 404
			retry:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retry := ShouldRetry(tc.status)
			require.Equal(t, tc.retry, retry)
		})
	}
}

type callback struct{ status, attempts int }

func (c *callback) Inc() (*http.Request, *httperr.Error) {
	c.attempts++
	return nil, httperr.NewError(c.status, "error")
}

func TestDoRequestWithRetries(t *testing.T) {
	testCases := []struct {
		name     string
		status   int
		attempts int
	}{
		{
			name:     "gateway timeout",
			status:   http.StatusGatewayTimeout, // 504
			attempts: maxRetries,
		},
		{
			name:     "unauthorized",
			status:   http.StatusUnauthorized, // 401
			attempts: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &callback{status: tc.status}
			_, err := DoRequestWithRetries(c.Inc, false)
			require.Equal(t, tc.status, err.Code())
			require.Equal(t, tc.attempts, c.attempts)
		})
	}
}

func TestGetURL(t *testing.T) {
	testCases := []struct {
		name    string
		baseURL string
		paths   []string
		query   map[string]string
		url     string
		err     string
	}{
		{
			name:    "Case 1: bad base URL",
			baseURL: "123:",
			err:     `parse "123:": first path segment in URL cannot contain colon`,
		},
		{
			name:    "Case 2: single base URL",
			baseURL: "http://localhost",
			url:     "http://localhost",
		},
		{
			name:    "Case 3: base URL with path",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			url:     "http://localhost/a/b/c/d",
		},
		{
			name:    "Case 4: base URL with path and query",
			baseURL: "http://localhost/",
			paths:   []string{"a", "b/", "/c", "d/"},
			query:   map[string]string{"key1": "val1", "key2": "val2"},
			url:     "http://localhost/a/b/c/d?key1=val1&key2=val2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := GetURL(tc.baseURL, tc.query, tc.paths...)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {

				require.Nil(t, err)
				require.Equal(t, tc.url, u)
			}
		})
	}
}

func TestGetNextBackoff(t *testing.T) {
	testCases := []struct {
		name  string
		resp  *http.Response
		iter  int
		check func(time.Duration) bool
	}{
		{
			name: "Case 1.1: valid Retry-After header (seconds)",
			resp: &http.Response{
				Header: http.Header{
					"Retry-After": []string{"5"},
				},
			},
			iter:  0,
			check: func(wait time.Duration) bool { return wait == 5*time.Second },
		},
		{
			name: "Case 1.2: exceeded Retry-After header (seconds)",
			resp: &http.Response{
				Header: http.Header{
					"Retry-After": []string{"1000"},
				},
			},
			iter:  0,
			check: func(wait time.Duration) bool { return wait == maxRetryAfter },
		},
		{
			name: "Case 2.1: valid Retry-After header (date)",
			resp: &http.Response{
				Header: http.Header{
					"Retry-After": []string{time.Now().Add(10 * time.Second).Format(time.RFC850)},
				},
			},
			iter:  3,
			check: func(wait time.Duration) bool { return wait > 8*time.Second && wait < 12*time.Second },
		},
		{
			name: "Case 2.2: exceeded Retry-After header (date)",
			resp: &http.Response{
				Header: http.Header{
					"Retry-After": []string{time.Now().Add(10 * time.Minute).Format(time.RFC850)},
				},
			},
			iter:  3,
			check: func(wait time.Duration) bool { return wait == maxRetryAfter },
		},
		{
			name: "Case 3.1: no Retry-After header",
			resp: &http.Response{
				Header: http.Header{},
			},
			iter:  0,
			check: func(wait time.Duration) bool { return wait == 500*time.Millisecond },
		},
		{
			name:  "Case 3.2: no response",
			iter:  1,
			check: func(wait time.Duration) bool { return wait == time.Second },
		},
		{
			name: "Case 4: invalid Retry-After header",
			resp: &http.Response{
				Header: http.Header{
					"Retry-After": []string{"invalid"},
				},
			},
			iter:  2,
			check: func(wait time.Duration) bool { return wait == 2*time.Second },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wait := GetNextBackoff(tc.resp, backOff, tc.iter)
			correct := tc.check(wait)
			require.True(t, correct)
		})
	}
}
