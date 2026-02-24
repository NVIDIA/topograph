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
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/server"
)

func TestRepeatingPayload(t *testing.T) {
	var counter int32
	type Int struct{ val int }

	processItem := func(item interface{}) (interface{}, *httperr.Error) {
		klog.Infof("Processing item: %v\n", item)
		atomic.AddInt32(&counter, 1)
		return nil, nil
	}

	queue := server.NewTrailingDelayQueue(processItem, 2*time.Second)

	for cycle := 1; cycle <= 2; cycle++ {
		for i := 0; i < 3; i++ {
			_, err := queue.Submit(&Int{val: 10})
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)
		}

		time.Sleep(4 * time.Second)
		val := int(atomic.LoadInt32(&counter))
		require.Equal(t, cycle, val)
	}

	queue.Shutdown()
}

func TestVaryingPayload(t *testing.T) {
	type Int struct {
		Val int `json:"val"`
	}

	processItem := func(item interface{}) (interface{}, *httperr.Error) {
		klog.Infof("Processing item: %v\n", item)
		item.(*Int).Val = item.(*Int).Val * 2
		return item, nil
	}

	queue := server.NewTrailingDelayQueue(processItem, 1*time.Second)

	submissions := [3]string{}
	for i := 0; i < len(submissions); i++ {
		uid, err := queue.Submit(&Int{Val: i})
		require.NoError(t, err)
		submissions[i] = uid
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	for i := 0; i < len(submissions); i++ {
		res := queue.Get(submissions[i])
		require.NotNil(t, res)
		require.Equal(t, http.StatusOK, res.Status)
		require.Equal(t, i*2, res.Ret.(*Int).Val)
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
