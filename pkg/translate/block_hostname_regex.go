/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package translate

import (
	"fmt"
	"regexp"
)

// BlockHostnameMatcher is a compiled blockHostnameRegex.
type BlockHostnameMatcher struct {
	raw string
	re  *regexp.Regexp
}

// CompileBlockHostnameRegex compiles a blockHostnameRegex. Returns (nil, nil)
// when raw is empty.
func CompileBlockHostnameRegex(raw string) (*BlockHostnameMatcher, error) {
	return compileBlockHostnameRegex(raw)
}

func compileBlockHostnameRegex(raw string) (*BlockHostnameMatcher, error) {
	if raw == "" {
		return nil, nil
	}
	re, err := regexp.Compile(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid blockHostnameRegex %q: %v", raw, err)
	}
	if n := re.NumSubexp(); n != 1 {
		return nil, fmt.Errorf("blockHostnameRegex %q must contain exactly one capture group, found %d", raw, n)
	}
	return &BlockHostnameMatcher{raw: raw, re: re}, nil
}

// blockRegexDigitsOnly matches non-empty decimal strings.
var blockRegexDigitsOnly = regexp.MustCompile(`^[0-9]+$`)

// MatchDigits returns the decimal capture from hostname. matched is false when
// the regex does not match; err is non-nil only when the capture is non-decimal.
func (r *BlockHostnameMatcher) MatchDigits(hostname string) (digits string, matched bool, err error) {
	if r == nil || r.re == nil {
		return "", false, nil
	}
	match := r.re.FindStringSubmatch(hostname)
	if len(match) < 2 || match[1] == "" {
		return "", false, nil
	}
	digits = match[1]
	if !blockRegexDigitsOnly.MatchString(digits) {
		return "", false, fmt.Errorf("hostname %q capture %q from blockHostnameRegex %q is not a decimal number",
			hostname, digits, r.raw)
	}
	return digits, true, nil
}

// blockIDForDigits returns the Slurm block name for a decimal capture,
// preserving leading zeros so "002" and "2" remain distinct.
func blockIDForDigits(digits string) string {
	return "block" + digits
}

// effectiveBlockHostnameRegex returns the effective cluster-wide value, falling
// back to a unique non-empty per-partition value. Errors when multiple distinct
// values are set.
func (cfg *Config) effectiveBlockHostnameRegex() (string, error) {
	chosen := cfg.BlockHostnameRegex
	chosenSource := "cluster-wide"
	for name, spec := range cfg.Topologies {
		if spec == nil || spec.BlockHostnameRegex == "" {
			continue
		}
		if chosen == "" {
			chosen = spec.BlockHostnameRegex
			chosenSource = fmt.Sprintf("partition %q", name)
			continue
		}
		if spec.BlockHostnameRegex != chosen {
			return "", fmt.Errorf("blockHostnameRegex mismatch: partition %q has %q but %s has %q",
				name, spec.BlockHostnameRegex, chosenSource, chosen)
		}
	}
	return chosen, nil
}
