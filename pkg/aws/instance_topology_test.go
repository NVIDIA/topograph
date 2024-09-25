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

package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/common"
)

func ptrString(s string) *string {
	return &s
}

func TestNewInstanceTopology(t *testing.T) {

	topology := []types.InstanceTopology{
		{
			InstanceId:   ptrString("i-0febfe7a633a552cc"),
			InstanceType: ptrString("p5.48xlarge"),
			NetworkNodes: []string{
				"nn-098f9e7674016cb1c",
				"nn-224a2a4d9df61a975",
				"nn-20da390f7d602f42f",
			},
			AvailabilityZone: ptrString("us-east-1e"),
			ZoneId:           ptrString("use1-az3"),
		},
		{
			InstanceId:   ptrString("i-0727864293842c5f1"),
			InstanceType: ptrString("p5.48xlarge"),
			NetworkNodes: []string{
				"nn-098f9e7674016cb1c",
				"nn-224a2a4d9df61a975",
				"nn-568b52163b3ce19c8",
			},
			AvailabilityZone: ptrString("us-east-1e"),
			ZoneId:           ptrString("use1-az3"),
		},
		{
			InstanceId:   ptrString("i-04e4ca4199532bbba"),
			InstanceType: ptrString("p5.48xlarge"),
			NetworkNodes: []string{
				"nn-098f9e7674016cb1c",
				"nn-224a2a4d9df61a975",
				"nn-d7d7a965aec389018",
			},
			AvailabilityZone: ptrString("us-east-1e"),
			ZoneId:           ptrString("use1-az3"),
		},
		{
			InstanceId:   ptrString("i-0359d6503bf895535"),
			InstanceType: ptrString("p5.48xlarge"),
			NetworkNodes: []string{
				"nn-098f9e7674016cb1c",
				"nn-224a2a4d9df61a975",
				"nn-ef5c999131844763a",
			},
			AvailabilityZone: ptrString("us-east-1e"),
			ZoneId:           ptrString("use1-az3"),
		},
	}

	i2n := map[string]string{
		"i-0febfe7a633a552cc": "node1",
		"i-0727864293842c5f1": "node2",
		"i-04e4ca4199532bbba": "node3",
		"i-0359d6503bf895535": "node4",
	}

	n1 := &common.Vertex{ID: "i-0febfe7a633a552cc", Name: "node1"}
	n2 := &common.Vertex{ID: "i-0727864293842c5f1", Name: "node2"}
	n3 := &common.Vertex{ID: "i-04e4ca4199532bbba", Name: "node3"}
	n4 := &common.Vertex{ID: "i-0359d6503bf895535", Name: "node4"}

	v31 := &common.Vertex{ID: "nn-20da390f7d602f42f", Vertices: map[string]*common.Vertex{"i-0febfe7a633a552cc": n1}}
	v32 := &common.Vertex{ID: "nn-568b52163b3ce19c8", Vertices: map[string]*common.Vertex{"i-0727864293842c5f1": n2}}
	v33 := &common.Vertex{ID: "nn-d7d7a965aec389018", Vertices: map[string]*common.Vertex{"i-04e4ca4199532bbba": n3}}
	v34 := &common.Vertex{ID: "nn-ef5c999131844763a", Vertices: map[string]*common.Vertex{"i-0359d6503bf895535": n4}}

	v2 := &common.Vertex{
		ID: "nn-224a2a4d9df61a975",
		Vertices: map[string]*common.Vertex{
			"nn-20da390f7d602f42f": v31,
			"nn-568b52163b3ce19c8": v32,
			"nn-d7d7a965aec389018": v33,
			"nn-ef5c999131844763a": v34,
		},
	}

	v1 := &common.Vertex{ID: "nn-098f9e7674016cb1c", Vertices: map[string]*common.Vertex{"nn-224a2a4d9df61a975": v2}}

	expected := &common.Vertex{Vertices: map[string]*common.Vertex{"nn-098f9e7674016cb1c": v1}}

	tree, err := toGraph(topology, []common.ComputeInstances{{Instances: i2n}})
	require.NoError(t, err)
	require.Equal(t, expected, tree)
}
