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
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCredentials(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name  string
		creds map[string]string
		env   map[string]string
		ret   *Credentials
		err   string
	}{
		{
			name:  "Case 1: missing accessKeyId",
			creds: map[string]string{"secretAccessKey": "secret"},
			err:   "credentials error: missing accessKeyId",
		},
		{
			name:  "Case 2: missing secretAccessKey",
			creds: map[string]string{"accessKeyId": "id"},
			err:   "credentials error: missing secretAccessKey",
		},
		{
			name:  "Case 3: valid provided creds",
			creds: map[string]string{"accessKeyId": "id", "secretAccessKey": "secret"},
			ret: &Credentials{
				AccessKeyId:     "id",
				SecretAccessKey: "secret",
			},
		},
		{
			name: "Case 4: valid env creds",
			env: map[string]string{
				"AWS_ACCESS_KEY_ID":     "id",
				"AWS_SECRET_ACCESS_KEY": "secret",
				"AWS_SESSION_TOKEN":     "token",
			},
			ret: &Credentials{
				AccessKeyId:     "id",
				SecretAccessKey: "secret",
				Token:           "token",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for key, val := range tc.env {
				os.Setenv(key, val)
				defer func() { _ = os.Unsetenv(key) }()
			}
			creds, err := getCredentials(ctx, tc.creds)
			if len(tc.err) != 0 {
				require.EqualError(t, err, tc.err)
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.ret, creds)
			}
		})
	}
}
