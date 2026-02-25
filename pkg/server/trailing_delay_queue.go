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

	lru "github.com/hashicorp/golang-lru"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
)

const RequestHistorySize = 100

type Hashable interface {
	Hash() (string, error)
}

type HandleFunc func(any) (any, *httperr.Error)

type Completion struct {
	Ret     any
	Status  int
	Message string
}

type TrailingDelayQueue struct {
	mutex    sync.Mutex
	handle   HandleFunc
	delay    time.Duration
	shutdown chan struct{}
	timers   map[string]*time.Timer // map hash:timer
	store    *lru.Cache             // map hash:processing result
}

func NewTrailingDelayQueue(handle HandleFunc, delay time.Duration) *TrailingDelayQueue {
	q := &TrailingDelayQueue{
		delay:    delay,
		handle:   handle,
		shutdown: make(chan struct{}),
		timers:   make(map[string]*time.Timer),
	}
	q.store, _ = lru.New(RequestHistorySize)

	go q.run()

	return q
}

func (q *TrailingDelayQueue) run() {
	<-q.shutdown
	klog.V(4).Infof("queue shutdown")
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for _, timer := range q.timers {
		timer.Stop()
	}
}

func (q *TrailingDelayQueue) Submit(item Hashable) (string, error) {
	klog.Infof("Submit request; delay processing by %s", q.delay.String())

	hash, err := item.Hash()
	if err != nil {
		return "", fmt.Errorf("failed to hash request: %v", err)
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	entry := &Completion{
		Status:  http.StatusAccepted,
		Message: fmt.Sprintf("request ID %s has been created", hash),
	}

	// if the timer for the request exists, stop it
	if timer, ok := q.timers[hash]; ok {
		timer.Stop()
	}

	q.timers[hash] = time.AfterFunc(q.delay, func() {
		klog.Infof("Processing request ID %s", hash)
		// process the request
		data, err := q.handle(item)

		// update the status and results
		q.mutex.Lock()
		defer q.mutex.Unlock()
		// update the status only there was no later request for the same hash
		if currEntry, ok := q.store.Get(hash); ok && currEntry == entry {
			if err != nil {
				entry.Status = err.Code()
				entry.Message = err.Error()
				klog.Errorf("HTTP %d: %s", entry.Status, entry.Message)
			} else {
				entry.Ret = data
				entry.Status = http.StatusOK
				klog.Info("HTTP 200")
			}
			q.store.Add(hash, entry)
		}
		delete(q.timers, hash)
	})
	q.store.Add(hash, entry)

	return hash, nil
}

func (q *TrailingDelayQueue) Get(hash string) *Completion {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if res, ok := q.store.Get(hash); ok {
		return res.(*Completion)
	}

	return &Completion{
		Message: fmt.Sprintf("request ID %s not found", hash),
		Status:  http.StatusNotFound,
	}
}

func (q *TrailingDelayQueue) Shutdown() {
	close(q.shutdown)
}
