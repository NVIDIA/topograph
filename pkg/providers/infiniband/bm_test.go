/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/accelerator"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestParsePdshNvidiaSMIOutput(t *testing.T) {
	output := bytes.NewBufferString(`node-1: uuid-1, 7
node-1: uuid-1, 7
malformed
node-2: uuid-2, 8
`)

	outputs, err := parsePdshNvidiaSMIOutput(output)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"node-1": "uuid-1, 7\nuuid-1, 7\n",
		"node-2": "uuid-2, 8\n",
	}, outputs)
}

func TestAcceleratorTargetsAndDomainMap(t *testing.T) {
	targets := acceleratorTargets([]topology.ComputeInstances{{
		Instances: map[string]string{
			"instance-1": "node-1",
			"instance-2": "node-2",
		},
	}})
	require.ElementsMatch(t, []accelerator.Target{
		{InstanceID: "instance-1", HostName: "node-1"},
		{InstanceID: "instance-2", HostName: "node-2"},
	}, targets)

	domainMap := domainMapFromAssignments(accelerator.Assignments{
		"instance-1": {DomainID: "domain-1", SubDomainID: "partition-1"},
	}, targets)
	require.Equal(t, topology.DomainMap{
		"domain-1": {
			"node-1": {
				Domain:     "domain-1",
				SubDomain:  "partition-1",
				InstanceID: "instance-1",
				HostName:   "node-1",
			},
		},
	}, domainMap)
}
