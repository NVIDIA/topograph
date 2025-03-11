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

package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"

	pb "github.com/NVIDIA/topograph/pkg/protos"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func forwardRequest(ctx context.Context, tr *topology.Request, url string, cis []topology.ComputeInstances) (*topology.Vertex, error) {
	klog.Infof("Forwarding request to %s", url)
	conn, err := grpc.NewClient(url, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", url, err)
	}
	defer func() { _ = conn.Close() }()

	client := pb.NewTopologyServiceClient(conn)
	topo := topology.NewClusterTopology()

	for _, ci := range cis {
		ids := []string{}
		for id := range ci.Instances {
			ids = append(ids, id)
		}
		klog.Infof("Getting topology for instances %v", ids)

		response, err := client.DescribeTopology(ctx, &pb.TopologyRequest{
			Provider:    tr.Provider.Name,
			Region:      ci.Region,
			InstanceIds: ids,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to forward request: %v", err)
		}

		klog.V(4).Infof("Response: %s", response.String())
		for _, elem := range response.GetInstances() {
			topo.Append(convert(elem))
		}
	}

	return topo.ToThreeTierGraph(tr.Provider.Name, cis, false)
}

func convert(inst *pb.Instance) *topology.InstanceTopology {
	topo := &topology.InstanceTopology{
		InstanceID:    inst.Id,
		BlockID:       inst.NetworkLayers[0],
		SpineID:       inst.NetworkLayers[1],
		DatacenterID:  inst.NetworkLayers[2],
		AcceleratorID: inst.NvlinkDomain,
	}
	klog.V(4).Infof("Adding instance topology %s", topo.String())
	return topo
}
