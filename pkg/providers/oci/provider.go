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

package oci

import (
	"context"
	"fmt"

	OCICommon "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/engines/k8s"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
)

type Provider struct{}

func GetProvider() (*Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) GetCredentials(creds map[string]string) (interface{}, error) {
	if len(creds) != 0 {
		var tenancyID, userID, region, fingerprint, privateKey, passphrase string
		klog.Info("Using provided credentials")
		if tenancyID = creds["tenancy_id"]; len(tenancyID) == 0 {
			return nil, fmt.Errorf("credentials error: missing tenancy_id")
		}
		if userID = creds["user_id"]; len(userID) == 0 {
			return nil, fmt.Errorf("credentials error: missing user_id")
		}
		if region = creds["region"]; len(region) == 0 {
			return nil, fmt.Errorf("credentials error: missing region")
		}
		if fingerprint = creds["fingerprint"]; len(fingerprint) == 0 {
			return nil, fmt.Errorf("credentials error: missing fingerprint")
		}
		if privateKey = creds["private_key"]; len(privateKey) == 0 {
			return nil, fmt.Errorf("credentials error: missing private_key")
		}
		passphrase = creds["passphrase"]

		return OCICommon.NewRawConfigurationProvider(tenancyID, userID, region, fingerprint, privateKey, &passphrase), nil
	}

	klog.Info("No credentials provided, trying default configuration provider")
	configProvider := OCICommon.DefaultConfigProvider()
	_, err := configProvider.AuthType()
	if err == nil {
		return configProvider, nil
	}

	klog.Infof("No default configuration provider found: %v; trying instance principal configuration provider", err)
	configProvider, err = auth.InstancePrincipalConfigurationProvider()

	if err != nil {
		return nil, fmt.Errorf("unable to authenticate API: %s", err.Error())
	}

	return configProvider, nil
}

func (p *Provider) GetComputeInstances(ctx context.Context, engine common.Engine) ([]common.ComputeInstances, error) {
	klog.InfoS("Getting compute instances", "provider", common.ProviderOCI, "engine", engine)

	switch eng := engine.(type) {
	case *slurm.SlurmEngine:
		nodes, err := slurm.GetNodeList(ctx)
		if err != nil {
			return nil, err
		}
		i2n, err := instanceToNodeMap(nodes)
		if err != nil {
			return nil, err
		}
		return []common.ComputeInstances{{Instances: i2n}}, nil

	case *k8s.K8sEngine:
		return eng.GetComputeInstances(ctx,
			func(n *v1.Node) string { return n.Labels["topology.kubernetes.io/region"] },
			func(n *v1.Node) string { return n.Spec.ProviderID })
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, cr interface{}, _ int, instances []common.ComputeInstances) (*common.Vertex, error) {
	creds := cr.(OCICommon.ConfigurationProvider)
	cfg, err := GenerateInstanceTopology(ctx, creds, instances)
	if err != nil {
		return nil, err
	}

	return toGraph(cfg, instances)
}
