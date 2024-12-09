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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Request struct {
	Provider Provider           `json:"provider"`
	Engine   Engine             `json:"engine"`
	Nodes    []ComputeInstances `json:"nodes"`
}

type Provider struct {
	Name   string            `json:"name"`
	Creds  map[string]string `json:"creds"` // access credentials
	Params map[string]any    `json:"params"`
}

type Engine struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

type ComputeInstances struct {
	Region    string            `json:"region"`
	Instances map[string]string `json:"instances"` // <instance ID>:<node name> map
}

func NewRequest(prv string, creds map[string]string, eng string, params map[string]any) *Request {
	return &Request{
		Provider: Provider{
			Name:  prv,
			Creds: creds,
		},
		Engine: Engine{
			Name:   eng,
			Params: params,
		},
	}
}

func (p *Request) String() string {
	var sb strings.Builder
	sb.WriteString("TopologyRequest:\n")
	sb.WriteString(fmt.Sprintf("  Provider:%s\n", spacer(p.Provider.Name)))
	sb.WriteString(map2string(p.Provider.Creds, "  Credentials", true, "\n"))
	sb.WriteString(mapOfAny2string(p.Provider.Params, "  Parameters", false, "\n"))
	sb.WriteString(fmt.Sprintf("  Engine:%s\n", spacer(p.Engine.Name)))
	sb.WriteString(mapOfAny2string(p.Engine.Params, "  Parameters", false, "\n"))
	sb.WriteString("  Nodes:")
	for _, nodes := range p.Nodes {
		sb.WriteByte(' ')
		sb.WriteString(map2string(nodes.Instances, nodes.Region, false, ""))
	}
	sb.WriteString("\n")
	return sb.String()
}

func GetTopologyRequest(body []byte) (*Request, error) {
	var payload Request

	if len(body) == 0 {
		return &payload, nil
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %v", err)
	}

	return &payload, nil
}

func spacer(value string) string {
	if len(value) > 0 {
		return " " + value
	}

	return ""
}

func map2string(m map[string]string, prefix string, hide bool, suffix string) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(": [")
	if n := len(m); n != 0 {
		keys := make([]string, 0, n)
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		terms := make([]string, 0, n)
		for _, key := range keys {
			if hide {
				terms = append(terms, fmt.Sprintf("%s:***", key))
			} else {
				terms = append(terms, fmt.Sprintf("%s:%s", key, m[key]))
			}
		}
		sb.WriteString(strings.Join(terms, " "))
	}
	sb.WriteString("]")
	sb.WriteString(suffix)

	return sb.String()
}

func mapOfAny2string(m map[string]any, prefix string, hide bool, suffix string) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(": [")
	if n := len(m); n != 0 {
		keys := make([]string, 0, n)
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		terms := make([]string, 0, n)
		for _, key := range keys {
			if hide {
				terms = append(terms, fmt.Sprintf("%s:***", key))
			} else {
				terms = append(terms, fmt.Sprintf("%s:%v", key, m[key]))
			}
		}
		sb.WriteString(strings.Join(terms, " "))
	}
	sb.WriteString("]")
	sb.WriteString(suffix)

	return sb.String()
}
