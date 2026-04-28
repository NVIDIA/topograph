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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAbsPathUnderRoot_ok(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.json")
	require.NoError(t, os.WriteFile(f, []byte("{}"), 0o644))

	abs, err := filepath.Abs(f)
	require.NoError(t, err)

	got, err := absPathUnderRoot(dir, abs)
	require.NoError(t, err)
	st1, err := os.Stat(abs)
	require.NoError(t, err)
	st2, err := os.Stat(got)
	require.NoError(t, err)
	require.True(t, os.SameFile(st1, st2), "expected same file for %q and %q", abs, got)
}

func TestAbsPathUnderRoot_rejectsOutside(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	f := filepath.Join(outside, "x")
	require.NoError(t, os.WriteFile(f, []byte{}, 0o644))
	abs, err := filepath.Abs(f)
	require.NoError(t, err)

	_, err = absPathUnderRoot(dir, abs)
	require.ErrorIs(t, err, errPathOutsideAbsRoot)
}
