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

package toposim

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/models"
	pb "github.com/NVIDIA/topograph/pkg/protos"
)

type Server struct {
	pb.UnimplementedTopologyServiceServer

	model  *models.Model
	port   int
	server *grpc.Server
}

func NewServer(model *models.Model, port int) *Server {
	return &Server{
		model: model,
		port:  port,
	}
}

func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen to port %d: %v", s.port, err)
	}

	s.server = grpc.NewServer()

	pb.RegisterTopologyServiceServer(s.server, s)

	klog.Infof("Starting gRPC server at %v", lis.Addr())
	return s.server.Serve(lis)
}

func (s *Server) Stop(err error) {
	klog.Infof("Stopping gRPC server")
	s.server.GracefulStop()
}

func (s *Server) DescribeTopology(ctx context.Context, in *pb.TopologyRequest) (*pb.TopologyResponse, error) {
	klog.InfoS("DescribeTopology", "provider", in.GetProvider(), "region", in.GetRegion(), "#instances", len(in.GetInstanceIds()))

	res := &pb.TopologyResponse{
		Instances: make([]*pb.Instance, 0, len(in.InstanceIds)),
	}

	for _, instance := range in.InstanceIds {
		node, ok := s.model.Nodes[instance]
		if !ok {
			klog.Warningf("Missing instance %s", instance)
			continue
		}
		res.Instances = append(res.Instances, &pb.Instance{
			Id:            node.Name,
			InstanceType:  node.Type,
			Provider:      in.Provider,
			Region:        in.Region,
			NetworkLayers: node.NetLayers,
			NvlinkDomain:  node.NVLink,
		})
	}

	return res, nil
}
