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

package providers

import (
	"fmt"

	"github.com/NVIDIA/topograph/internal/config"
)

type SimulationParams struct {
	ModelPath string `mapstructure:"model_path"`
}

func GetSimulationParams(params map[string]any) (*SimulationParams, error) {
	var p SimulationParams
	if err := config.Decode(params, &p); err != nil {
		return nil, fmt.Errorf("error decoding params: %w", err)
	}
	if len(p.ModelPath) == 0 {
		return nil, fmt.Errorf("no model path for simulation")
	}

	return &p, nil
}
