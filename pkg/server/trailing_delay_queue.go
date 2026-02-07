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
	item     any        // current item to be processed, if not nil
	lastTime time.Time  // last submit time
	uid      string     // unique item processing ID
	store    *lru.Cache // map uid:process result
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
			var item any
			var uid string
			q.mutex.Lock()
			if time.Since(q.lastTime) > q.delay && q.item != nil {
				item = q.item
				uid = q.uid
				q.item = nil
				q.uid = ""
			}
			q.mutex.Unlock()

			if item != nil {
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
			}
		}
	}
}

func (q *TrailingDelayQueue) Submit(item any) string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	klog.Infof("Submit request; delay processing by %s", q.delay.String())
	q.item = item
	q.lastTime = time.Now()
	if len(q.uid) == 0 {
		q.uid = uuid.New().String()
		res := &Completion{
			Status:  http.StatusAccepted,
			Message: fmt.Sprintf("request ID %s has not completed yet", q.uid),
		}
		q.store.Add(q.uid, res)
	}

	return q.uid
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
