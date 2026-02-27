/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package gcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetProjectID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		content   []byte
		params    map[string]any
		err       string
		projectID string
	}{
		{
			name:      "Case 1: valid credentials file with params",
			params:    map[string]any{"project_id": "test-project1"},
			content:   []byte(`{"project_id": "test-project2"}`),
			projectID: "test-project1",
		},
		{
			name:      "Case 2: valid credentials file without params",
			content:   []byte(`{"project_id": "test-project"}`),
			projectID: "test-project",
		},
		{
			name:    "Case 3: invalid project_id in params",
			params:  map[string]any{"project_id": false},
			content: []byte(`{"project_id": "test-project"}`),
			err:     "project_id in provider parameters must be a string",
		},
		{
			name: "Case 4: invalid credentials file path",
			err:  "GOOGLE_APPLICATION_CREDENTIALS is not a file: stat /does/not/exist: no such file or directory",
		},
		{
			name:    "Case 5: invalid content",
			content: []byte("{invalid-json"),
			err:     "failed to parse GOOGLE_APPLICATION_CREDENTIALS: invalid character 'i' looking for beginning of object key string",
		},
		{
			name:    "Case 6: missing project_id",
			content: []byte(`{"key": "val"}`),
			err:     "missing project_id in GOOGLE_APPLICATION_CREDENTIALS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.content == nil {
				filePath = "/does/not/exist"
			} else {
				filePath = filepath.Join(t.TempDir(), "creds.json")
				err := os.WriteFile(filePath, tt.content, 0600)
				require.NoError(t, err)
			}

			t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filePath)

			projectID, err := getProjectID(ctx, tt.params)

			if len(tt.err) != 0 {
				require.EqualError(t, err, tt.err)
			} else {
				require.Equal(t, tt.projectID, projectID)
			}
		})
	}
}
