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

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
)

func GetEngine(engine string) (common.Engine, *common.HTTPError) {
	var (
		eng common.Engine
		err error
	)

	switch engine {
	case common.EngineSLURM:
		eng = &slurm.SlurmEngine{}
	case common.EngineK8S:
		eng, err = k8s.GetK8sEngine()
	case common.EngineTest:
		eng = &testEngine{}
	default:
		return nil, common.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unsupported engine %q", engine))
	}

	if err != nil {
		return nil, common.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return eng, nil
}

type testEngine struct{}

func (eng *testEngine) GenerateOutput(ctx context.Context, tree *common.Vertex, params map[string]string) ([]byte, error) {
	if params == nil {
		params = make(map[string]string)
	}
	params[common.KeySkipReload] = ""
	return slurm.GenerateOutput(ctx, tree, params)
}
