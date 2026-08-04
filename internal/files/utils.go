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

package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Validate(name, description string) error {
	if len(name) == 0 {
		return fmt.Errorf("missing filename for %s", description)
	}
	if _, err := os.Stat(name); err != nil {
		return fmt.Errorf("failed to validate %s: %v", name, err)
	}
	return nil
}

// ValidateOutputPath checks whether path is a permitted write destination.
// A bare filename (no directory separator) is always allowed.
// A path with directory components must begin with outputDir; when outputDir is
// empty the current working directory (".") is used as the effective directory.
func ValidateOutputPath(path, outputDir string) error {
	if path == "" {
		return nil
	}
	if filepath.Base(path) == path {
		return nil
	}
	effective := outputDir
	if effective == "" {
		effective = "."
	}
	clean := filepath.Clean(path)
	allowedPrefix := filepath.Clean(effective) + string(filepath.Separator)
	if !strings.HasPrefix(clean, allowedPrefix) {
		return fmt.Errorf("topologyConfigPath %q is outside the configured output directory %q", path, effective)
	}
	return nil
}

func Create(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %q: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to %q: %v", path, err)
	}

	return nil
}
