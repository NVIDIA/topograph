/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package dsx

import "context"

// TopologyResponse is the DSX GET …/topology/…/nodes JSON body.
type TopologyResponse struct {
	Switches map[string]SwitchInfo `json:"switches"`
}

// SwitchInfo describes one fabric switch and its children (switches and/or nodes).
type SwitchInfo struct {
	Switches []string   `json:"switches,omitempty"`
	Nodes    []NodeInfo `json:"nodes,omitempty"`
}

// NodeInfo is one compute attachment at a leaf.
type NodeInfo struct {
	NodeID               string `json:"node_id"`
	AcceleratedNetworkID string `json:"accelerated_network_id,omitempty"`
}

type ClientFactory func() (Client, error)

// Client retrieves topology from REST API endpoints.
type Client interface {
	GetTopology(ctx context.Context, vpcID string, nodeIDs []string, pageSize int, pageToken string) (*TopologyResponse, error)
}
