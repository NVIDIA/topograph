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
	"net/http"
	"strings"
)

// QueryParamFilePath is the query key that selects a response file (path or name).
const QueryParamFilePath = "filePath"

var errMissingFilePath = errors.New("missing filePath query parameter")

// filePathFromRequest returns the raw filePath query value (trimmed). The path may be
// relative to the server responses directory or absolute; see [readResponseBytes].
func filePathFromRequest(r *http.Request) (string, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(QueryParamFilePath))
	if raw == "" {
		return "", errMissingFilePath
	}
	return raw, nil
}
