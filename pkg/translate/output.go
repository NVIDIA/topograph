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

package translate

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
)

func ToGraph(wr io.Writer, root *topology.Vertex) error {
	if len(root.Metadata) != 0 {
		if root.Metadata[topology.KeyPlugin] == topology.TopologyBlock {
			return toBlockTopology(wr, root)
		} else if root.Metadata[topology.KeyPlugin] == topology.TopologyTree {
			return toTreeTopology(wr, root)
		}
	}
	return fmt.Errorf("topology metadata not set in root node")
}

func printBlock(wr io.Writer, block *topology.Vertex, domainVisited map[string]int) error {
	if _, exists := domainVisited[block.ID]; !exists {
		nodes := make([]string, 0, len(block.Vertices))
		for _, node := range block.Vertices { //nodes within each domain
			nodes = append(nodes, node.Name)
		}
		_, err := wr.Write([]byte(fmt.Sprintf("BlockName=%s Nodes=%s\n", block.ID, strings.Join(compress(nodes), ","))))
		if err != nil {
			return err
		}
		domainVisited[block.ID] = len(nodes)
	}
	return nil
}

func findBlock(wr io.Writer, nodename string, root *topology.Vertex, domainVisited map[string]int) error { // blockRoot
	for _, block := range root.Vertices {
		if _, exists := block.Vertices[nodename]; exists {
			return printBlock(wr, block, domainVisited)
		}
	}
	return nil
}

func sortVertices(root *topology.Vertex) []string {
	// sort the IDs
	keys := make([]string, 0, len(root.Vertices))
	for key := range root.Vertices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func printDisconnectedBlocks(wr io.Writer, root *topology.Vertex, domainVisited map[string]int) error {
	if root != nil {
		keys := sortVertices(root)
		for _, key := range keys {
			block := root.Vertices[key]
			err := printBlock(wr, block, domainVisited)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getBlockSize(domainVisited map[string]int, adminBlockSize string) string {
	minDomainSize := -1
	for _, dSize := range domainVisited {
		if minDomainSize == -1 || minDomainSize > dSize {
			minDomainSize = dSize
		}
	}
	if adminBlockSize != "" {
		blockSizes := strings.Split(adminBlockSize, ",")
		planningBS, err := strconv.Atoi(blockSizes[0])
		if err != nil {
			metrics.AddBlockSizeValidationError("parsing error")
			klog.Warningf("Failed to parse blockSize %v: %v. Ignoring.", blockSizes[0], err)
		} else {
			if planningBS > 0 && planningBS <= minDomainSize {
				return adminBlockSize
			}
			metrics.AddBlockSizeValidationError("bad domain size")
			klog.Warningf("Overriden planning blockSize of %v does not meet criteria, minimum domain size %v. Ignoring.", planningBS, minDomainSize)
		}
	}
	logDsize := math.Log2(float64(minDomainSize))
	bs := math.Pow(2, float64(int(logDsize)))
	return strconv.Itoa(int(bs))
}

func toBlockTopology(wr io.Writer, root *topology.Vertex) error {
	// traverse tree topology in DFS manner and when a node is reached, check within blockRoot for domain and print that domain.
	// keep a map of which domain has been printed
	treeRoot := root.Vertices[topology.TopologyTree]
	blockRoot := root.Vertices[topology.TopologyBlock]
	visited := make(map[string]bool)
	domainVisited := make(map[string]int)

	if treeRoot != nil {
		err := dfsTraversal(wr, treeRoot, blockRoot, visited, domainVisited)
		if err != nil {
			return err
		}
	}
	err := printDisconnectedBlocks(wr, blockRoot, domainVisited)
	if err != nil {
		return err
	}
	blockSize := ""
	if _, exists := root.Metadata[topology.KeyBlockSizes]; exists {
		blockSize = root.Metadata[topology.KeyBlockSizes]
	}
	blockSize = getBlockSize(domainVisited, blockSize)
	_, err = wr.Write([]byte(fmt.Sprintf("BlockSizes=%s\n", blockSize)))
	return err
}

func dfsTraversal(wr io.Writer, curVertex *topology.Vertex, blockRoot *topology.Vertex, visited map[string]bool, domainVisited map[string]int) error {
	visited[curVertex.ID] = true
	keys := sortVertices(curVertex)
	for _, key := range keys {
		w := curVertex.Vertices[key]
		if len(w.Vertices) == 0 { // it's a leaf; don't add to queue
			err := findBlock(wr, w.ID, blockRoot, domainVisited)
			if err != nil {
				return err
			}
		} else {
			if !visited[w.ID] {
				err := dfsTraversal(wr, w, blockRoot, visited, domainVisited)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func toTreeTopology(wr io.Writer, root *topology.Vertex) error {
	visited := make(map[string]bool)
	leaves := make(map[string][]string)
	parents := []*topology.Vertex{}
	queue := []*topology.Vertex{root}
	idToName := make(map[string]string)

	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		if len(v.ID) != 0 {
			parents = append(parents, v)
		}
		idToName[v.ID] = v.Name

		// sort the IDs
		keys := sortVertices(v)
		for _, key := range keys {
			w := v.Vertices[key]
			if len(w.Vertices) == 0 { // it's a leaf; don't add to queue
				_, ok := leaves[v.ID]
				if !ok {
					leaves[v.ID] = []string{}
				}
				leaves[v.ID] = append(leaves[v.ID], w.Name)
			} else if !visited[w.ID] {
				queue = append(queue, w)
				visited[w.ID] = true
			}
		}
	}

	for _, sw := range parents {
		if _, ok := leaves[sw.ID]; !ok {
			err := writeSwitch(wr, sw)
			if err != nil {
				return fmt.Errorf("failed to write switch %s: %w", sw.ID, err)
			}
		}
	}

	// sort leaves IDs
	ids := make([]string, 0, len(leaves))
	for id := range leaves {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var comment, switchName string
	for _, sw := range ids {
		nodes := leaves[sw]
		if idToName[sw] != "" {
			comment = fmt.Sprintf("# %s=%s\n", idToName[sw], sw)
			switchName = idToName[sw]
		} else {
			comment = ""
			switchName = sw
		}
		_, err := wr.Write([]byte(fmt.Sprintf("%sSwitchName=%s Nodes=%s\n", comment, switchName, strings.Join(compress(nodes), ","))))
		if err != nil {
			return err
		}
	}

	return nil
}

func writeSwitch(wr io.Writer, v *topology.Vertex) error {
	if len(v.ID) == 0 {
		return nil
	}

	arr := make([]string, 0, len(v.Vertices))
	for _, node := range v.Vertices {
		if node.Name == "" {
			arr = append(arr, node.ID)
		} else {
			arr = append(arr, node.Name)
		}
	}
	var comment string
	if v.Name == "" {
		comment = ""
		v.Name = v.ID
	} else {
		comment = fmt.Sprintf("# %s=%s\n", v.Name, v.ID)
	}
	_, err := wr.Write([]byte(fmt.Sprintf("%sSwitchName=%s Switches=%s\n", comment, v.Name, strings.Join(compress(arr), ","))))
	if err != nil {
		return err
	}

	return nil
}

// compress finds contiguos numerical suffixes in names and presents then as ranges.
// example: ["eos0507", "eos0509", "eos0508"] -> ["eos0[507-509"]
func compress(input []string) []string {
	ret := []string{}
	keys := []string{}
	m := make(map[string][]int) // map of prefix : array of numerical suffix
	for _, name := range input {
		if prefix, suffix := split(name); len(suffix) == 0 {
			ret = append(ret, name)
		} else {
			num, _ := strconv.Atoi(suffix)
			if arr, ok := m[prefix]; !ok {
				m[prefix] = []int{num}
			} else {
				m[prefix] = append(arr, num)
			}
		}
	}

	// we sort the prefix to get consistent output for tests
	for prefix := range m {
		keys = append(keys, prefix)
	}
	sort.Strings(keys)

	for _, prefix := range keys {
		arr := m[prefix]
		sort.Ints(arr)
		var start, end int
		for i, num := range arr {
			if i == 0 {
				start = num
				end = num
			} else if num == end+1 {
				end = num
			} else if start == end {
				ret = append(ret, fmt.Sprintf("%s%d", prefix, start))
				start = num
				end = num
			} else {
				ret = append(ret, fmt.Sprintf("%s[%d-%d]", prefix, start, end))
				start = num
				end = num
			}
		}
		if start == end {
			ret = append(ret, fmt.Sprintf("%s%d", prefix, end))
		} else {
			ret = append(ret, fmt.Sprintf("%s[%d-%d]", prefix, start, end))
		}
	}

	return ret
}

// split divides a string into a prefix and a numerical suffix
func split(input string) (string, string) {
	n := len(input)
	if n == 0 {
		return input, ""
	}

	// find numerical suffix
	i := n - 1
	for i >= 0 {
		if input[i] >= '0' && input[i] <= '9' {
			i--
		} else {
			break
		}
	}
	i++

	// ignore leading zeros
	for i < n {
		if input[i] == '0' {
			i++
		} else {
			break
		}
	}

	if i == n { // no suffix
		return input, ""
	}

	return input[:i], input[i:]
}

func GetTreeTestSet(testForLongLabelName bool) (*topology.Vertex, map[string]string) {
	var s3name string
	if testForLongLabelName {
		s3name = "S3very-very-long-id-to-check-label-value-limits-of-63-characters"
	} else {
		s3name = "S3"
	}

	instance2node := map[string]string{
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
		"I34": "Node304", "I35": "Node305", "I36": "Node306",
	}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	n34 := &topology.Vertex{ID: "I34", Name: "Node304"}
	n35 := &topology.Vertex{ID: "I35", Name: "Node305"}
	n36 := &topology.Vertex{ID: "I36", Name: "Node306"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	sw3 := &topology.Vertex{
		ID:       s3name,
		Vertices: map[string]*topology.Vertex{"I34": n34, "I35": n35, "I36": n36},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S2": sw2, s3name: sw3},
	}
	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S1": sw1},
		Metadata: map[string]string{
			topology.KeyPlugin: topology.TopologyTree,
		},
	}

	return root, instance2node
}

func GetBlockWithMultiIBTestSet() (*topology.Vertex, map[string]string) {
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
		"I31": "Node301", "I32": "Node302", "I33": "Node303",
		"I41": "Node401", "I42": "Node402", "I43": "Node403",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	n31 := &topology.Vertex{ID: "I31", Name: "Node301"}
	n32 := &topology.Vertex{ID: "I32", Name: "Node302"}
	n33 := &topology.Vertex{ID: "I33", Name: "Node303"}

	n41 := &topology.Vertex{ID: "I41", Name: "Node401"}
	n42 := &topology.Vertex{ID: "I42", Name: "Node402"}
	n43 := &topology.Vertex{ID: "I43", Name: "Node403"}

	sw5 := &topology.Vertex{
		ID:       "S5",
		Vertices: map[string]*topology.Vertex{"I31": n31, "I32": n32, "I33": n33},
	}
	sw6 := &topology.Vertex{
		ID:       "S6",
		Vertices: map[string]*topology.Vertex{"I41": n41, "I42": n42, "I43": n43},
	}
	sw4 := &topology.Vertex{
		ID:       "S4",
		Vertices: map[string]*topology.Vertex{"S5": sw5, "S6": sw6},
	}
	ibRoot1 := &topology.Vertex{
		ID:       "ibRoot1",
		Vertices: map[string]*topology.Vertex{"S4": sw4},
	}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S2": sw2, "S3": sw3},
	}
	ibRoot2 := &topology.Vertex{
		ID:       "ibRoot2",
		Vertices: map[string]*topology.Vertex{"S1": sw1},
	}

	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"IB1": ibRoot1, "IB2": ibRoot2},
	}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	block3 := &topology.Vertex{
		ID:       "B3",
		Vertices: map[string]*topology.Vertex{"I31": n31, "I32": n32, "I33": n33},
	}
	block4 := &topology.Vertex{
		ID:       "B4",
		Vertices: map[string]*topology.Vertex{"I41": n41, "I42": n42, "I43": n43},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2, "B3": block3, "B4": block4},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyEngine:     engines.EngineSLURM,
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}

func GetBlockWithIBTestSet() (*topology.Vertex, map[string]string) {
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S2": sw2, "S3": sw3},
	}
	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S1": sw1},
	}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyEngine:     engines.EngineSLURM,
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}

func GetBlockWithDFSIBTestSet() (*topology.Vertex, map[string]string) {
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105",
		"I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}

	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	sw2 := &topology.Vertex{
		ID:       "S2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}

	sw4 := &topology.Vertex{
		ID:       "S4",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	sw5 := &topology.Vertex{
		ID:       "S5",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	sw3 := &topology.Vertex{
		ID:       "S3",
		Vertices: map[string]*topology.Vertex{"S5": sw5},
	}
	sw1 := &topology.Vertex{
		ID:       "S1",
		Vertices: map[string]*topology.Vertex{"S4": sw4},
	}

	sw0 := &topology.Vertex{
		ID:       "S0",
		Vertices: map[string]*topology.Vertex{"S1": sw1, "S2": sw2, "S3": sw3},
	}

	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S0": sw0},
	}

	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15},
	}
	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I22": n22},
	}

	block3 := &topology.Vertex{
		ID:       "B3",
		Vertices: map[string]*topology.Vertex{"I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2, "B3": block3},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot, topology.TopologyTree: treeRoot},
		Metadata: map[string]string{
			topology.KeyEngine:     engines.EngineSLURM,
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "1",
		},
	}
	return root, instance2node
}

func GetBlockTestSet() (*topology.Vertex, map[string]string) {
	instance2node := map[string]string{
		"I14": "Node104", "I15": "Node105", "I16": "Node106",
		"I21": "Node201", "I22": "Node202", "I25": "Node205",
	}

	n14 := &topology.Vertex{ID: "I14", Name: "Node104"}
	n15 := &topology.Vertex{ID: "I15", Name: "Node105"}
	n16 := &topology.Vertex{ID: "I16", Name: "Node106"}

	n21 := &topology.Vertex{ID: "I21", Name: "Node201"}
	n22 := &topology.Vertex{ID: "I22", Name: "Node202"}
	n25 := &topology.Vertex{ID: "I25", Name: "Node205"}

	block1 := &topology.Vertex{
		ID:       "B1",
		Vertices: map[string]*topology.Vertex{"I14": n14, "I15": n15, "I16": n16},
	}
	block2 := &topology.Vertex{
		ID:       "B2",
		Vertices: map[string]*topology.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}

	blockRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"B1": block1, "B2": block2},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyBlock: blockRoot},
		Metadata: map[string]string{
			topology.KeyEngine:     engines.EngineSLURM,
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}
