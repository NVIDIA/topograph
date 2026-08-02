/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package lldp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

const (
	NAME_BM  = "lldp-bm"
	NAME_K8S = "lldp-k8s"

	lldpctlExecutable = "lldpctl"
	lldpctlCommand    = `output=$(lldpctl -f json) && printf '%s' "$output" | tr -d '\n'`
)

var errNoLLDPNeighbor = errors.New("no LLDP neighbor found") // nolint:gochecknoglobals

// Neighbor is the node-local LLDP view of a directly attached device.
type Neighbor struct {
	Interface     string
	ChassisIDType string
	ChassisID     string
	SystemName    string
	PortID        string
}

type lldpDocument struct {
	LLDP struct {
		Interface json.RawMessage `json:"interface"`
	} `json:"lldp"`
}

type lldpInterface struct {
	Name    string
	Via     string
	Chassis lldpChassis
	Port    lldpPort
}

type lldpInterfaceRecord struct {
	Name    string          `json:"name"`
	Via     string          `json:"via"`
	Chassis json.RawMessage `json:"chassis"`
	Port    lldpPort        `json:"port"`
}

type lldpChassis struct {
	ID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"id"`
	Name string `json:"name"`
}

type lldpPort struct {
	ID struct {
		Value string `json:"value"`
	} `json:"id"`
}

func parseNeighbors(data []byte) ([]Neighbor, error) {
	var document lldpDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("failed to parse lldpctl JSON: %w", err)
	}
	if len(document.LLDP.Interface) == 0 || string(document.LLDP.Interface) == "null" {
		return nil, nil
	}

	interfaces, err := decodeInterfaces(document.LLDP.Interface)
	if err != nil {
		return nil, err
	}

	neighbors := make([]Neighbor, 0, len(interfaces))
	for _, intf := range interfaces {
		if intf.Via != "" && !strings.EqualFold(intf.Via, "LLDP") {
			continue
		}
		if strings.TrimSpace(intf.Chassis.ID.Value) == "" {
			continue
		}
		neighbors = append(neighbors, Neighbor{
			Interface:     strings.TrimSpace(intf.Name),
			ChassisIDType: strings.ToLower(strings.TrimSpace(intf.Chassis.ID.Type)),
			ChassisID:     strings.TrimSpace(intf.Chassis.ID.Value),
			SystemName:    strings.TrimSpace(intf.Chassis.Name),
			PortID:        strings.TrimSpace(intf.Port.ID.Value),
		})
	}

	return neighbors, nil
}

func decodeInterfaces(data []byte) ([]lldpInterface, error) {
	entries, err := rawObjectEntries(data, "lldpctl interfaces")
	if err != nil {
		return nil, err
	}

	interfaces := []lldpInterface{}
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(entry, &fields); err != nil {
			return nil, fmt.Errorf("failed to parse lldpctl interface entry: %w", err)
		}
		if isInterfaceRecord(fields) {
			decoded, err := decodeInterfaceRecord("", entry)
			if err != nil {
				return nil, err
			}
			interfaces = append(interfaces, decoded...)
			continue
		}

		for _, name := range sortedRawKeys(fields) {
			decoded, err := decodeInterfaceRecord(name, fields[name])
			if err != nil {
				return nil, fmt.Errorf("failed to parse lldpctl interface %q: %w", name, err)
			}
			interfaces = append(interfaces, decoded...)
		}
	}
	return interfaces, nil
}

func decodeInterfaceRecord(name string, data []byte) ([]lldpInterface, error) {
	var record lldpInterfaceRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("invalid interface record: %w", err)
	}
	if record.Name == "" {
		record.Name = name
	}

	chassis, err := decodeChassis(record.Chassis)
	if err != nil {
		return nil, err
	}
	interfaces := make([]lldpInterface, 0, max(1, len(chassis)))
	if len(chassis) == 0 {
		chassis = []lldpChassis{{}}
	}
	for _, member := range chassis {
		interfaces = append(interfaces, lldpInterface{
			Name:    record.Name,
			Via:     record.Via,
			Chassis: member,
			Port:    record.Port,
		})
	}
	return interfaces, nil
}

func decodeChassis(data []byte) ([]lldpChassis, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, nil
	}
	entries, err := rawObjectEntries(data, "lldpctl chassis")
	if err != nil {
		return nil, err
	}

	chassis := []lldpChassis{}
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(entry, &fields); err != nil {
			return nil, fmt.Errorf("failed to parse lldpctl chassis entry: %w", err)
		}
		if isChassisRecord(fields) {
			var member lldpChassis
			if err := json.Unmarshal(entry, &member); err != nil {
				return nil, fmt.Errorf("failed to parse lldpctl chassis: %w", err)
			}
			chassis = append(chassis, member)
			continue
		}

		for _, name := range sortedRawKeys(fields) {
			var member lldpChassis
			if err := json.Unmarshal(fields[name], &member); err != nil {
				return nil, fmt.Errorf("failed to parse lldpctl chassis %q: %w", name, err)
			}
			if member.Name == "" {
				member.Name = name
			}
			chassis = append(chassis, member)
		}
	}
	return chassis, nil
}

func rawObjectEntries(data []byte, field string) ([]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, nil
	}
	if data[0] != '[' {
		return []json.RawMessage{append(json.RawMessage(nil), data...)}, nil
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", field, err)
	}
	return entries, nil
}

func isInterfaceRecord(fields map[string]json.RawMessage) bool {
	return rawJSONString(fields["name"]) || rawJSONString(fields["via"])
}

func isChassisRecord(fields map[string]json.RawMessage) bool {
	if rawJSONString(fields["name"]) {
		return true
	}
	var idFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["id"], &idFields); err != nil {
		return false
	}
	_, hasType := idFields["type"]
	_, hasValue := idFields["value"]
	return hasType || hasValue
}

func rawJSONString(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) != 0 && data[0] == '"'
}

func sortedRawKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func selectNeighbor(neighbors []Neighbor, interfaces []string) (Neighbor, error) {
	selectedInterfaces := make(map[string]bool, len(interfaces))
	for _, name := range interfaces {
		selectedInterfaces[name] = true
	}

	byChassis := make(map[string][]Neighbor)
	for _, neighbor := range neighbors {
		if len(selectedInterfaces) != 0 && !selectedInterfaces[neighbor.Interface] {
			continue
		}
		key := neighbor.chassisKey()
		if key == ":" {
			continue
		}
		byChassis[key] = append(byChassis[key], neighbor)
	}

	if len(byChassis) == 0 {
		if len(interfaces) == 0 {
			return Neighbor{}, errNoLLDPNeighbor
		}
		return Neighbor{}, fmt.Errorf("%w on selected interfaces %s", errNoLLDPNeighbor, strings.Join(interfaces, ","))
	}
	if len(byChassis) > 1 {
		attachments := make([]string, 0, len(byChassis))
		for chassis, matches := range byChassis {
			localInterfaces := make([]string, 0, len(matches))
			for _, match := range matches {
				localInterfaces = append(localInterfaces, match.Interface)
			}
			sort.Strings(localInterfaces)
			attachments = append(attachments, fmt.Sprintf("%s=%s", strings.Join(localInterfaces, ","), chassis))
		}
		sort.Strings(attachments)
		return Neighbor{}, fmt.Errorf("multiple LLDP switches found (%s); configure provider.params.interfaces to select one attachment", strings.Join(attachments, "; "))
	}

	for _, matches := range byChassis {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].Interface < matches[j].Interface
		})
		return matches[0], nil
	}
	return Neighbor{}, errNoLLDPNeighbor
}

func validateInterfaces(interfaces []string) error {
	seen := make(map[string]bool, len(interfaces))
	for index, name := range interfaces {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("interfaces[%d] must not be empty", index)
		}
		if seen[name] {
			return fmt.Errorf("duplicate interface %q", name)
		}
		seen[name] = true
		interfaces[index] = name
	}
	return nil
}

func parseInterfaceList(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	interfaces := strings.Split(value, ",")
	if err := validateInterfaces(interfaces); err != nil {
		return nil, err
	}
	return interfaces, nil
}

func (n Neighbor) chassisKey() string {
	idType := strings.ToLower(strings.TrimSpace(n.ChassisIDType))
	value := strings.TrimSpace(n.ChassisID)
	if idType == "mac" {
		if mac, err := net.ParseMAC(value); err == nil {
			value = mac.String()
		} else {
			value = strings.ToLower(value)
		}
	}
	return idType + ":" + value
}

func switchID(chassisKey string) (string, error) {
	idType, value, ok := strings.Cut(chassisKey, ":")
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("invalid LLDP chassis identifier %q", chassisKey)
	}
	if strings.EqualFold(strings.TrimSpace(idType), "mac") {
		mac, err := net.ParseMAC(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("invalid LLDP MAC chassis identifier %q: %w", value, err)
		}
		return "lldp-" + hex.EncodeToString(mac), nil
	}

	normalized := strings.ToLower(strings.TrimSpace(idType)) + ":" + strings.TrimSpace(value)
	sum := sha256.Sum256([]byte(normalized))
	return "lldp-" + hex.EncodeToString(sum[:8]), nil
}
