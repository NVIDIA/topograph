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
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

// DomainMap maps domain name to a map of hostname:instance
type DomainMap map[string]map[string]string

func NewDomainMap() DomainMap {
	return make(DomainMap)
}

func (m DomainMap) AddHost(domain, instance, host string) {
	if domain == "" {
		klog.Warningf("skipping topology domain with empty name for host %q (instance %q)", host, instance)
		return
	}

	if hosts, ok := m[domain]; ok {
		hosts[host] = instance
		return
	}

	m[domain] = map[string]string{host: instance}
}

func (m DomainMap) String() string {
	var str strings.Builder
	str.WriteString("DomainMap:\n")
	for name, nodes := range m {
		fmt.Fprintf(&str, " %s : %v\n", name, nodes)
	}
	return str.String()
}
