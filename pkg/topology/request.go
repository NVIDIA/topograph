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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
)

var (
	credentialHashKey     []byte
	credentialHashKeyErr  error
	credentialHashKeyOnce sync.Once
)

type Request struct {
	Provider Provider           `json:"provider"`
	Engine   Engine             `json:"engine"`
	Nodes    []ComputeInstances `json:"nodes"`
}

type Provider struct {
	Name   string         `json:"name"`
	Creds  map[string]any `json:"creds"` // access credentials
	Params map[string]any `json:"params"`
}

type Engine struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

type ComputeInstances struct {
	Region    string            `json:"region"`
	Instances map[string]string `json:"instances"` // <instance ID>:<node name> map
}

type requestHashData struct {
	Provider providerHashData       `json:"provider"`
	Engine   Engine                 `json:"engine"`
	Nodes    []computeInstancesHash `json:"nodes,omitempty"`
}

type providerHashData struct {
	Name             string         `json:"name"`
	Params           map[string]any `json:"params,omitempty"`
	CredentialDigest string         `json:"credentialDigest,omitempty"`
}

type computeInstancesHash struct {
	Region    string         `json:"region"`
	Instances []instanceHash `json:"instances"`
}

type instanceHash struct {
	ID   string `json:"id"`
	Node string `json:"node"`
}

func NewRequest(prv Provider, eng Engine) *Request {
	return &Request{
		Provider: prv,
		Engine:   eng,
	}
}

func (p *Request) String() string {
	var sb strings.Builder
	sb.WriteString("TopologyRequest:\n")
	sb.WriteString(fmt.Sprintf("  Provider:%s\n", spacer(p.Provider.Name)))
	sb.WriteString(map2string(p.Provider.Creds, "  Credentials", true, "\n"))
	sb.WriteString(map2string(p.Provider.Params, "  Parameters", false, "\n"))
	sb.WriteString(fmt.Sprintf("  Engine:%s\n", spacer(p.Engine.Name)))
	sb.WriteString(map2string(p.Engine.Params, "  Parameters", false, "\n"))
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

func map2string[T string | any](m map[string]T, prefix string, hide bool, suffix string) string {
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

// GetNodeNameList retrieves all the nodenames
func GetNodeNameList(cis []ComputeInstances) []string {
	nodes := []string{}
	for _, ci := range cis {
		for _, node := range ci.Instances {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// GetNodeNameMap retrieves all the nodenames
func GetNodeNameMap(cis []ComputeInstances) map[string]bool {
	nodes := make(map[string]bool)
	for _, ci := range cis {
		for _, node := range ci.Instances {
			nodes[node] = true
		}
	}
	return nodes
}

func (p *Request) Hash() (string, error) {
	credentialDigest, err := getCredentialDigest(p.Provider.Creds)
	if err != nil {
		return "", err
	}

	dataToHash := requestHashData{
		Provider: providerHashData{
			Name:             p.Provider.Name,
			Params:           p.Provider.Params,
			CredentialDigest: credentialDigest,
		},
		Engine: Engine{
			Name:   p.Engine.Name,
			Params: p.Engine.Params,
		},
		Nodes: canonicalComputeInstances(p.Nodes),
	}
	return GetHash(dataToHash)
}

func canonicalComputeInstances(nodes []ComputeInstances) []computeInstancesHash {
	if len(nodes) == 0 {
		return nil
	}

	canonical := make([]computeInstancesHash, 0, len(nodes))
	for _, nodeGroup := range nodes {
		instances := make([]instanceHash, 0, len(nodeGroup.Instances))
		for id, node := range nodeGroup.Instances {
			instances = append(instances, instanceHash{
				ID:   id,
				Node: node,
			})
		}
		sort.Slice(instances, func(i, j int) bool {
			if instances[i].ID != instances[j].ID {
				return instances[i].ID < instances[j].ID
			}
			return instances[i].Node < instances[j].Node
		})

		canonical = append(canonical, computeInstancesHash{
			Region:    nodeGroup.Region,
			Instances: instances,
		})
	}

	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Region != canonical[j].Region {
			return canonical[i].Region < canonical[j].Region
		}
		if len(canonical[i].Instances) != len(canonical[j].Instances) {
			return len(canonical[i].Instances) < len(canonical[j].Instances)
		}
		for idx := range canonical[i].Instances {
			if canonical[i].Instances[idx].ID != canonical[j].Instances[idx].ID {
				return canonical[i].Instances[idx].ID < canonical[j].Instances[idx].ID
			}
			if canonical[i].Instances[idx].Node != canonical[j].Instances[idx].Node {
				return canonical[i].Instances[idx].Node < canonical[j].Instances[idx].Node
			}
		}
		return false
	})

	return canonical
}

func getCredentialDigest(creds map[string]any) (string, error) {
	if len(creds) == 0 {
		return "", nil
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal credentials for hashing: %v", err)
	}

	key, err := getCredentialHashKey()
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func getCredentialHashKey() ([]byte, error) {
	credentialHashKeyOnce.Do(func() {
		credentialHashKey, credentialHashKeyErr = newCredentialHashKey()
	})
	if credentialHashKeyErr != nil {
		return nil, credentialHashKeyErr
	}
	return credentialHashKey, nil
}

func newCredentialHashKey() ([]byte, error) {
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate credential hash key: %v", err)
	}
	return key, nil
}

func GetHash(obj any) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request for hashing: %v", err)
	}

	h := fnv.New64a()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum64()), nil
}
