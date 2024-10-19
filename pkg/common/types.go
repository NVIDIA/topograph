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

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Vertex is a tree node, representing a compute node or a network switch, where
// - Name is a compute node name
// - ID is an CSP defined instance ID of switches and compute nodes
// - Vertices is a list of connected compute nodes or network switches
type Vertex struct {
	Name     string
	ID       string
	Vertices map[string]*Vertex
	Metadata map[string]string
}

func (v *Vertex) String() string {
	vertices := []string{}
	for _, w := range v.Vertices {
		vertices = append(vertices, w.ID)
	}
	return fmt.Sprintf("ID:%q Name:%q Vertices: %s", v.ID, v.Name, strings.Join(vertices, ","))
}

type HTTPError struct {
	Code    int
	Message string
}

func NewHTTPError(code int, msg string) *HTTPError {
	return &HTTPError{
		Code:    code,
		Message: msg,
	}
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Message)
}

type Provider interface {
	GetCredentials(map[string]string) (interface{}, error)
	GetComputeInstances(context.Context, Engine) ([]ComputeInstances, error)
	GenerateTopologyConfig(context.Context, interface{}, int, []ComputeInstances) (*Vertex, error)
}

type Engine interface {
	GenerateOutput(context.Context, *Vertex, map[string]string) ([]byte, error)
}

type TopologyRequest struct {
	Provider provider           `json:"provider"`
	Engine   engine             `json:"engine"`
	Nodes    []ComputeInstances `json:"nodes"`
}

type provider struct {
	Name  string            `json:"name"`
	Creds map[string]string `json:"creds"` // access credentials
}

type engine struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"` // access credentials
}

type ComputeInstances struct {
	Region    string            `json:"region"`
	Instances map[string]string `json:"instances"` // <instance ID>:<node name> map
}

func NewTopologyRequest(prv string, creds map[string]string, eng string, params map[string]string) *TopologyRequest {
	return &TopologyRequest{
		Provider: provider{
			Name:  prv,
			Creds: creds,
		},
		Engine: engine{
			Name:   eng,
			Params: params,
		},
	}
}

func (p *TopologyRequest) String() string {
	var sb strings.Builder
	sb.WriteString("TopologyRequest:\n")
	sb.WriteString(fmt.Sprintf("  Provider: %s\n", p.Provider.Name))
	sb.WriteString("  Credentials: ")
	for key := range p.Provider.Creds {
		sb.WriteString(fmt.Sprintf("%s=***,", key))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Engine: %s\n", p.Engine.Name))
	sb.WriteString(fmt.Sprintf("  Parameters: %v\n", p.Engine.Params))
	sb.WriteString(fmt.Sprintf("  Nodes: %s\n", p.Nodes))

	return sb.String()
}

func GetTopologyRequest(body []byte) (*TopologyRequest, error) {
	var payload TopologyRequest

	if len(body) == 0 {
		return &payload, nil
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %v", err)
	}

	return &payload, nil
}
