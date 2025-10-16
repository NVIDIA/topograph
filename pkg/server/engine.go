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
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/registry"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	maxRetries = 5
	baseDelay  = 2 * time.Second
)

type asyncController struct {
	queue *TrailingDelayQueue
}

func processRequest(item any) (any, *HTTPError) {
	return processRequestWithRetries(baseDelay, item.(*topology.Request), processTopologyRequest)
}

func processRequestWithRetries(delay time.Duration, tr *topology.Request, f func(*topology.Request) ([]byte, *HTTPError)) ([]byte, *HTTPError) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var code int
		start := time.Now()

		ret, err := f(tr)
		if err != nil {
			code = err.Code
		} else {
			code = http.StatusOK
		}
		metrics.AddTopologyRequest(tr.Provider.Name, tr.Engine.Name, code, time.Since(start))

		if code != http.StatusInternalServerError || attempt == maxRetries {
			return ret, err
		}

		// Exponential backoff: delay = delay * 2^attempt
		sleep := time.Duration(float64(delay) * math.Pow(2, float64(attempt)))
		klog.Infof("Attempt %d failed: %v — retrying in %v", attempt+1, err, sleep)
		time.Sleep(sleep)
	}

	return nil, NewHTTPError(http.StatusInternalServerError, "no attempts")
}

func processTopologyRequest(tr *topology.Request) ([]byte, *HTTPError) {
	klog.InfoS("Creating topology config", "provider", tr.Provider.Name, "engine", tr.Engine.Name)
	defer klog.Info("Topology request completed")

	engLoader, err := registry.Engines.Get(tr.Engine.Name)
	if err != nil {
		klog.Error(err.Error())
		if errors.Is(err, engines.ErrUnsupportedEngine) {
			return nil, NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return nil, NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	prvLoader, err := registry.Providers.Get(tr.Provider.Name)
	if err != nil {
		klog.Error(err.Error())
		if errors.Is(err, providers.ErrUnsupportedProvider) {
			return nil, NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return nil, NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	ctx := context.Background()

	eng, err := engLoader(ctx, tr.Engine.Params)
	if err != nil {
		// TODO: Logic to determine between StatusBadRequest and StatusInternalServerError
		return nil, NewHTTPError(http.StatusBadRequest, err.Error())
	}

	prv, err := prvLoader(ctx, providers.Config{
		Creds:  checkCredentials(tr.Provider.Creds, srv.cfg.Credentials),
		Params: tr.Provider.Params,
	})
	if err != nil {
		// TODO: Logic to determine between StatusBadRequest and StatusInternalServerError
		return nil, NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Optional provider interface if it directly supports getting compute instances.
	// (e.g., Test provider)
	type simpleGetComputeInstances interface {
		GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error)
	}

	// if the instance/node mapping is not provided in the payload, get the mapping from the provider
	computeInstances := tr.Nodes
	if len(computeInstances) == 0 {
		var err error
		switch t := prv.(type) {
		case simpleGetComputeInstances:
			computeInstances, err = t.GetComputeInstances(ctx)
		default:
			computeInstances, err = eng.GetComputeInstances(ctx, prv)
		}

		if err != nil {
			return nil, NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	var root *topology.Vertex
	if srv.cfg.FwdSvcURL != nil {
		// forward the request to the global service
		root, err = forwardRequest(ctx, tr, *srv.cfg.FwdSvcURL, computeInstances)
	} else {
		root, err = prv.GenerateTopologyConfig(ctx, srv.cfg.PageSize, computeInstances)
	}
	if err != nil {
		klog.Error(err.Error())
		return nil, NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	data, err := eng.GenerateOutput(ctx, root, tr.Engine.Params)
	if err != nil {
		klog.Error(err.Error())
		return nil, NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return data, nil
}

func checkCredentials(payloadCreds, cfgCreds map[string]string) map[string]string {
	if len(payloadCreds) != 0 {
		return payloadCreds
	}
	return cfgCreds
}
