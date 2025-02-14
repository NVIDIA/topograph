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

package cluset

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var compactRegexp, expandRegexp *regexp.Regexp

func init() {
	compactRegexp = regexp.MustCompile(`^(.*?)(0*\d+)$`)
	expandRegexp = regexp.MustCompile(`^(.*?)\[(.*?)\]$`)
}

// Compact compresses a list of nodes into a compact range format
func Compact(nodes []string) []string {
	groups := make(map[string][]string)

	// Parse node names into groups
	for _, node := range nodes {
		matches := compactRegexp.FindStringSubmatch(node)
		if matches != nil {
			prefix, numStr := matches[1], matches[2]
			groups[prefix] = append(groups[prefix], numStr)
		} else {
			groups[node] = nil
		}
	}

	// Sort prefixes for consistent output
	prefixes := make([]string, 0, len(groups))
	for prefix := range groups {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)

	// Format compressed ranges
	var result []string
	for _, prefix := range prefixes {
		numbers := groups[prefix]
		sort.Slice(numbers, func(i, j int) bool {
			return atoi(numbers[i]) < atoi(numbers[j])
		})
		compressed := compressRange(numbers)
		result = append(result, fmt.Sprintf("%s%s", prefix, compressed))
	}

	return result
}

// Expand decompresses a list of compacted node names back to individual entries
func Expand(compressed []string) []string {
	var result []string

	for _, entry := range compressed {
		matches := expandRegexp.FindStringSubmatch(entry)
		if matches != nil {
			prefix, ranges := matches[1], matches[2]
			rangesParts := strings.Split(ranges, ",")
			for _, part := range rangesParts {
				if strings.Contains(part, "-") {
					bounds := strings.Split(part, "-")
					width := len(bounds[0]) // Preserve leading zeros
					for i := atoi(bounds[0]); i <= atoi(bounds[1]); i++ {
						result = append(result, fmt.Sprintf("%s%0*d", prefix, width, i))
					}
				} else {
					result = append(result, fmt.Sprintf("%s%s", prefix, part))
				}
			}
		} else {
			result = append(result, entry)
		}
	}
	return result
}

// compressRange converts a list of numbers into a compact range format
func compressRange(numbers []string) string {
	switch len(numbers) {
	case 0:
		return ""
	case 1:
		return numbers[0]
	default:
		var parts []string
		start, end := numbers[0], numbers[0]

		for i := 1; i < len(numbers); i++ {
			if atoi(numbers[i]) == atoi(end)+1 {
				end = numbers[i]
			} else {
				parts = append(parts, formatRange(start, end))
				start, end = numbers[i], numbers[i]
			}
		}
		parts = append(parts, formatRange(start, end))

		return "[" + strings.Join(parts, ",") + "]"
	}
}

// formatRange formats a range into a string
func formatRange(start, end string) string {
	if start == end {
		return start
	}
	return fmt.Sprintf("%s-%s", start, end)
}

// atoi converts a zero-padded string number to an integer
func atoi(s string) int {
	num, _ := strconv.Atoi(s)
	return num
}
