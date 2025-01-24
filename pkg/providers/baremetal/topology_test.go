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

package baremetal

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClique(t *testing.T) {
	cliqueOutput := `node-10:         CliqueId                          : 4000000004
	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-10:         CliqueId                          : 4000000004
	node-10:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-07:         CliqueId                          : 4000000005
	node-07:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-07:         CliqueId                          : 4000000005
	node-07:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-08:         CliqueId                          : 4000000005
	node-08:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-08:         CliqueId                          : 4000000005
	node-08:         ClusterUUID                       : 50000000-0000-0000-0000-000000000004
	node-09:         CliqueId                          : 4000000005
	node-09:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005
	node-09:         CliqueId                          : 4000000005
	node-09:         ClusterUUID                       : 50000000-0000-0000-0000-000000000005`

	domainObj45 := domain{
		nodeMap: map[string]bool{
			"node-07": true,
			"node-08": true,
		},
	}

	domainObj54 := domain{
		nodeMap: map[string]bool{
			"node-10": true,
		},
	}

	domainObj55 := domain{
		nodeMap: map[string]bool{
			"node-09": true,
		},
	}

	expectedDomainMap := map[string]domain{
		"50000000-0000-0000-0000-0000000000044000000005": domainObj45,
		"50000000-0000-0000-0000-0000000000054000000004": domainObj54,
		"50000000-0000-0000-0000-0000000000054000000005": domainObj55,
	}

	domainMap, err := populateDomains(bytes.NewBufferString(cliqueOutput))
	require.NoError(t, err)
	require.Equal(t, expectedDomainMap, domainMap)
}

func TestSlurmPartition(t *testing.T) {
	partitions := `cq          up    6:00:00      1  down* node2-14
	cq          up    6:00:00      1  drain node1-01
	cq          up    6:00:00     30   idle node1-[02-16],node2-[01-13,15-16]
	c1q        up    8:00:00      1  drain node1-01
	c1q        up    8:00:00     15   idle node1-[02-16]
	c2q       up    8:00:00      1  down* node2-14
	c2q       up    8:00:00     15   idle node2-[01-13,15-16]`

	expectedPartitionMap := map[string][]string{
		"cq":  {"node2-14", "node1-01", "node1-02", "node1-03", "node1-04", "node1-05", "node1-06", "node1-07", "node1-08", "node1-09", "node1-10", "node1-11", "node1-12", "node1-13", "node1-14", "node1-15", "node1-16", "node2-01", "node2-02", "node2-03", "node2-04", "node2-05", "node2-06", "node2-07", "node2-08", "node2-09", "node2-10", "node2-11", "node2-12", "node2-13", "node2-15", "node2-16"},
		"c1q": {"node1-01", "node1-02", "node1-03", "node1-04", "node1-05", "node1-06", "node1-07", "node1-08", "node1-09", "node1-10", "node1-11", "node1-12", "node1-13", "node1-14", "node1-15", "node1-16"},
		"c2q": {"node2-14", "node2-01", "node2-02", "node2-03", "node2-04", "node2-05", "node2-06", "node2-07", "node2-08", "node2-09", "node2-10", "node2-11", "node2-12", "node2-13", "node2-15", "node2-16"},
	}

	partitionMap, err := populatePartitions(bytes.NewBufferString(partitions))
	require.NoError(t, err)
	require.Equal(t, expectedPartitionMap, partitionMap)
}
