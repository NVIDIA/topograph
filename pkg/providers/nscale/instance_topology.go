/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package nscale

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	defaultPageSize = 100
)

func (p *baseProvider) generateInstanceTopology(ctx context.Context, pSize *int, cis []topology.ComputeInstances) (*topology.ClusterTopology, *httperr.Error) {
	topo := topology.NewClusterTopology()

	pageSize := defaultPageSize
	if pSize != nil {
		pageSize = *pSize
	}

	for _, ci := range cis {
		if err := p.generateRegionInstanceTopology(ctx, topo, pageSize, &ci); err != nil {
			return nil, err
		}
	}

	return topo, nil
}

func (p *baseProvider) generateRegionInstanceTopology(ctx context.Context, topo *topology.ClusterTopology, pageSize int, ci *topology.ComputeInstances) *httperr.Error {
	if len(ci.Region) == 0 {
		return httperr.NewError(http.StatusBadRequest, "must specify region")
	}
	klog.InfoS("Getting instance topology", "region", ci.Region)

	offset := 0
	for {
		resp, err := p.client.Topology(ctx, ci.Region, pageSize, offset)
		if err != nil {
			return httperr.NewError(http.StatusBadGateway, fmt.Sprintf("failed to get topology: %v", err))
		}

		n := len(resp)
		if n == 0 {
			klog.V(4).Infof("Total processed nodes: %d", topo.Len())
			return nil
		}
		offset += n

		for _, inst := range resp {
			t := &topology.InstanceTopology{
				InstanceID: inst.ID,
			}

			for indx := range minPathSize(inst.NetworkPath) {
				switch indx {
				case 0:
					t.CoreID = inst.NetworkPath[indx]
				case 1:
					t.SpineID = inst.NetworkPath[indx]
				case 2:
					t.LeafID = inst.NetworkPath[indx]
				default:
					klog.Warningf("unsupported size %d of topology path for instance %q", len(inst.NetworkPath), inst.ID)
				}
			}

			if inst.BlockID != nil {
				t.AcceleratorID = *inst.BlockID
			}

			klog.Infof("Adding topology: %s", t.String())
			topo.Append(t)
		}
	}
}

func minPathSize(path []string) int {
	n := len(path)
	if n > 3 {
		// return one extra index to print warning
		return 4
	}
	return n
}
