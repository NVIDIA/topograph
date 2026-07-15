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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetCredentialsProvider(t *testing.T) {
	testCases := []struct {
		name            string
		creds           map[string]any
		ret             *Credentials
		useDefaultChain bool
		err             string
	}{
		{
			name:  "Case 1: missing accessKeyId",
			creds: map[string]any{"secretAccessKey": "secret"},
			err:   "credentials error: missing 'accessKeyId'",
		},
		{
			name:  "Case 2: missing secretAccessKey",
			creds: map[string]any{"accessKeyId": "id"},
			err:   "credentials error: missing 'secretAccessKey'",
		},
		{
			name:  "Case 3: invalid secretAccessKey",
			creds: map[string]any{"accessKeyId": "id", "secretAccessKey": false},
			err:   "* 'secretAccessKey' expected type 'string'",
		},
		{
			name:  "Case 4: invalid token",
			creds: map[string]any{"accessKeyId": "id", "secretAccessKey": "secret", "token": false},
			err:   "* 'token' expected type 'string'",
		},
		{
			name:  "Case 5: valid provided credentials",
			creds: map[string]any{"accessKeyId": "id", "secretAccessKey": "secret", "token": "token"},
			ret: &Credentials{
				AccessKeyId:     "id",
				SecretAccessKey: "secret",
				Token:           "token",
			},
		},
		{
			name:            "Case 6: default credential chain",
			useDefaultChain: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := getCredentialsProvider(tc.creds)
			if len(tc.err) != 0 {
				require.ErrorContains(t, err, tc.err)
				return
			}

			require.Nil(t, err)
			if tc.useDefaultChain {
				require.Nil(t, provider)
				return
			}

			creds, retrieveErr := provider.Retrieve(context.Background())
			require.NoError(t, retrieveErr)
			require.Equal(t, tc.ret.AccessKeyId, creds.AccessKeyID)
			require.Equal(t, tc.ret.SecretAccessKey, creds.SecretAccessKey)
			require.Equal(t, tc.ret.Token, creds.SessionToken)
		})
	}
}

func TestLoadAWSConfigExplicitCredentialsTakePrecedence(t *testing.T) {
	for key, value := range map[string]string{
		"AWS_ACCESS_KEY_ID":     "environment-id",
		"AWS_SECRET_ACCESS_KEY": "environment-secret",
		"AWS_SESSION_TOKEN":     "environment-token",
	} {
		t.Setenv(key, value)
	}

	provider, httpErr := getCredentialsProvider(map[string]any{
		"accessKeyId":     "explicit-id",
		"secretAccessKey": "explicit-secret",
		"token":           "explicit-token",
	})
	require.Nil(t, httpErr)

	awsCfg, err := loadAWSConfig(context.Background(), "us-east-2", provider)
	require.NoError(t, err)
	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "explicit-id", creds.AccessKeyID)
	require.Equal(t, "explicit-secret", creds.SecretAccessKey)
	require.Equal(t, "explicit-token", creds.SessionToken)
}

func TestLoadAWSConfigUsesEnvironmentCredentials(t *testing.T) {
	for key, value := range map[string]string{
		"AWS_ACCESS_KEY_ID":     "environment-id",
		"AWS_SECRET_ACCESS_KEY": "environment-secret",
		"AWS_SESSION_TOKEN":     "environment-token",
		"AWS_PROFILE":           "",
	} {
		t.Setenv(key, value)
	}

	awsCfg, err := loadAWSConfig(context.Background(), "us-east-2", nil)
	require.NoError(t, err)
	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "environment-id", creds.AccessKeyID)
	require.Equal(t, "environment-secret", creds.SecretAccessKey)
	require.Equal(t, "environment-token", creds.SessionToken)
}

func TestLoadAWSConfigUsesContainerCredentials(t *testing.T) {
	const (
		expiredAccessKeyID   = "expired-pod-identity-access-key"
		refreshedAccessKeyID = "refreshed-pod-identity-access-key"
		secretAccessKey      = "pod-identity-secret-key"
		sessionToken         = "pod-identity-session-token"
		authToken            = "pod-identity-authorization-token"
	)

	var requestCount atomic.Int32
	authorization := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")

		accessKeyID := refreshedAccessKeyID
		expiration := time.Now().Add(time.Hour)
		if requestCount.Add(1) == 1 {
			accessKeyID = expiredAccessKeyID
			expiration = time.Now().Add(-time.Minute)
		}

		_, _ = fmt.Fprintf(w, `{
			"AccessKeyId": %q,
			"SecretAccessKey": %q,
			"Token": %q,
			"Expiration": %q
		}`, accessKeyID, secretAccessKey, sessionToken, expiration.UTC().Format(time.RFC3339))
	}))
	t.Cleanup(server.Close)

	tokenPath := filepath.Join(t.TempDir(), "eks-pod-identity-token")
	require.NoError(t, os.WriteFile(tokenPath, []byte(authToken), 0o600))

	for key, value := range map[string]string{
		"AWS_ACCESS_KEY_ID":                      "",
		"AWS_SECRET_ACCESS_KEY":                  "",
		"AWS_SESSION_TOKEN":                      "",
		"AWS_WEB_IDENTITY_TOKEN_FILE":            "",
		"AWS_ROLE_ARN":                           "",
		"AWS_PROFILE":                            "",
		"AWS_CONFIG_FILE":                        filepath.Join(t.TempDir(), "config"),
		"AWS_SHARED_CREDENTIALS_FILE":            filepath.Join(t.TempDir(), "credentials"),
		"AWS_EC2_METADATA_DISABLED":              "true",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI":     server.URL,
		"AWS_CONTAINER_AUTHORIZATION_TOKEN":      "",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE": tokenPath,
	} {
		t.Setenv(key, value)
	}

	awsCfg, err := loadAWSConfig(context.Background(), "us-east-2", nil)
	require.NoError(t, err)
	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, expiredAccessKeyID, creds.AccessKeyID)
	require.Equal(t, secretAccessKey, creds.SecretAccessKey)
	require.Equal(t, sessionToken, creds.SessionToken)
	require.Equal(t, authToken, <-authorization)

	creds, err = awsCfg.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, refreshedAccessKeyID, creds.AccessKeyID)
	require.Equal(t, secretAccessKey, creds.SecretAccessKey)
	require.Equal(t, sessionToken, creds.SessionToken)
	require.Equal(t, authToken, <-authorization)
	require.Equal(t, int32(2), requestCount.Load())
}
