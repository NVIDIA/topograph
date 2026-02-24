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
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const RequestHistorySize = 100

type HandleFunc func(any) (any, *httperr.Error)

type Completion struct {
	Ret     any
	Status  int
	Message string
}

type TrailingDelayQueue struct {
	mutex    sync.Mutex
	ticker   *time.Ticker
	handle   HandleFunc
	delay    time.Duration
	shutdown chan struct{}
	items    sync.Map   // map request hash to item being processed
	store    *lru.Cache // map uid:process result
}

type QueueItem struct {
	item     any       // current item to be processed, if not nil
	lastTime time.Time // last submit time
	uid      string    // unique item processing ID
}

func NewTrailingDelayQueue(handle HandleFunc, delay time.Duration) *TrailingDelayQueue {
	q := &TrailingDelayQueue{
		delay:    delay,
		handle:   handle,
		shutdown: make(chan struct{}),
		ticker:   time.NewTicker(delay),
	}
	q.store, _ = lru.New(RequestHistorySize)

	go q.run()

	return q
}

func (q *TrailingDelayQueue) run() {
	defer q.ticker.Stop()
	for {
		select {
		case <-q.shutdown:
			klog.V(4).Infof("queue shutdown")
			return
		case <-q.ticker.C:
			q.items.Range(func(key, value any) bool {
				entry := value.(*QueueItem)
				if time.Since(entry.lastTime) < q.delay {
					return true
				}
				q.mutex.Lock()
				q.items.Delete(key)
				q.mutex.Unlock()

				item := entry.item
				uid := entry.uid

				res := &Completion{}
				if data, err := q.handle(item); err != nil {
					res.Status = err.Code()
					res.Message = err.Error()
					klog.Errorf("HTTP %d: %s", res.Status, res.Message)
				} else {
					res.Ret = data
					res.Status = http.StatusOK
					klog.Info("HTTP 200")
				}

				q.mutex.Lock()
				q.store.Add(uid, res)
				q.mutex.Unlock()
				return true
			})
		}
	}
}

func (q *TrailingDelayQueue) Submit(item any) (string, error) {

	klog.Infof("Submit request; delay processing by %s", q.delay.String())
	var hash string
	var err error
	if h, ok := item.(interface{ Hash() (string, error) }); ok {
		hash, err = h.Hash()
	} else {
		hash, err = topology.GetHash(item)
	}

	if err != nil {
		return "", fmt.Errorf("failed to hash request: %v", err)
	}

	entry := &QueueItem{
		item:     item,
		lastTime: time.Now(),
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()
	if prevEntry, exists := q.items.Load(hash); !exists {
		entry.uid = uuid.New().String()
		q.items.Store(hash, entry)
	} else {
		entry.uid = prevEntry.(*QueueItem).uid
		q.items.Swap(hash, entry)
	}

	res := &Completion{
		Status:  http.StatusAccepted,
		Message: fmt.Sprintf("request ID %s has not completed yet", entry.uid),
	}
	q.store.Add(entry.uid, res)

	return entry.uid, nil
}

func (q *TrailingDelayQueue) Get(uid string) *Completion {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if res, ok := q.store.Get(uid); ok {
		return res.(*Completion)
	}

	return &Completion{
		Message: fmt.Sprintf("request ID %s not found", uid),
		Status:  http.StatusNotFound,
	}
}

func (q *TrailingDelayQueue) Shutdown() {
	close(q.shutdown)
}
