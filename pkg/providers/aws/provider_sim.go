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

	// "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	// "github.com/aws/aws-sdk-go-v2/credentials"
	// "github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	// v1 "k8s.io/api/core/v1"

	// "github.com/NVIDIA/topograph/internal/exec"
	int_config "github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	// "github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_SIM = "aws-sim"

type SimParams struct {
	ModelPath string `mapstructure:"model_path"`
}

type SimClient struct {
	model *models.Model
}

func (client SimClient) DescribeInstanceTopology(ctx context.Context, params *ec2.DescribeInstanceTopologyInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTopologyOutput, error) {
	return nil, fmt.Errorf("aws simulation not yet implemented")
}

func getSimClient(m *models.Model) EC2Client {
	return SimClient{model: m}
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
		simClient := getSimClient(csp_model)

		return &Client{
			EC2: simClient,
		}, nil
	}

	return New(clientFactory, imdsClient), nil
}
