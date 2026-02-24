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

package server_test

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/server"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestRepeatingPayload(t *testing.T) {
	var counter int32

	processItem := func(item any) (any, *httperr.Error) {
		atomic.AddInt32(&counter, 1)
		return nil, nil
	}

	queue := server.NewTrailingDelayQueue(processItem, 500*time.Millisecond)

	request := &topology.Request{
		Provider: topology.Provider{Name: "test"},
		Engine: topology.Engine{
			Name:   "test",
			Params: map[string]any{"a": 1, "b": 2, "c": 3, "d": 4},
		},
	}
	for cycle := 1; cycle <= 2; cycle++ {
		for range 3 {
			_, err := queue.Submit(request)
			require.NoError(t, err)
			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(time.Second)
		val := int(atomic.LoadInt32(&counter))
		require.Equal(t, cycle, val)
	}

	queue.Shutdown()
}

func TestVaryingPayload(t *testing.T) {

	processItem := func(item any) (any, *httperr.Error) {
		return item, nil
	}

	queue := server.NewTrailingDelayQueue(processItem, 500*time.Millisecond)

	submissions := [3]string{}
	for i := range len(submissions) {
		request := &topology.Request{
			Provider: topology.Provider{Name: "test"},
			Engine: topology.Engine{
				Name:   "test",
				Params: map[string]any{"a": i, "b": 2, "c": 3, "d": 4},
			},
		}
		uid, err := queue.Submit(request)
		require.NoError(t, err)
		submissions[i] = uid
		time.Sleep(100 * time.Millisecond)
	}

	for i := 1; i < len(submissions); i++ {
		require.NotEqual(t, submissions[i], submissions[i-1])
	}

	time.Sleep(time.Second)
	for i := 0; i < len(submissions); i++ {
		res := queue.Get(submissions[i])
		require.NotNil(t, res)
		require.Equal(t, http.StatusOK, res.Status)
		require.Equal(t, i, res.Ret.(*topology.Request).Engine.Params["a"])
	}

	queue.Shutdown()
}

func TestLRU(t *testing.T) {
	cache, _ := lru.New(3)

	_, ok := cache.Get(1)
	require.False(t, ok) // not found

	require.False(t, cache.Add(1, 1)) // w/o eviction
	require.False(t, cache.Add(2, 2)) // w/o eviction

	v, ok := cache.Get(1)
	require.True(t, ok) // found
	require.Equal(t, 1, v)

	require.False(t, cache.Add(3, 3)) // w/o eviction
	require.True(t, cache.Add(4, 4))  // with eviction of LRU "2"

	_, ok = cache.Get(2)
	require.False(t, ok) // not found

	v, ok = cache.Get(1)
	require.True(t, ok) // found
	require.Equal(t, 1, v)
}
