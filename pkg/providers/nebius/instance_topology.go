/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"

	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func (p *baseProvider) generateInstanceTopology(ctx context.Context, cis []topology.ComputeInstances) (*topology.ClusterTopology, error) {
	client, err := p.clientFactory()
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %v", err)
	}

	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := p.generateRegionInstanceTopology(ctx, client, topo, &ci); err != nil {
			return nil, fmt.Errorf("failed to get instance topology: %v", err)
		}
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, client Client, topo *topology.ClusterTopology, ci *topology.ComputeInstances) error {
	if len(ci.Region) == 0 {
		return fmt.Errorf("must specify region")
	}
	klog.InfoS("Getting instance topology", "region", ci.Region)

	for id, hostname := range ci.Instances {
		req := &compute.GetInstanceRequest{Id: id}
		instance, err := client.GetComputeInstance(ctx, req)
		if err != nil {
			return fmt.Errorf("error in getting compute instance: id:%s hostname:%s err:%v", id, hostname, err)
		}

		ibTopology := instance.GetStatus().GetInfinibandTopologyPath()
		if ibTopology == nil {
			klog.Warningf("missing topology for node %q id %q", hostname, id)
			continue
		}

		inst := &topology.InstanceTopology{
			InstanceID: id,
		}

		path := ibTopology.GetPath()
		switch len(path) {
		case 3:
			inst.DatacenterID = path[0]
			inst.SpineID = path[1]
			inst.BlockID = path[2]
		default:
			klog.Warningf("unsupported size %d of topology path for node %q id %q", len(path), hostname, id)
			continue
		}

		klog.Infof("Adding topology: %s", inst.String())
		topo.Append(inst)
	}
	return nil
}
