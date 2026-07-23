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

package aws

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

type recordingEC2Client struct {
	region string
	trace  *[]string
}

func (c *recordingEC2Client) DescribeInstanceTopology(_ context.Context, input *ec2.DescribeInstanceTopologyInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTopologyOutput, error) {
	*c.trace = append(*c.trace, "describe:"+c.region+":"+strings.Join(input.InstanceIds, ","))
	return &ec2.DescribeInstanceTopologyOutput{}, nil
}

func TestGenerateInstanceTopologyCanonicalizesProviderCalls(t *testing.T) {
	permutations := [][]topology.ComputeInstances{
		{
			{Region: "region-b", Instances: map[string]string{"i4": "node4", "i3": "node3"}},
			{Region: "region-a", Instances: map[string]string{"i2": "node2", "i1": "node1"}},
		},
		{
			{Region: "region-a", Instances: map[string]string{"i1": "node1", "i2": "node2"}},
			{Region: "region-b", Instances: map[string]string{"i3": "node3", "i4": "node4"}},
		},
	}

	for _, instances := range permutations {
		trace := []string{}
		provider := &baseProvider{
			clientFactory: func(region string, _ *int) (*Client, error) {
				trace = append(trace, "factory:"+region)
				return &Client{ec2: &recordingEC2Client{region: region, trace: &trace}}, nil
			},
		}

		_, err := provider.generateInstanceTopology(context.Background(), nil, instances)

		require.Nil(t, err)
		require.Equal(t, []string{
			"factory:region-a",
			"describe:region-a:i1,i2",
			"factory:region-b",
			"describe:region-b:i3,i4",
		}, trace)
	}
}

func TestGenerateInstanceTopologyReturnsCanonicalFirstError(t *testing.T) {
	provider := &baseProvider{
		clientFactory: func(region string, _ *int) (*Client, error) {
			return nil, errors.New(region)
		},
	}
	instances := []topology.ComputeInstances{
		{Region: "region-b", Instances: map[string]string{"i2": "node2"}},
		{Region: "region-a", Instances: map[string]string{"i1": "node1"}},
	}

	_, err := provider.generateInstanceTopology(context.Background(), nil, instances)

	require.EqualError(t, err, "failed to get client: region-a")
}
