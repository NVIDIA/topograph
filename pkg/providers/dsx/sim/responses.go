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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var safeFileStemRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,254}$`)

func validFileStem(stem string) bool {
	return safeFileStemRE.MatchString(stem)
}

// readResponseBytes loads a response by filePath:
//  1. If filePath is an absolute path, read that file only (no embed fallback if missing).
//  2. Otherwise read embedded responses/<stem>.json where stem is the basename of filePath
//     with an optional .json suffix removed.
func readResponseBytes(embedFS fs.FS, filePath string) ([]byte, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, errMissingFilePath
	}

	rel := filepath.FromSlash(filePath)

	if filepath.IsAbs(rel) {
		clean := filepath.Clean(rel)
		b, err := os.ReadFile(clean)
		if err == nil {
			return b, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fs.ErrNotExist
	}

	base := filepath.Base(rel)
	stem := strings.TrimSuffix(base, ".json")
	stem = strings.TrimSpace(stem)
	if stem == "" || !validFileStem(stem) {
		return nil, fs.ErrNotExist
	}
	if embedFS == nil {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(embedFS, path.Join("responses", stem+".json"))
}
