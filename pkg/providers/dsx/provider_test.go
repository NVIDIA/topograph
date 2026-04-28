/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package dsx

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/providers"
)

func TestLoaderMissingBaseURL(t *testing.T) {
	_, err := Loader(context.Background(), providers.Config{
		Params: map[string]any{"bearerToken": "x"},
		Creds:  map[string]any{},
	})
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.Code())
}

func TestLoaderMissingBearerToken(t *testing.T) {
	_, err := Loader(context.Background(), providers.Config{
		Params: map[string]any{"baseUrl": "x"},
		Creds:  map[string]any{},
	})
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.Code())
}

func TestLoaderMissingVpcId(t *testing.T) {
	_, err := Loader(context.Background(), providers.Config{
		Params: map[string]any{"baseUrl": "x", "bearerToken": "x"},
		Creds:  map[string]any{},
	})
	require.Nil(t, err)
}

func TestLoaderInvalidTrimTiers(t *testing.T) {
	_, err := Loader(context.Background(), providers.Config{
		Params: map[string]any{"baseUrl": "x", "bearerToken": "x", "vpcId": "x", "trimTiers": "invalid"},
		Creds:  map[string]any{},
	})
	require.NotNil(t, err)
	require.Equal(t, "invalid 'trimTiers' value 'invalid': unsupported type string", err.Error())
}

func TestLoaderWrongTrimTiers(t *testing.T) {
	_, err := Loader(context.Background(), providers.Config{
		Params: map[string]any{"baseUrl": "x", "bearerToken": "x", "vpcId": "x", "trimTiers": 6},
		Creds:  map[string]any{},
	})
	require.NotNil(t, err)
	require.Equal(t, "invalid 'trimTiers' value '6': must be an integer between 0 and 2", err.Error())
}

func TestSlurmInstanceMapper(t *testing.T) {
	ctx := context.Background()
	p := &Provider{}
	nodes := []string{"n1", "n2"}

	i2n, err := p.Instances2NodeMap(ctx, nodes)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"n1": "n1", "n2": "n2"}, i2n)

	regions, err := p.GetInstancesRegions(ctx, nodes)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"n1": "", "n2": ""}, regions)
}
