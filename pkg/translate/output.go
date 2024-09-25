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
	"sort"
	"strconv"
	"strings"

	"github.com/NVIDIA/topograph/pkg/common"
)

func ToSLURM(wr io.Writer, root *common.Vertex) error {
	visited := make(map[string]bool)
	leaves := make(map[string][]string)
	parents := []*common.Vertex{}
	queue := []*common.Vertex{root}

	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		if len(v.ID) != 0 {
			parents = append(parents, v)
		}

		for _, w := range v.Vertices {
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
			writeSwitch(wr, sw)
		}
	}

	for sw, nodes := range leaves {
		_, err := wr.Write([]byte(fmt.Sprintf("SwitchName=%s Nodes=%s\n", sw, strings.Join(compress(nodes), ","))))
		if err != nil {
			return err
		}
	}

	return nil
}

func writeSwitch(wr io.Writer, v *common.Vertex) error {
	if len(v.ID) == 0 {
		return nil
	}

	arr := make([]string, 0, len(v.Vertices))
	for _, node := range v.Vertices {
		arr = append(arr, node.ID)
	}

	_, err := wr.Write([]byte(fmt.Sprintf("SwitchName=%s Switches=%s\n", v.ID, strings.Join(compress(arr), ","))))
	if err != nil {
		return err
	}

	return nil
}

// compress finds contiguos numerical suffixes in names and presents then as ranges.
// example: ["eos0507", "eos0509", "eos0508"] -> ["eos0[507-509]
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

func GetTestSet(testForLongLabelName bool) (*common.Vertex, map[string]string) {
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

	n21 := &common.Vertex{ID: "I21", Name: "Node201"}
	n22 := &common.Vertex{ID: "I22", Name: "Node202"}
	n25 := &common.Vertex{ID: "I25", Name: "Node205"}

	n34 := &common.Vertex{ID: "I34", Name: "Node304"}
	n35 := &common.Vertex{ID: "I35", Name: "Node305"}
	n36 := &common.Vertex{ID: "I36", Name: "Node306"}

	sw2 := &common.Vertex{
		ID:       "S2",
		Vertices: map[string]*common.Vertex{"I21": n21, "I22": n22, "I25": n25},
	}
	sw3 := &common.Vertex{
		ID:       s3name,
		Vertices: map[string]*common.Vertex{"I34": n34, "I35": n35, "I36": n36},
	}
	sw1 := &common.Vertex{
		ID:       "S1",
		Vertices: map[string]*common.Vertex{"S2": sw2, s3name: sw3},
	}
	root := &common.Vertex{
		Vertices: map[string]*common.Vertex{"S1": sw1},
	}

	return root, instance2node
}
