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

var errNoAuth = errors.New("missing or invalid Authorization")

// requireBearerScheme ensures the request has a non-empty Bearer token (simulation only:
// the token value is not interpreted — response files come from the filePath query parameter).
func requireBearerScheme(r *http.Request) error {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return errNoAuth
	}
	fields := strings.Fields(h)
	if len(fields) != 2 || strings.ToLower(fields[0]) != "bearer" {
		return errNoAuth
	}
	if fields[1] == "" {
		return errNoAuth
	}
	return nil
}
