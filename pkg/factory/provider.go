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

package factory

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers/aws"
	"github.com/NVIDIA/topograph/pkg/providers/baremetal"
	"github.com/NVIDIA/topograph/pkg/providers/cw"
	"github.com/NVIDIA/topograph/pkg/providers/gcp"
	"github.com/NVIDIA/topograph/pkg/providers/oci"
	"github.com/NVIDIA/topograph/pkg/translate"
)

func GetProvider(provider string, params map[string]string) (common.Provider, *common.HTTPError) {
	var (
		prv common.Provider
		err error
	)

	switch provider {
	case common.ProviderAWS:
		prv, err = aws.GetProvider()
	case common.ProviderGCP:
		prv, err = gcp.GetProvider()
	case common.ProviderOCI:
		prv, err = oci.GetProvider()
	case common.ProviderCW:
		prv, err = cw.GetProvider()
	case common.ProviderBM:
		prv, err = baremetal.GetProvider()
	case common.ProviderTest:
		prv, err = GetTestProvider(params)
	default:
		return nil, common.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unsupported provider %q", provider))
	}

	if err != nil {
		return nil, common.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return prv, nil
}

type testProvider struct {
	tree          *common.Vertex
	instance2node map[string]string
}

func GetTestProvider(params map[string]string) (*testProvider, error) {
	p := &testProvider{}

	if path, ok := params[common.KeyModelPath]; !ok || len(path) == 0 {
		p.tree, p.instance2node = translate.GetTreeTestSet(false)
	} else {
		klog.InfoS("Using simulated topology", "model path", params[common.KeyModelPath])
		model, err := models.NewModelFromFile(params[common.KeyModelPath])
		if err != nil {
			return nil, err // Wrapped by models.NewModelFromFile
		}
		p.tree, p.instance2node = model.ToGraph()
	}
	return p, nil
}

func (p *testProvider) GetCredentials(_ map[string]string) (interface{}, error) {
	return nil, nil
}

func (p *testProvider) GetComputeInstances(_ context.Context, _ common.Engine) ([]common.ComputeInstances, error) {
	return []common.ComputeInstances{{Instances: p.instance2node}}, nil
}

func (p *testProvider) GenerateTopologyConfig(_ context.Context, _ interface{}, _ int, _ []common.ComputeInstances) (*common.Vertex, error) {
	return p.tree, nil
}
