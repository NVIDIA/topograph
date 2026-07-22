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
	"slices"
	"strings"

	"k8s.io/klog/v2"
)

type HostInfo struct {
	Domain     string
	InstanceID string
	HostName   string
	Level1     string
	Level2     string
	Level3     string
}

// DomainMap maps accelerator domain name to host metadata.
type DomainMap map[string]map[string]*HostInfo

func NewDomainMap() DomainMap {
	return make(DomainMap)
}

func (m DomainMap) AddHost(domain, instance, host string) {
	m.AddHostInfo(&HostInfo{Domain: domain, InstanceID: instance, HostName: host})
}

func (m DomainMap) String() string {
	var str strings.Builder
	str.WriteString("DomainMap:\n")
	for name, nodes := range m {
		fmt.Fprintf(&str, " %s : %v\n", name, nodes)
	}
	return str.String()
}

func (m DomainMap) AddHostInfo(hostInfo *HostInfo) {
	if hostInfo == nil {
		return
	}
	if hostInfo.Domain == "" {
		klog.Warningf("skipping topology domain with empty name for host %q (instance %q)", hostInfo.HostName, hostInfo.InstanceID)
		return
	}

	if hosts, ok := m[hostInfo.Domain]; ok {
		hosts[hostInfo.HostName] = hostInfo
	} else {
		m[hostInfo.Domain] = map[string]*HostInfo{hostInfo.HostName: hostInfo}
	}
}

func (m DomainMap) GetLevelInfo(level int) (present bool, members map[string][]string) {
	children := make(map[string]map[string]struct{})
	for _, hosts := range m {
		for _, host := range hosts {
			var key, child string
			switch level {
			case 1:
				key = host.Level1
				child = host.Level2
			case 2:
				key = host.Level2
				child = host.Level3
			case 3:
				key = host.Level3
				child = host.Domain
			case 4:
				key = host.Domain
				child = host.HostName
			default:
				continue
			}
			if len(key) > 0 {
				if _, ok := children[key]; !ok {
					children[key] = make(map[string]struct{})
				}
				if len(child) > 0 {
					children[key][child] = struct{}{}
				}
			}
		}
	}
	if len(children) == 0 {
		return false, nil
	}
	members = make(map[string][]string, len(children))
	for key, childMap := range children {
		childList := make([]string, 0, len(childMap))
		for child := range childMap {
			childList = append(childList, child)
		}
		slices.Sort(childList)
		members[key] = childList
	}
	return true, members
}
