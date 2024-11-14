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

package ib

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/NVIDIA/topograph/pkg/topology"
	"golang.org/x/exp/maps"
)

var (
	reEmptyLine, reHCA, reSwitch, reConn, reSwitchName, reNodeName *regexp.Regexp
	seen                                                           map[int]map[string]*Switch
)

func init() {
	reEmptyLine = regexp.MustCompile(`^\s*$`)
	reHCA = regexp.MustCompile(`^Ca\s+\d+\s+"([^"]+)"\s+# "([^"]+)"`)
	reSwitch = regexp.MustCompile(`^Switch\s+\d+\s+"([^"]+)"\s+# "([^"]+)"`)
	reConn = regexp.MustCompile(`^\[\d+\]\s+"([^"]+)"\[\d+\](\([0-9a-f]+\))?\s+# "([^"]+)"`)
	reSwitchName = regexp.MustCompile(`^[^;:]+;([^;:]+):[^;:]+$`)
	reNodeName = regexp.MustCompile(`^(\S+)\s\S+$`)
}

type Switch struct {
	ID       string
	Name     string
	Height   int
	Conn     map[string]string  // ID:name
	Parents  map[string]bool    // ID:
	Children map[string]*Switch // ID:switch
	Nodes    map[string]string  // ID:node name
}

func GenerateTopologyConfig(data []byte) (*topology.Vertex, error) {
	switches, hca, err := ParseIbnetdiscoverFile(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse ibnetdiscover file: %v", err)
	}

	root, err := buildTree(switches, hca)
	if err != nil {
		return nil, fmt.Errorf("unable to build tree: %v", err)
	}
	seen = make(map[int]map[string]*Switch)
	root.simplify(root.getHeight())
	rootNode, err := root.toGraph()
	if err != nil {
		return nil, err
	}
	rootNode.Metadata = map[string]string{
		topology.KeyPlugin: topology.TopologyTree,
	}
	return rootNode, nil
}

func (sw *Switch) toGraph() (*topology.Vertex, error) {
	vertex := &topology.Vertex{
		Vertices: make(map[string]*topology.Vertex),
	}
	vertex.ID = sw.Name
	if len(sw.Children) == 0 {
		for id, name := range sw.Nodes {
			vertex.Vertices[id] = &topology.Vertex{
				Name: name,
				ID:   id,
			}
		}
	} else {
		for id, child := range sw.Children {
			v, err := child.toGraph()
			if err != nil {
				return nil, err
			}
			vertex.Vertices[id] = v
		}
	}
	return vertex, nil
}

// getHeight returns the height of the switch in the cluster topology.
// The height of a switch is defined as the maximum number of hops required to reach a leaf node from the switch.
func (sw *Switch) getHeight() int {
	height := 0
	if len(sw.Nodes) == 0 {
		for _, child := range sw.Children {
			if 1+child.getHeight() > height {
				height = 1 + child.getHeight()
			}
		}
	} else {
		height = 1
	}

	sw.Height = height
	return height
}

// process output of ibnetdiscover
func ParseIbnetdiscoverFile(data []byte) (map[string]*Switch, map[string]string, error) {
	switches := make(map[string]*Switch)
	hca := make(map[string]string)
	var entry *Switch

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			continue
		}

		if reEmptyLine.MatchString(line) {
			if entry != nil {
				switches[entry.ID] = entry
				entry = nil
			}
			continue
		}

		if match := reHCA.FindStringSubmatch(line); len(match) != 0 {
			hca[match[1]] = extractNodeName(match[2])
			continue
		}

		if match := reSwitch.FindStringSubmatch(line); len(match) != 0 {
			entry = &Switch{
				ID:       match[1],
				Name:     extractSwitchName(match[2]),
				Conn:     make(map[string]string),
				Parents:  make(map[string]bool),
				Children: make(map[string]*Switch),
				Nodes:    make(map[string]string),
			}
			continue
		}

		if match := reConn.FindStringSubmatch(line); len(match) != 0 && entry != nil {
			id := match[1]
			destName := match[3]
			entry.Conn[id] = destName
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return switches, hca, nil
}

func buildPatternFromName(nodeName string) string {
	pattern := ""
	gettingDigits := false
	for _, c := range nodeName {
		if strings.Contains("0123456789", string(c)) {
			gettingDigits = true
		} else {
			if gettingDigits {
				gettingDigits = false
				pattern += `(\d+)`
			}
			if c == '.' {
				pattern += `\.`

			} else {
				pattern += string(c)
			}
		}
	}
	if gettingDigits {
		pattern += `(\d+)`
	}
	return strings.ToLower(pattern)
}

func buildTree(switches map[string]*Switch, hca map[string]string) (*Switch, error) {
	// all visited nodes in tree
	visited := make(map[string]bool)
	// current level in the tree
	level := make(map[string]*Switch)

	nodeToPatternMap := make(map[string][]string)
	for _, name := range hca {
		builtPattern := buildPatternFromName(name)
		if _, ok := nodeToPatternMap[builtPattern]; !ok {
			nodeToPatternMap[builtPattern] = []string{}
		}
		nodeToPatternMap[builtPattern] = append(nodeToPatternMap[builtPattern], name)
	}

	computeNodePattern := ""
	maxLen := 0
	for pattern, names := range nodeToPatternMap {
		if len(names) > maxLen {
			maxLen = len(names)
			computeNodePattern = pattern
		}
	}
	computeNodePatternRegex := regexp.MustCompile(fmt.Sprintf("(?i)%s", computeNodePattern))

	// first pass: find all leaves
	for swID, sw := range switches {
		for conID := range sw.Conn {
			if nodeName, ok := hca[conID]; ok {
				// If a switch has HCA, it is a leaf.
				if len(nodeName) != 0 && computeNodePatternRegex.MatchString(nodeName) {
					sw.Nodes[conID] = nodeName
				}
				delete(sw.Conn, conID)
			}
		}
		if len(sw.Nodes) != 0 {
			// this is the leaf switch.
			// all conections are the parents.
			// add the node to the first (lower) level in the tree
			for conID := range sw.Conn {
				sw.Parents[conID] = true
			}
			level[swID] = sw
			visited[swID] = true
			sw.Conn = nil
		}
	}
	cnt := 1
	// next pass: complete the tree level by level
	for {
		// create an upper level
		upper := make(map[string]*Switch)
		for swID, sw := range switches {
			if visited[swID] {
				continue
			}
			for conID := range sw.Conn {
				// find children if any
				if child, ok := level[conID]; ok {
					sw.Children[conID] = child
					child.Parents[swID] = true
				}
			}
			if len(sw.Children) != 0 {
				for conID := range sw.Conn {
					if _, ok := sw.Children[conID]; !ok {
						sw.Parents[conID] = true
					}
				}
				upper[swID] = sw
				visited[swID] = true
				sw.Conn = nil
			}
		}
		if len(upper) == 0 {
			break
		}
		// complete level
		level = upper
		cnt++
	}

	root := &Switch{
		Children: make(map[string]*Switch),
	}
	for swID, sw := range level {
		root.Children[swID] = sw
	}

	return root, nil
}

func extractSwitchName(name string) string {
	if m := reSwitchName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	if !strings.ContainsAny(name, " ;:") {
		return name
	}
	return ""
}

func extractNodeName(name string) string {
	if m := reNodeName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// simplify simplifies the switch hierarchy by removing duplicate children at the specified height.
// It recursively traverses the switch hierarchy and checks for duplicate children.
// If a duplicate child is found, it is removed from the switch's children map.
// The `height` parameter specifies the current height in the hierarchy.
// The `seen` map is used to keep track of already seen children at each height.
// This method modifies the `sw` Switch object.
func (sw *Switch) simplify(height int) {
	if _, ok := seen[height]; !ok {
		seen[height] = make(map[string]*Switch)
	}

	if sw.Height >= 3 {
		for _, child := range sw.Children {
			child.simplify(height - 1)
		}
	}
	duplicates := make([]string, 0)
	for cID, v := range sw.Children {
		var childrenList string
		if v.Height > 1 {
			childrenList = getChildrenList(getChildrenName(maps.Values(v.Children)))
		} else {
			childrenList = getChildrenList(maps.Values(v.Nodes))
		}
		if len(childrenList) == 0 {
			duplicates = append(duplicates, cID)
			continue
		}

		if v_, ok := seen[height][childrenList]; ok {
			if v.Name != v_.Name {
				duplicates = append(duplicates, cID)
			}
			continue
		}
		seen[height][childrenList] = v
		groupName := fmt.Sprintf("group_%d_%d", height, len(seen[height]))
		v.Name = groupName
	}
	for _, cID := range duplicates {
		delete(sw.Children, cID)
	}
}

func getChildrenName(children []*Switch) []string {
	names := make([]string, 0)
	for _, sw := range children {
		names = append(names, sw.Name)
	}
	return names
}

func getChildrenList(children []string) string {
	sort.Strings(children)
	return strings.Join(children, ",")
}
