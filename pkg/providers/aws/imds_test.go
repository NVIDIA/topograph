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

package aws

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInstanceOutput(t *testing.T) {
	input := `node1: instance1
node2: instance2
node3: instance3
node4: instance4
`
	expected := map[string]string{"instance1": "node1", "instance2": "node2", "instance3": "node3", "instance4": "node4"}

	output, err := parseInstanceOutput(bytes.NewBufferString(input))
	require.NoError(t, err)
	require.Equal(t, expected, output)
}

func TestPdshCmd(t *testing.T) {
	expected := fmt.Sprintf(`TOKEN=$(curl -s -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 60" %s); echo $(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" %s)`, IMDSTokenURL, IMDSInstanceURL)
	require.Equal(t, expected, pdshCmd(IMDSInstanceURL))
}
