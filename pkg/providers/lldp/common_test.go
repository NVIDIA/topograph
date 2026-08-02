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
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLLDPCTLCommandPreservesCollectionFailure(t *testing.T) {
	command := strings.Replace(lldpctlCommand, "lldpctl -f json", "false", 1)
	err := exec.Command("sh", "-c", command).Run()
	require.Error(t, err)
}

const multipleNeighborsJSON = `{
  "lldp": {
    "interface": [
      {
        "eno1": {
          "via": "LLDP",
          "chassis": {
            "leaf-1": {
              "id": {"type": "mac", "value": "00:11:22:33:44:55"}
            }
          },
          "port": {"id": {"value": "Ethernet1/1"}}
        }
      },
      {
        "eno2": {
          "via": "LLDP",
          "chassis": {
            "leaf-1": {
              "id": {"type": "mac", "value": "00:11:22:33:44:55"}
            }
          },
          "port": {"id": {"value": "Ethernet1/2"}}
        }
      },
      {
        "eno3": {
          "via": "LLDP",
          "chassis": {
            "leaf-2": {
              "id": {"type": "mac", "value": "00:11:22:33:44:66"}
            }
          },
          "port": {"id": {"value": "Ethernet1/3"}}
        }
      },
      {
        "eno4": {
          "via": "CDPv2",
          "chassis": {
            "other-device": {
              "id": {"type": "local", "value": "not-lldp"}
            }
          }
        }
      }
    ]
  }
}`

func TestParseNeighbors(t *testing.T) {
	neighbors, err := parseNeighbors([]byte(multipleNeighborsJSON))
	require.NoError(t, err)
	require.Len(t, neighbors, 3)
	require.Equal(t, Neighbor{
		Interface:     "eno1",
		ChassisIDType: "mac",
		ChassisID:     "00:11:22:33:44:55",
		SystemName:    "leaf-1",
		PortID:        "Ethernet1/1",
	}, neighbors[0])
}

func TestParseNeighborsAcceptsSingleInterfaceObject(t *testing.T) {
	data := `{"lldp":{"interface":{"eth0":{"via":"LLDP","chassis":{"leaf-switch-a":{"id":{"type":"local","value":"switch-a"}}}}}}}`
	neighbors, err := parseNeighbors([]byte(data))
	require.NoError(t, err)
	require.Equal(t, []Neighbor{{
		Interface:     "eth0",
		ChassisIDType: "local",
		ChassisID:     "switch-a",
		SystemName:    "leaf-switch-a",
	}}, neighbors)
}

func TestParseNeighborsAcceptsMultipleKeysInInterfaceObject(t *testing.T) {
	data := `{
  "lldp": {
    "interface": {
      "eth0": {
        "via": "LLDP",
        "chassis": {"leaf-a": {"id": {"type": "local", "value": "switch-a"}}}
      },
      "eth1": {
        "via": "LLDP",
        "chassis": {"leaf-b": {"id": {"type": "local", "value": "switch-b"}}}
      }
    }
  }
}`
	neighbors, err := parseNeighbors([]byte(data))
	require.NoError(t, err)
	require.Equal(t, []Neighbor{
		{Interface: "eth0", ChassisIDType: "local", ChassisID: "switch-a", SystemName: "leaf-a"},
		{Interface: "eth1", ChassisIDType: "local", ChassisID: "switch-b", SystemName: "leaf-b"},
	}, neighbors)
}

func TestParseNeighborsHandlesNamesMatchingRecordFields(t *testing.T) {
	data := `{
  "lldp": {
    "interface": {
      "port": {
        "via": "LLDP",
        "chassis": {"id": {"id": {"type": "local", "value": "switch-a"}}}
      }
    }
  }
}`
	neighbors, err := parseNeighbors([]byte(data))
	require.NoError(t, err)
	require.Equal(t, []Neighbor{{
		Interface:     "port",
		ChassisIDType: "local",
		ChassisID:     "switch-a",
		SystemName:    "id",
	}}, neighbors)
}

func TestParseNeighborsRetainsNormalizedCompatibility(t *testing.T) {
	data := `{"lldp":{"interface":{"name":"eth0","via":"LLDP","chassis":{"id":{"type":"local","value":"switch-a"},"name":"leaf-a"}}}}`
	neighbors, err := parseNeighbors([]byte(data))
	require.NoError(t, err)
	require.Equal(t, []Neighbor{{
		Interface:     "eth0",
		ChassisIDType: "local",
		ChassisID:     "switch-a",
		SystemName:    "leaf-a",
	}}, neighbors)
}

func TestParseNeighborsRejectsInvalidJSON(t *testing.T) {
	_, err := parseNeighbors([]byte(`{"lldp":`))
	require.ErrorContains(t, err, "failed to parse lldpctl JSON")
}

func TestParseNeighborsRejectsInvalidKeyedInterface(t *testing.T) {
	_, err := parseNeighbors([]byte(`{"lldp":{"interface":{"eth0":"invalid"}}}`))
	require.ErrorContains(t, err, `failed to parse lldpctl interface "eth0"`)
}

func TestSelectNeighbor(t *testing.T) {
	neighbors, err := parseNeighbors([]byte(multipleNeighborsJSON))
	require.NoError(t, err)

	neighbor, err := selectNeighbor(neighbors, []string{"eno1", "eno2"})
	require.NoError(t, err)
	require.Equal(t, "eno1", neighbor.Interface)

	neighbor, err = selectNeighbor(neighbors, []string{"eno3"})
	require.NoError(t, err)
	require.Equal(t, "00:11:22:33:44:66", neighbor.ChassisID)

	_, err = selectNeighbor(neighbors, nil)
	require.ErrorContains(t, err, "multiple LLDP switches found")
	require.ErrorContains(t, err, "configure provider.params.interfaces")

	_, err = selectNeighbor(neighbors, []string{"eth9"})
	require.ErrorIs(t, err, errNoLLDPNeighbor)
}

func TestSwitchID(t *testing.T) {
	id, err := switchID("mac:00:11:22:33:44:55")
	require.NoError(t, err)
	require.Equal(t, "lldp-001122334455", id)

	localID, err := switchID("local:leaf switch/1")
	require.NoError(t, err)
	require.Regexp(t, `^lldp-[0-9a-f]{16}$`, localID)

	sameID, err := switchID("LOCAL:leaf switch/1")
	require.NoError(t, err)
	require.Equal(t, localID, sameID)

	differentCaseID, err := switchID("local:Leaf switch/1")
	require.NoError(t, err)
	require.NotEqual(t, localID, differentCaseID)

	_, err = switchID("missing-separator")
	require.EqualError(t, err, `invalid LLDP chassis identifier "missing-separator"`)
}

func TestValidateInterfaces(t *testing.T) {
	interfaces := []string{" eno1 ", "eno2"}
	require.NoError(t, validateInterfaces(interfaces))
	require.Equal(t, []string{"eno1", "eno2"}, interfaces)
	require.EqualError(t, validateInterfaces([]string{"eno1", "eno1"}), `duplicate interface "eno1"`)
	require.EqualError(t, validateInterfaces([]string{""}), "interfaces[0] must not be empty")
}
