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
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
)

type Provider struct{}

type Credentials struct {
	AccessKeyId     string
	SecretAccessKey string
	Token           string // token is optional
}

func GetProvider() (*Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) GetCredentials(creds map[string]string) (interface{}, error) {
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
		nodeCreds, err := GetCredentials()
		if err != nil {
			return nil, err
		}
		accessKeyID = nodeCreds.AccessKeyId
		secretAccessKey = nodeCreds.SecretAccessKey
		sessionToken = nodeCreds.Token
	}

	return &Credentials{
		AccessKeyId:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		Token:           sessionToken,
	}, nil
}

func (p *Provider) GetComputeInstances(ctx context.Context, engine common.Engine) ([]common.ComputeInstances, error) {
	klog.InfoS("Getting compute instances", "provider", common.ProviderAWS, "engine", engine)

	switch eng := engine.(type) {
	case *slurm.SlurmEngine:
		nodes, err := slurm.GetNodeList(ctx)
		if err != nil {
			return nil, err
		}
		i2n, err := Instance2NodeMap(ctx, nodes)
		if err != nil {
			return nil, err
		}
		region, err := GetRegion()
		if err != nil {
			return nil, err
		}
		return []common.ComputeInstances{{Region: region, Instances: i2n}}, nil
	case *k8s.K8sEngine:
		return eng.GetComputeInstances(ctx,
			func(n *v1.Node) string { return n.Labels["topology.kubernetes.io/region"] },
			func(n *v1.Node) string {
				// ProviderID format: "aws:///us-east-1f/i-0acd9257c6569d371"
				parts := strings.Split(n.Spec.ProviderID, "/")
				return parts[len(parts)-1]
			})
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, cr interface{}, pageSize int, instances []common.ComputeInstances) (*common.Vertex, error) {
	creds := cr.(*Credentials)
	topology, err := GenerateInstanceTopology(ctx, creds, int32(pageSize), instances)
	if err != nil {
		return nil, err
	}

	klog.Infof("Extracted topology for %d instances", len(topology))

	return toGraph(topology, instances)
}
