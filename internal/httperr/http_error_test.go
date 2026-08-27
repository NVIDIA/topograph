/*
 * Copyright 2025-2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package httperr

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewError(t *testing.T) {
	tests := []struct {
		name string
		code int
		msg  string
	}{
		{name: "bad request", code: http.StatusBadRequest, msg: "invalid input"},
		{name: "not found", code: http.StatusNotFound, msg: "resource not found"},
		{name: "internal error", code: http.StatusInternalServerError, msg: "something went wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError(tt.code, tt.msg)
			require.NotNil(t, err)
			require.Equal(t, tt.code, err.Code())
			require.Equal(t, tt.msg, err.Error())
		})
	}
}
