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

package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDomainMapAddHost(t *testing.T) {
	domainMap := NewDomainMap()

	domainMap.AddHost("domain1", "instance1", "host1")
	domainMap.AddHost("domain1", "instance2", "host2")
	domainMap.AddHost("domain2", "instance3", "host3")
	domainMap.AddHost("", "instance4", "host4")

	require.Equal(t, DomainMap{
		"domain1": map[string]string{"host1": "instance1", "host2": "instance2"},
		"domain2": map[string]string{"host3": "instance3"},
	}, domainMap)
}
