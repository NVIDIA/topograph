/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nebius

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	compute "github.com/nebius/gosdk/proto/nebius/compute/v1"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pageSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, *httperr.Error) {
	client, err := p.clientFactory(pageSize)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to create API client: %v", err))
	}

	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		if err := p.generateRegionInstanceTopology(ctx, client, topo, &ci); err != nil {
			return nil, err
		}
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, client Client, topo *topology.ClusterTopology, ci *topology.ComputeInstances) *httperr.Error {
	if len(ci.Region) == 0 {
		return httperr.NewError(http.StatusBadRequest, "must specify region")
	}
	klog.InfoS("Getting instance topology", "region", ci.Region)

	req := &compute.ListInstancesRequest{
		ParentId: client.ProjectID(),
		PageSize: client.PageSize(),
	}

	for {
		resp, err := client.GetComputeInstanceList(ctx, req)
		if err != nil {
			return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get instance list: %v", err))
		}

		for _, instance := range resp.Items {
			hostname, intf, ok := hasNetIntf(ci, instance.GetStatus().GetNetworkInterfaces())
			if !ok {
				klog.V(4).Infof("Skipping instance %s", instance.String())
				continue
			}
			ibTopology := instance.GetStatus().GetInfinibandTopologyPath()
			if ibTopology == nil {
				klog.Warningf("missing topology path for node %q", hostname)
				continue
			}

			inst := &topology.InstanceTopology{
				InstanceID: intf,
			}

			path := ibTopology.GetPath()
			switch len(path) {
			case 3:
				inst.CoreID = path[0]
				inst.SpineID = path[1]
				inst.LeafID = path[2]
			default:
				klog.Warningf("unsupported size %d of topology path for node %q", len(path), hostname)
				continue
			}

			klog.Infof("Adding topology: %s", inst.String())
			topo.Append(inst)
		}

		if len(resp.NextPageToken) == 0 {
			klog.V(4).Infof("Total processed nodes: %d", topo.Len())
			return nil
		}
		req.PageToken = resp.NextPageToken
	}
}

func hasNetIntf(ci *topology.ComputeInstances, nw []*compute.NetworkInterfaceStatus) (string, string, bool) {
	for _, status := range nw {
		if hostname, ok := ci.Instances[strings.ToUpper(status.MacAddress)]; ok {
			return hostname, status.MacAddress, true
		}
	}

	return "", "", false
}
