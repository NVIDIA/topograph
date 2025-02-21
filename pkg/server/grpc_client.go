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

	"github.com/NVIDIA/topograph/pkg/metrics"
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

	ids := []string{}
	for _, ci := range cis {
		for id := range ci.Instances {
			ids = append(ids, id)
		}
	}

	klog.Infof("Getting topology for instances %v", ids)

	response, err := client.DescribeTopology(ctx, &pb.TopologyRequest{
		Provider:    tr.Provider.Name,
		Region:      "",
		InstanceIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to forward request: %v", err)
	}

	klog.V(4).Infof("Response: %s", response.String())

	return toGraph(response, cis, getTopologyFormat(tr.Engine.Params)), nil
}

// getTopologyFormat derives topology format from engine parameters: tree (default) or block
func getTopologyFormat(params map[string]any) string {
	if len(params) != 0 {
		if formatI, ok := params[topology.KeyPlugin]; ok {
			if format, ok := formatI.(string); ok && len(format) != 0 {
				return format
			}
		}
	}
	return topology.TopologyTree
}

// TODO: replace with translate.ToThreeTierGraph()
func toGraph(response *pb.TopologyResponse, cis []topology.ComputeInstances, format string) *topology.Vertex {
	i2n := make(map[string]string)
	for _, ci := range cis {
		for instance, node := range ci.Instances {
			i2n[instance] = node
		}
	}
	klog.V(4).Infof("Instance/Node map %v", i2n)

	forest := make(map[string]*topology.Vertex)
	blocks := make(map[string]*topology.Vertex)
	vertices := make(map[string]*topology.Vertex)

	for _, ins := range response.Instances {
		nodeName, ok := i2n[ins.Id]
		if !ok {
			klog.V(5).Infof("Instance ID %q not found", ins.Id)
			continue
		}

		klog.V(4).Infof("Found node %q instance %q", nodeName, ins.Id)
		delete(i2n, ins.Id)

		vertex := &topology.Vertex{
			Name: nodeName,
			ID:   ins.Id,
		}
		id := ins.Id

		// check for NVLink and add to the forest
		if len(ins.NvlinkDomain) != 0 {
			klog.V(4).Infof("Adding node %q to NVLink domain %q", nodeName, ins.NvlinkDomain)
			switchName := fmt.Sprintf("nvlink-%s", ins.NvlinkDomain)
			sw, ok := blocks[switchName]
			if !ok {
				sw = &topology.Vertex{
					ID:       switchName,
					Vertices: map[string]*topology.Vertex{id: vertex},
				}
				blocks[switchName] = sw
			} else {
				sw.Vertices[id] = vertex
			}
		}

		// iterate over network layers and construct tree path
		for _, net := range ins.NetworkLayers {
			// remove internal vertex from the forest roots
			delete(forest, id)

			// create or reuse vertex
			sw, ok := vertices[net]
			if !ok {
				sw = &topology.Vertex{
					ID:       net,
					Vertices: map[string]*topology.Vertex{id: vertex},
				}
				vertices[net] = sw
			} else {
				sw.Vertices[id] = vertex
			}
			vertex = sw
			id = net
		}
		// add the top of the tree path to forest, unless it is a leaf
		if id != ins.Id {
			if _, ok := forest[id]; !ok {
				forest[id] = vertex
			}
		}
	}

	if len(i2n) != 0 {
		klog.V(4).Infof("Adding nodes w/o topology: %v", i2n)
		sw := &topology.Vertex{
			ID:       topology.NoTopology,
			Vertices: make(map[string]*topology.Vertex),
		}
		for instanceID, nodeName := range i2n {
			sw.Vertices[instanceID] = &topology.Vertex{
				Name: nodeName,
				ID:   instanceID,
			}
			metrics.SetMissingTopology("GTS", nodeName)
		}
		forest[topology.NoTopology] = sw
	}

	treeRoot := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	for name, node := range forest {
		treeRoot.Vertices[name] = node
	}

	root := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	if format == topology.TopologyBlock {
		blockRoot := &topology.Vertex{
			Vertices: make(map[string]*topology.Vertex),
		}
		for name, domain := range blocks {
			blockRoot.Vertices[name] = domain
		}
		root.Vertices[topology.TopologyBlock] = blockRoot
	}
	root.Vertices[topology.TopologyTree] = treeRoot
	return root

}
