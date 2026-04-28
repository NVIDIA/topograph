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

package sim

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvAbsResponseRoot names the directory root for absolute filePath query values.
// When unset, [NewServer] uses [os.TempDir]. Only paths under the effective root (after Clean and symlink checks) are readable.
const EnvAbsResponseRoot = "TOPOGRAPH_DSX_SIM_ABS_RESPONSE_ROOT"

var (
	errAbsolutePathNeedsRoot = fmt.Errorf(
		"absolute filePath requires %s to point at an existing directory", EnvAbsResponseRoot)
	errPathOutsideAbsRoot = fmt.Errorf("path resolves outside %s", EnvAbsResponseRoot)
)

var safeFileStemRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$`)

func validFileStem(stem string) bool {
	return safeFileStemRE.MatchString(stem)
}

// readResponseBytes loads a response by filePath:
//  1. If filePath is an absolute path, read that file only when [absResponseRoot] is non-empty
//     and the path resolves under that directory (no fallback if missing).
//     YAML (.yaml/.yml) is parsed as a tests/models topology model and marshaled to DSX JSON.
//  2. Otherwise resolve tests/models/<stem>.yaml where stem is the basename with optional
//     .json / .yaml / .yml suffix removed, and convert to DSX topology JSON.
func readResponseBytes(absResponseRoot, filePath string) ([]byte, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, errMissingFilePath
	}

	rel := filepath.FromSlash(filePath)
	if filepath.IsAbs(rel) {
		return readResponseBytesAbsolute(absResponseRoot, rel)
	}

	stem := trimModelStem(filepath.Base(rel))
	if stem == "" || !validFileStem(stem) {
		return nil, fs.ErrNotExist
	}
	return responseBytesFromModelFile(stem + ".yaml")
}

func readResponseBytesAbsolute(absResponseRoot, absUserPath string) ([]byte, error) {
	root := strings.TrimSpace(absResponseRoot)
	if root == "" {
		return nil, errAbsolutePathNeedsRoot
	}

	canonical, err := absPathUnderRoot(root, absUserPath)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(canonical)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fs.ErrNotExist
	case err != nil:
		return nil, err
	}

	if isYAMLPath(canonical) {
		return responseBytesFromModelFile(canonical)
	}
	return b, nil
}

// absPathUnderRoot returns a path safe to open under root. It expands OS aliases like
// macOS /var -> /private/var on both sides so containment via filepath.Rel is well-defined.
func absPathUnderRoot(root, userPath string) (string, error) {
	rootEval, err := evalRoot(root)
	if err != nil {
		return "", err
	}

	target, err := canonicalizeAbsForCompare(userPath)
	if err != nil {
		return "", err
	}
	if err := assertUnderRoot(rootEval, target); err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return target, nil
	case err != nil:
		return "", err
	}
	if err := assertUnderRoot(rootEval, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// evalRoot validates the configured response root and returns its symlink-resolved absolute form.
func evalRoot(root string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(root))
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return "", fmt.Errorf("dsx sim: %s %q: %w", EnvAbsResponseRoot, root, err)
	}
	return abs, nil
}

// canonicalizeAbsForCompare expands abs into a symlink-resolved form. EvalSymlinks needs an
// existing path; when abs (or its leaf) does not exist, we resolve the closest existing
// directory ancestor and re-attach the missing suffix.
func canonicalizeAbsForCompare(abs string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(abs))
	if err != nil {
		return "", err
	}

	switch _, err := os.Lstat(abs); {
	case err == nil:
		return filepath.EvalSymlinks(abs)
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}

	for dir := filepath.Dir(abs); ; {
		fi, err := os.Stat(dir)
		switch {
		case err == nil && fi.IsDir():
			return joinResolved(dir, abs)
		case err != nil && !errors.Is(err, os.ErrNotExist):
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		dir = parent
	}
}

// joinResolved returns EvalSymlinks(prefix) joined with the relative path from prefix to abs,
// so the resulting path uses the same canonical form as the symlink-resolved root.
func joinResolved(prefix, abs string) (string, error) {
	eval, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return "", err
	}
	suffix, err := filepath.Rel(prefix, abs)
	if err != nil {
		return "", err
	}
	return filepath.Join(eval, suffix), nil
}

func assertUnderRoot(rootEval, pathAbs string) error {
	rel, err := filepath.Rel(rootEval, pathAbs)
	if err != nil || !filepath.IsLocal(rel) {
		return errPathOutsideAbsRoot
	}
	return nil
}

func trimModelStem(base string) string {
	s := strings.TrimSpace(base)
	s = strings.TrimSuffix(s, ".json")
	s = strings.TrimSuffix(s, ".yaml")
	s = strings.TrimSuffix(s, ".yml")
	return strings.TrimSpace(s)
}

func isYAMLPath(p string) bool {
	lower := strings.ToLower(filepath.Base(p))
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
