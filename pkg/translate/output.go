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

	"github.com/NVIDIA/topograph/internal/cluset"
	"github.com/NVIDIA/topograph/pkg/metrics"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	NVL72_DOMAIN_SIZE = 18
)

type fakeNodeConfig struct {
	baseBlockSize int
	index         int
	nodes         []string
}

func Write(wr io.Writer, root *topology.Vertex) error {
	var plugin string

	if len(root.Metadata) != 0 {
		plugin = root.Metadata[topology.KeyPlugin]
	}

	if plugin == topology.TopologyBlock {
		return toBlockTopology(wr, root)
	}

	return toTreeTopology(wr, root.Vertices[topology.TopologyTree])
}

func getFakeNodeConfig(fakeNodeData string) *fakeNodeConfig {
	return &fakeNodeConfig{
		nodes: cluset.Expand([]string{fakeNodeData}),
		index: 0,
	}
}

// getFreeFakeNodes generates fake nodes names.
func (fnc *fakeNodeConfig) getFreeFakeNodes(numFakeNodes int) []string {
	start := fnc.index
	end := fnc.index + numFakeNodes
	fnc.index = end
	return fnc.nodes[start:end]
}

func (fnc *fakeNodeConfig) isEnoughFakeNodesAvailable(blockSize int, numDomains int) bool {
	return len(fnc.nodes) >= (blockSize * numDomains)
}

func printBlock(wr io.Writer, block *topology.Vertex, domainVisited map[string]int, fnc *fakeNodeConfig) error {
	if _, ok := domainVisited[block.ID]; ok {
		return nil
	}

	nodes := make([]string, 0, len(block.Vertices))
	for _, node := range block.Vertices { //nodes within each domain
		nodes = append(nodes, node.Name)
	}
	var comment string
	if len(block.Name) != 0 {
		comment = fmt.Sprintf("# %s=%s\n", block.ID, block.Name)
	}

	outputNodeNames := strings.Join(cluset.Compact(nodes), ",")
	if fnc != nil && len(nodes) < fnc.baseBlockSize {
		fakeNodes := fnc.getFreeFakeNodes(fnc.baseBlockSize - len(nodes))
		fakeNodeNames := strings.Join(cluset.Compact(fakeNodes), ",")
		outputNodeNames = fmt.Sprintf("%s,%s", outputNodeNames, fakeNodeNames)
	}

	domainVisited[block.ID] = len(nodes)
	_, err := fmt.Fprintf(wr, "%sBlockName=%s Nodes=%s\n", comment, block.ID, outputNodeNames)
	return err
}

func findBlock(wr io.Writer, nodename string, root *topology.Vertex, domainVisited map[string]int, fnc *fakeNodeConfig) error {
	for _, block := range root.Vertices {
		if _, exists := block.Vertices[nodename]; exists {
			return printBlock(wr, block, domainVisited, fnc)
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

func printDisconnectedBlocks(wr io.Writer, root *topology.Vertex, domainVisited map[string]int, fnc *fakeNodeConfig) error {
	if root != nil {
		keys := sortVertices(root)
		for _, key := range keys {
			block := root.Vertices[key]
			err := printBlock(wr, block, domainVisited, fnc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func findMinDomainSize(blockRoot *topology.Vertex) (int, int) {
	minDomainSize := -1
	for _, block := range blockRoot.Vertices {
		blocklen := len(block.Vertices)
		if minDomainSize == -1 || minDomainSize > blocklen {
			minDomainSize = blocklen
		}
	}
	return minDomainSize, len(blockRoot.Vertices)
}

// getBlockSize returns blocksize for qeach possible level.
// Admin provided blocksize is validated and is overriden with default blocksizes if validation fails.
func getBlockSize(blockRoot *topology.Vertex, adminBlockSize string, fnc *fakeNodeConfig) (string, error) {
	// smallest domain size
	minDomainSize, numDomains := findMinDomainSize(blockRoot)

	if fnc != nil {
		minDomainSize = NVL72_DOMAIN_SIZE
	}

	maxnumbs := int(math.Log2(float64(numDomains)))
	var outputbs []string

	// If admin provided blocksize, validate it
	if len(strings.TrimSpace(adminBlockSize)) != 0 {
		blockSizes := strings.Split(adminBlockSize, ",")
		// parse blocksizes
		var planningBS int
		possiblebs := make(map[int]bool)
		for i := 0; i < len(blockSizes); i++ {
			bs, err := strconv.Atoi(blockSizes[i])
			if err != nil {
				metrics.AddValidationError("BlockSize parsing error")
				klog.Warningf("Failed to parse blockSize %v: %v. Ignoring admin blockSizes.", blockSizes[i], err)
				break
			}
			if i == 0 {
				if bs <= 0 || bs > minDomainSize {
					metrics.AddValidationError("bad admin blockSize")
					klog.Warningf("Overriding admin blockSizes. Planning blockSize %v does not meet criteria, should be > 0 & <= %v.", bs, minDomainSize)
					break
				}
				planningBS = bs
				// get possible blocksizes with the planningBS
				for l := 0; l <= maxnumbs; l++ {
					levelblocksize := int(math.Pow(2, float64(l))) * planningBS
					possiblebs[levelblocksize] = true
				}
			}

			if _, exists := possiblebs[bs]; !exists {
				metrics.AddValidationError("bad admin blockSize")
				klog.Warningf("Overriding admin blockSizes. BlockSize %v should follow the pattern (2^n) * %v, with n <= %v", bs, planningBS, maxnumbs)
				break
			}
			outputbs = append(outputbs, blockSizes[i])
		}
		if len(outputbs) == len(blockSizes) {
			if fnc != nil {
				fnc.baseBlockSize = planningBS
				if !fnc.isEnoughFakeNodesAvailable(fnc.baseBlockSize, numDomains) {
					return "", fmt.Errorf("not enough fake nodes available")
				}
			}
			return strings.Join(outputbs, ","), nil
		}
	}

	outputbs = nil
	/*
		Commented lines choose base blocksize as the largest power of 2, which is less than minDomainSize
		logDsize := math.Log2(float64(minDomainSize))
		bs := int(math.Pow(2, float64(int(logDsize))))
	*/
	bs := minDomainSize
	outputbs = append(outputbs, fmt.Sprintf("%d", bs))

	for i := 1; i <= maxnumbs; i++ {
		levelblocksize := int(math.Pow(2, float64(i))) * bs
		outputbs = append(outputbs, fmt.Sprintf("%d", levelblocksize))
	}

	if fnc != nil {
		fnc.baseBlockSize = bs
		if !fnc.isEnoughFakeNodesAvailable(fnc.baseBlockSize, numDomains) {
			return "", fmt.Errorf("not enough fake nodes available")
		}
	}

	return strings.Join(outputbs, ","), nil
}

func toBlockTopology(wr io.Writer, root *topology.Vertex) error {
	// traverse tree topology in DFS manner and when a node is reached, check within blockRoot for domain and print that domain.
	// keep a map of which domain has been printed
	treeRoot := root.Vertices[topology.TopologyTree]
	blockRoot := root.Vertices[topology.TopologyBlock]
	visited := make(map[string]bool)
	domainVisited := make(map[string]int)

	var fnc *fakeNodeConfig
	var err error
	if len(root.Metadata[topology.KeyFakeConfig]) > 0 {
		fnc = getFakeNodeConfig(root.Metadata[topology.KeyFakeConfig])
	}

	// calculate blocksize
	blockSize := ""
	if _, exists := root.Metadata[topology.KeyBlockSizes]; exists {
		blockSize = root.Metadata[topology.KeyBlockSizes]
	}
	blockSize, err = getBlockSize(blockRoot, blockSize, fnc)
	if err != nil {
		return err
	}

	// print blocks
	if treeRoot != nil {
		err = dfsTraversal(wr, treeRoot, blockRoot, visited, domainVisited, fnc)
		if err != nil {
			return err
		}
	}
	err = printDisconnectedBlocks(wr, blockRoot, domainVisited, fnc)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(wr, "BlockSizes=%s\n", blockSize)
	return err
}

func dfsTraversal(wr io.Writer, curVertex *topology.Vertex, blockRoot *topology.Vertex, visited map[string]bool, domainVisited map[string]int, fnc *fakeNodeConfig) error {
	visited[curVertex.ID] = true
	keys := sortVertices(curVertex)
	for _, key := range keys {
		w := curVertex.Vertices[key]
		if len(w.Vertices) == 0 { // it's a leaf; don't recurse
			err := findBlock(wr, w.ID, blockRoot, domainVisited, fnc)
			if err != nil {
				return err
			}
		} else {
			if !visited[w.ID] {
				err := dfsTraversal(wr, w, blockRoot, visited, domainVisited, fnc)
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
		_, err := fmt.Fprintf(wr, "%sSwitchName=%s Nodes=%s\n", comment, switchName, strings.Join(cluset.Compact(nodes), ","))
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
	_, err := wr.Write([]byte(fmt.Sprintf("%sSwitchName=%s Switches=%s\n", comment, v.Name, strings.Join(cluset.Compact(arr), ","))))
	if err != nil {
		return err
	}

	return nil
}

func GetTreeTestSet(testForLongLabelName bool) (*topology.Vertex, map[string]string) {
	//
	//        S1
	//      /    \
	//    S2      S3
	//    |       |
	//   ---     ---
	//   I14     I21
	//   I15     I22
	//   I16     I25
	//   ---     ---
	//
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
	treeRoot := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{"S1": sw1},
	}

	root := &topology.Vertex{
		Vertices: map[string]*topology.Vertex{topology.TopologyTree: treeRoot},
	}
	return root, instance2node
}

func GetBlockWithMultiIBTestSet() (*topology.Vertex, map[string]string) {
	//
	//     ibRoot2        ibRoot1
	//        |               |
	//        S1              S4
	//      /    \          /    \
	//    S2      S3      S5      S6
	//    |       |       |       |
	//   ---     ---     ---     ---
	//   I14\    I21\    I31\    I41\
	//   I15-B1  I22-B2  I32-B3  I42-B4
	//   I16/    I25/     I33/   I43/
	//   ---     ---      ---    ---
	//
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
			topology.KeyPlugin:     topology.TopologyBlock,
			topology.KeyBlockSizes: "3",
		},
	}
	return root, instance2node
}
