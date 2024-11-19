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

package aws

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	int_config "github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
)

const NAME_SIM = "aws-sim"
const DEFAULT_MAX_RESULTS = 20

type SimParams struct {
	ModelPath string `mapstructure:"model_path"`
}

type SimClient struct {
	Model      *models.Model
	Outputs    map[string](*[]types.InstanceTopology)
	NextTokens map[string]string
}

func (client SimClient) DescribeInstanceTopology(ctx context.Context, params *ec2.DescribeInstanceTopologyInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTopologyOutput, error) {

	// If we need to calculate new results (a previous token was not given)
	givenToken := params.NextToken
	if givenToken == nil {
		// Gets availability zone and placement group for each instance
		instanceAzs, err := client.Model.NodeToLayerMap(models.PhysicalLayerAZ)
		if err != nil {
			return nil, err
		}
		instancePgs, err := client.Model.NodeToLayerMap(models.PhysicalLayerPG)
		if err != nil {
			return nil, err
		}

		// Refreshes the clients internal storage for outputs
		client.Outputs = make(map[string](*[]types.InstanceTopology))
		client.NextTokens = make(map[string]string)

		// Sets the maximum number of results to return per output
		var maxResults int = DEFAULT_MAX_RESULTS
		if params.MaxResults != nil {
			maxResults = int(*params.MaxResults)
		}

		// Creates the list of instances whose topology is requested
		var firstToken string
		var instanceIdx int = 0
		for instanceIdx < len(params.InstanceIds) {

			// Only collect a list up to params.MaxResults
			var instances []types.InstanceTopology
			var i int
			for i = 0; i < maxResults && i+instanceIdx < len(params.InstanceIds); i++ {

				// Gets the instance ID
				instanceId := params.InstanceIds[instanceIdx+i]

				// Gets the availability zone and placement group of the instance
				node := client.Model.Nodes[instanceId]
				az, ok := instanceAzs[instanceId]
				if !ok {
					return nil, fmt.Errorf("availability zone not found for instance %q in aws simulation", instanceId)
				}
				pg, ok := instancePgs[instanceId]
				if !ok {
					return nil, fmt.Errorf("placement group not found for instance %q in aws simulation", instanceId)
				}

				// Sets up the structure for the instance
				var instTopo types.InstanceTopology
				instTopo.InstanceId = &instanceId
				instTopo.InstanceType = &node.Type
				instTopo.AvailabilityZone = &az
				instTopo.ZoneId = &az
				instTopo.GroupName = &pg
				instTopo.CapacityBlockId = &node.CapacityBlock
				var netLayers []string
				for j := len(node.NetLayers) - 1; j >= 0; j-- {
					netLayers = append(netLayers, node.NetLayers[j])
				}
				instTopo.NetworkNodes = netLayers
				instances = append(instances, instTopo)
			}

			token := strconv.Itoa(instanceIdx)
			if instanceIdx == 0 {
				firstToken = token
			}
			client.Outputs[token] = &instances
			instanceIdx += i
			if instanceIdx < len(params.InstanceIds) {
				var nextToken string = strconv.Itoa(instanceIdx)
				client.NextTokens[token] = nextToken
			}
		}

		// Sets the given token to the first token generated, then proceed normally
		givenToken = &firstToken
	}

	// Otherwise return the requested, already calculated output
	output := ec2.DescribeInstanceTopologyOutput{
		Instances: *client.Outputs[*givenToken],
	}
	nextToken, ok := client.NextTokens[*givenToken]
	if ok {
		output.NextToken = &nextToken
	}
	return &output, nil
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

func LoaderSim(ctx context.Context, cfg providers.Config) (providers.Provider, error) {
	defaultCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	imdsClient := imds.NewFromConfig(defaultCfg)

	var p SimParams
	if err := int_config.Decode(cfg.Params, &p); err != nil {
		return nil, fmt.Errorf("error decoding params: %w", err)
	}
	if len(p.ModelPath) == 0 {
		return nil, fmt.Errorf("no model path for AWS simulation")
	}

	clientFactory := func(region string) (*Client, error) {
		csp_model, err := models.NewModelFromFile(p.ModelPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load model file for AWS simulation, %v", err)
		}
		simClient := SimClient{Model: csp_model}

		return &Client{
			EC2: simClient,
		}, nil
	}

	return New(clientFactory, imdsClient), nil
}
