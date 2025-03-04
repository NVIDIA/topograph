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

	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NAME_SIM = "aws-sim"

	AvailabilityZoneKey = "availability_zone"
	GroupNameKey        = "group"

	DEFAULT_MAX_RESULTS = 20
)

type simClient struct {
	Model       *models.Model
	Outputs     map[string]([]types.InstanceTopology)
	NextTokens  map[string]string
	InstanceIds []string
}

func (client *simClient) DescribeInstanceTopology(ctx context.Context, params *ec2.DescribeInstanceTopologyInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTopologyOutput, error) {
	// If we need to calculate new results (a previous token was not given)
	var instanceIds []string
	if len(params.InstanceIds) != 0 {
		instanceIds = params.InstanceIds
	} else {
		instanceIds = client.InstanceIds
	}

	givenToken := params.NextToken
	if givenToken == nil {
		// Refreshes the clients internal storage for outputs
		client.Outputs = make(map[string][]types.InstanceTopology)
		client.NextTokens = make(map[string]string)

		// Sets the maximum number of results to return per output
		var maxResults int = DEFAULT_MAX_RESULTS
		if params.MaxResults != nil {
			maxResults = int(*params.MaxResults)
		}

		// Creates the list of instances whose topology is requested
		var firstToken string
		var instanceIdx int = 0
		for instanceIdx < len(instanceIds) {
			// Only collect a list up to params.MaxResults
			var instances []types.InstanceTopology
			var i int
			for i = 0; i < maxResults && i+instanceIdx < len(instanceIds); i++ {
				// Gets the instance ID
				instanceId := instanceIds[instanceIdx+i]

				// Gets the availability zone and placement group of the instance
				node, ok := client.Model.Nodes[instanceId]
				if !ok {
					continue
				}
				var az, pg string
				if len(node.Metadata) != 0 {
					az = node.Metadata[AvailabilityZoneKey]
					pg = node.Metadata[GroupNameKey]
				}
				if len(az) == 0 {
					return nil, fmt.Errorf("availability zone not found for instance %q in AWS simulation", instanceId)
				}
				if len(pg) == 0 {
					return nil, fmt.Errorf("placement group not found for instance %q in AWS simulation", instanceId)
				}

				// Sets up the structure for the instance
				var netLayers []string
				for j := len(node.NetLayers) - 1; j >= 0; j-- {
					netLayers = append(netLayers, node.NetLayers[j])
				}
				instTopo := types.InstanceTopology{
					InstanceId:       &instanceId,
					InstanceType:     &node.Type,
					AvailabilityZone: &az,
					ZoneId:           &az,
					GroupName:        &pg,
					CapacityBlockId:  &node.NVLink,
					NetworkNodes:     netLayers,
				}
				instances = append(instances, instTopo)
			}

			token := strconv.Itoa(instanceIdx)
			if instanceIdx == 0 {
				firstToken = token
			}
			client.Outputs[token] = instances
			instanceIdx += i
			if instanceIdx < len(instanceIds) {
				var nextToken string = strconv.Itoa(instanceIdx)
				client.NextTokens[token] = nextToken
			}
		}

		// Sets the given token to the first token generated, then proceed normally
		givenToken = &firstToken
	}

	// Otherwise return the requested, already calculated output
	output := ec2.DescribeInstanceTopologyOutput{
		Instances: client.Outputs[*givenToken],
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

	p, err := providers.GetSimulationParams(cfg.Params)
	if err != nil {
		return nil, err
	}

	model, err := models.NewModelFromFile(p.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model file for AWS simulation: %v", err)
	}

	sim := &simClient{
		Model:       model,
		InstanceIds: make([]string, 0, len(model.Nodes)),
	}
	for _, node := range model.Nodes {
		sim.InstanceIds = append(sim.InstanceIds, node.Name)
	}

	client := &Client{EC2: sim}

	clientFactory := func(region string) (*Client, error) {
		return client, nil
	}

	return NewSim(clientFactory, imdsClient), nil
}

type simProvider struct {
	baseProvider
}

func NewSim(clientFactory ClientFactory, imdsClient IDMSClient) *simProvider {
	return &simProvider{
		baseProvider: baseProvider{
			clientFactory: clientFactory,
			imdsClient:    imdsClient,
		},
	}
}

// Engine support

func (p *simProvider) GetComputeInstances(ctx context.Context) ([]topology.ComputeInstances, error) {
	client, _ := p.clientFactory("")

	return client.EC2.(*simClient).Model.Instances, nil
}
