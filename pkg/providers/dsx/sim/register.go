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
	"net/http"
	"strings"
	"sync"

	"github.com/NVIDIA/topograph/pkg/providersim"
)

var registerSimOnce sync.Once

// RegisterHTTP mounts the DSX simulation API under prefix (e.g. "dsx-sim" → /dsx-sim/…).
// The [Client] base URL must include the same path segment (see pkg/providers/dsx/provider_sim).
// Safe to call from every [LoaderSim] invocation; registration runs at most once.
func RegisterHTTP(prefix string, reg providersim.HandlerRegistry) {
	if reg == nil {
		return
	}
	p := strings.Trim(strings.TrimSpace(prefix), "/")
	registerSimOnce.Do(func() {
		if p == "" {
			reg.RegisterHandler("/", NewServer().Handler())
			return
		}
		mount := "/" + p
		h := http.StripPrefix(mount, NewServer().Handler())
		reg.RegisterHandler(mount+"/", h)
	})
}
