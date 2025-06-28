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
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "aws"

type baseProvider struct {
	clientFactory ClientFactory
	imdsClient    IMDSClient
}

type EC2Client interface {
	DescribeInstanceTopology(ctx context.Context, params *ec2.DescribeInstanceTopologyInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTopologyOutput, error)
}

type IMDSClient interface {
	GetRegion(ctx context.Context, params *imds.GetRegionInput, optFns ...func(*imds.Options)) (*imds.GetRegionOutput, error)
}

type CredsClient interface {
	Retrieve(ctx context.Context) (aws.Credentials, error)
}

type ClientFactory func(region string, pageSize *int) (*Client, error)

type Client struct {
	ec2      EC2Client
	pageSize int32
}

func (c *Client) PageSize() *int32 {
	return &c.pageSize
}

type Credentials struct {
	AccessKeyId     string
	SecretAccessKey string
	Token           string // Token is optional
}

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, cfg providers.Config) (providers.Provider, error) {
	defaultCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	imdsClient := imds.NewFromConfig(defaultCfg)

	creds, err := getCredentials(ctx, cfg.Creds)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %v", err)
	}

	clientFactory := func(region string, pageSize *int) (*Client, error) {
		opts := []func(*config.LoadOptions) error{
			config.WithRegion(region),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(creds.AccessKeyId, creds.SecretAccessKey, creds.Token),
			)}

		awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to load SDK config: %v", err)
		}

		ec2Client := ec2.NewFromConfig(awsCfg)

		return &Client{
			ec2:      ec2Client,
			pageSize: setPageSize(pageSize),
		}, nil
	}

	return New(clientFactory, imdsClient), nil
}

func getCredentials(ctx context.Context, creds map[string]string) (*Credentials, error) {
	var accessKeyID, secretAccessKey, sessionToken string

	if len(creds) != 0 {
		klog.Infof("Using provided AWS credentials")
		if accessKeyID = creds["access_key_id"]; len(accessKeyID) == 0 {
			return nil, fmt.Errorf("credentials error: missing access_key_id")
		}
		if secretAccessKey = creds["secret_access_key"]; len(secretAccessKey) == 0 {
			return nil, fmt.Errorf("credentials error: missing secret_access_key")
		}
		sessionToken = creds["token"]
	} else if len(os.Getenv("AWS_ACCESS_KEY_ID")) != 0 && len(os.Getenv("AWS_SECRET_ACCESS_KEY")) != 0 {
		klog.Infof("Using shell AWS credentials")
		accessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
		secretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		sessionToken = os.Getenv("AWS_SESSION_TOKEN")
	} else {
		klog.Infof("Using node AWS access credentials")
		creds, err := getCredentialsFromProvider(ctx)
		if err != nil {
			return nil, err
		}
		accessKeyID = creds.AccessKeyID
		secretAccessKey = creds.SecretAccessKey
		sessionToken = creds.SessionToken
	}

	return &Credentials{
		AccessKeyId:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Token:           sessionToken,
	}, nil
}

func getCredentialsFromProvider(ctx context.Context) (creds aws.Credentials, err error) {
	credsClient := ec2rolecreds.New()

	for {
		creds, err = credsClient.Retrieve(ctx)
		if err != nil {
			return creds, err
		}

		if time.Now().Add(tokenTimeDelay).After(creds.Expires) {
			klog.V(4).Infof("Waiting %s for new token", tokenTimeDelay.String())
			time.Sleep(tokenTimeDelay)
			continue
		}

		return creds, nil
	}
}

func (p *baseProvider) GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	topo, err := p.generateInstanceTopology(ctx, pageSize, instances)
	if err != nil {
		return nil, err
	}

	klog.Infof("Extracted topology for %d instances", topo.Len())

	return topo.ToThreeTierGraph(NAME, instances, false)
}

type Provider struct {
	baseProvider
}

func New(clientFactory ClientFactory, imdsClient IMDSClient) *Provider {
	return &Provider{
		baseProvider: baseProvider{
			clientFactory: clientFactory,
			imdsClient:    imdsClient,
		},
	}
}

// Engine support

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	return instanceToNodeMap(ctx, nodes)
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion(ctx context.Context) (string, error) {
	output, err := p.imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		return "", err
	}
	return output.Region, nil
}
