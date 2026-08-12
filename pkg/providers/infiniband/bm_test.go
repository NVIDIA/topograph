/*
 * Copyright 2024-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package infiniband

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePdshNvidiaSMIOutput(t *testing.T) {
	output := bytes.NewBufferString(`node-1: uuid-1, 7
node-1: uuid-1, 7
malformed
node-2: uuid-2, 8
`)

	outputs, err := parsePdshNvidiaSMIOutput(output)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"node-1": "uuid-1, 7\nuuid-1, 7\n",
		"node-2": "uuid-2, 8\n",
	}, outputs)
}
