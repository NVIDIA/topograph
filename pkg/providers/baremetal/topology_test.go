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
