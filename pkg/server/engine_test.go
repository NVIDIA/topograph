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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestCheckCredentials(t *testing.T) {
	credPayload := map[string]string{"key1": "val1"}
	credConfig := map[string]string{"key2": "val2"}

	testCases := []struct {
		name     string
		payload  map[string]string
		config   map[string]string
		expected map[string]string
	}{
		{
			name:     "Case 1: payload only",
			payload:  credPayload,
			expected: credPayload,
		},
		{
			name:     "Case 2: config only",
			config:   credConfig,
			expected: credConfig,
		},
		{
			name:     "Case 3: both",
			payload:  credPayload,
			config:   credConfig,
			expected: credPayload,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, checkCredentials(tc.payload, tc.config))
		})
	}

}

type retrier struct {
	codes []int
}

func (r *retrier) callback(_ *topology.Request) ([]byte, *httperr.Error) {
	var code int
	if len(r.codes) == 0 {
		code = http.StatusInternalServerError
	} else {
		code = r.codes[0]
		r.codes = r.codes[1:]
	}

	if code == http.StatusOK {
		return []byte{1, 2, 3, 4, 5}, nil
	}

	return nil, httperr.NewError(code, "error")
}

func TestProcessRequestWithRetries(t *testing.T) {
	tr := &topology.Request{
		Provider: topology.Provider{
			Name: "test",
		},
		Engine: topology.Engine{
			Name: "test",
		},
	}

	testCases := []struct {
		name    string
		retrier *retrier
		err     string
		code    int
	}{
		{
			name:    "Case 1: retry and failure",
			retrier: &retrier{},
			err:     "error",
			code:    500,
		},
		{
			name:    "Case 2: retry and success",
			retrier: &retrier{codes: []int{http.StatusInternalServerError, http.StatusOK}},
		},
		{
			name:    "Case 3: user error",
			retrier: &retrier{codes: []int{http.StatusBadRequest}},
			err:     "error",
			code:    400,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ret, err := processRequestWithRetries(time.Millisecond, tr, tc.retrier.callback)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
				require.Equal(t, tc.code, err.Code())
			} else {
				require.Nil(t, err)
				require.Equal(t, []byte{1, 2, 3, 4, 5}, ret)
			}
		})
	}
}
