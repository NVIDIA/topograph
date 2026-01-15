/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package lambdai

import (
	"context"
	"fmt"
	"net/http"

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

	req := &InstanceListRequest{PageSize: client.PageSize()}

	for {
		resp, err := client.InstanceList(ctx, req)
		if err != nil {
			return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get instance list: %v", err))
		}

		for _, inst := range resp.Items {
			t := &topology.InstanceTopology{
				InstanceID: inst.ID,
			}

			for indx := range len(inst.NetworkPath) {
				switch indx {
				case 0:
					t.BlockID = inst.NetworkPath[indx]
				case 1:
					t.SpineID = inst.NetworkPath[indx]
				case 2:
					t.DatacenterID = inst.NetworkPath[indx]
				default:
					klog.Warningf("unsupported size %d of topology path for instance %q", len(inst.NetworkPath), inst.ID)
				}
			}

			if inst.NVLink != nil {
				if len(inst.NVLink.DomainID) == 0 || len(inst.NVLink.CliqueID) == 0 {
					klog.Warningf("incomplete NVL data for instance %s: DomainID=%q CliqueID=%q", inst.ID, inst.NVLink.DomainID, inst.NVLink.CliqueID)
				} else {
					t.AcceleratorID = inst.NVLink.DomainID + "." + inst.NVLink.CliqueID
				}
			}

			klog.Infof("Adding topology: %s", t.String())
			topo.Append(t)
		}

		if len(resp.NextPageToken) == 0 {
			klog.V(4).Infof("Total processed nodes: %d", topo.Len())
			return nil
		}
		req.PageToken = resp.NextPageToken
	}
}
