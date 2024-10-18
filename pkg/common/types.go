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
	GetCredentials(*Credentials) (interface{}, error)
	GetComputeInstances(context.Context, Engine) ([]ComputeInstances, error)
	GenerateTopologyConfig(context.Context, interface{}, int, []ComputeInstances) (*Vertex, error)
}

type Engine interface {
	GenerateOutput(context.Context, *Vertex, map[string]string) ([]byte, error)
}

type Payload struct {
	Nodes []ComputeInstances `json:"nodes"`
	Creds *Credentials       `json:"creds,omitempty"` // access credentials
}

type ComputeInstances struct {
	Region    string            `json:"region"`
	Instances map[string]string `json:"instances"` // <instance ID>:<node name> map
}

type Credentials struct {
	AWS *AWSCredentials `yaml:"aws,omitempty" json:"aws,omitempty"` // AWS credentials
	OCI *OCICredentials `yaml:"oci,omitempty" json:"oci,omitempty"` // OCI credentials
}

type AWSCredentials struct {
	AccessKeyId     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	Token           string `yaml:"token,omitempty" json:"token,omitempty"` // token is optional
}

type OCICredentials struct {
	TenancyID   string `yaml:"tenancy_id" json:"tenancy_id"`
	UserID      string `yaml:"user_id" json:"user_id"`
	Region      string `yaml:"region" json:"region"`
	Fingerprint string `yaml:"fingerprint" json:"fingerprint"`
	PrivateKey  string `yaml:"private_key" json:"private_key"`
	Passphrase  string `yaml:"passphrase,omitempty" json:"passphrase,omitempty"` // passphrase is optional
}

func (p *Payload) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Payload:\n  Nodes: %v\n", p.Nodes))
	if p.Creds != nil {
		sb.WriteString("  Credentials:\n")
		if p.Creds.AWS != nil {
			var accessKeyId, secretAccessKey, token string
			if len(p.Creds.AWS.AccessKeyId) != 0 {
				accessKeyId = "***"
			}
			if len(p.Creds.AWS.SecretAccessKey) != 0 {
				secretAccessKey = "***"
			}
			if len(p.Creds.AWS.Token) != 0 {
				token = "***"
			}
			sb.WriteString(fmt.Sprintf("    AWS: AccessKeyID=%s SecretAccessKey=%s SessionToken=%s\n",
				accessKeyId, secretAccessKey, token))
		}
		if p.Creds.OCI != nil {
			sb.WriteString("    OCI:\n")
			sb.WriteString(fmt.Sprintf("         UserID=%s\n", p.Creds.OCI.UserID))
			sb.WriteString(fmt.Sprintf("         TenancyID=%s\n", p.Creds.OCI.TenancyID))
			sb.WriteString(fmt.Sprintf("         Region=%s\n", p.Creds.OCI.Region))
		}
	}

	return sb.String()
}

func GetPayload(body []byte) (*Payload, error) {
	var payload Payload

	if len(body) == 0 {
		return &payload, nil
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %v", err)
	}

	if payload.Creds != nil {
		if payload.Creds.AWS != nil {
			if len(payload.Creds.AWS.AccessKeyId) == 0 || len(payload.Creds.AWS.SecretAccessKey) == 0 {
				return nil, fmt.Errorf("invalid payload: must provide access_key_id and secret_access_key for AWS")
			}
		}
	}

	return &payload, nil
}
