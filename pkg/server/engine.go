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
	"net/http"
	"time"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/factory"
	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/utils"
)

var providerName string = common.ProviderTest // default value uses test provider
var engineName string = common.EngineTest     // default value uses test engine

type asyncController struct {
	queue *utils.TrailingDelayQueue
}

func processRequest(item interface{}) (interface{}, *common.HTTPError) {
	tr := item.(*common.TopologyRequest)
	var code int
	start := time.Now()

	ret, err := processTopologyRequest(tr)
	if err != nil {
		code = err.Code
	} else {
		code = http.StatusOK
	}
	metrics.Add(tr.Provider.Name, tr.Engine.Name, code, time.Since(start))

	return ret, err
}

func processTopologyRequest(tr *common.TopologyRequest) ([]byte, *common.HTTPError) {
	engName := tr.Engine.Name
	prvName := tr.Provider.Name
	if engName == "" {
		engName = engineName
	}
	if prvName == "" {
		prvName = providerName
	}
	klog.InfoS("Creating topology config", "provider", engName, "engine", prvName)

	eng, httpErr := factory.GetEngine(engName)
	if httpErr != nil {
		klog.Error(httpErr.Error())
		return nil, httpErr
	}

	prv, httpErr := factory.GetProvider(prvName, tr.Provider.Params)
	if httpErr != nil {
		klog.Error(httpErr.Error())
		return nil, httpErr
	}

	ctx := context.TODO()

	// if the instance/node mapping is not provided in the payload, get the mapping from the provider
	computeInstances := tr.Nodes
	if len(computeInstances) == 0 {
		var err error
		computeInstances, err = prv.GetComputeInstances(ctx, eng)
		if err != nil {
			return nil, common.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	creds, err := prv.GetCredentials(checkCredentials(tr.Provider.Creds, srv.cfg.Credentials))
	if err != nil {
		klog.Error(err.Error())
		return nil, common.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	var root *common.Vertex
	if srv.cfg.FwdSvcURL != nil {
		// forward the request to the global service
		root, err = forwardRequest(ctx, tr, *srv.cfg.FwdSvcURL, computeInstances)
	} else {
		root, err = prv.GenerateTopologyConfig(ctx, creds, srv.cfg.PageSize, computeInstances)
	}
	if err != nil {
		klog.Error(err.Error())
		return nil, common.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	data, err := eng.GenerateOutput(ctx, root, tr.Engine.Params)
	if err != nil {
		klog.Error(err.Error())
		return nil, common.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return data, nil
}

func checkCredentials(payloadCreds, cfgCreds map[string]string) map[string]string {
	if len(payloadCreds) != 0 {
		return payloadCreds
	}
	return cfgCreds
}
