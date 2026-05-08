/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/topograph/pkg/topology"
)

func TestNamedLoader(t *testing.T) {
	name, _ := NamedLoader()
	require.Equal(t, NAME, name)
}

func TestGetParameters(t *testing.T) {
	testCases := []struct {
		name   string
		params map[string]any
		want   *Params
	}{
		{
			name:   "empty params",
			params: nil,
			want:   &Params{},
		},
		{
			name:   "valid outputPath",
			params: map[string]any{"outputPath": "/var/lib/graph/out.json"},
			want:   &Params{OutputPath: "/var/lib/graph/out.json"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := getParameters(tc.params)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoader(t *testing.T) {
	eng, herr := Loader(context.Background(), map[string]any{"outputPath": "/tmp/x"})
	require.Nil(t, herr)
	require.NotNil(t, eng)
	ge, ok := eng.(*GraphEngine)
	require.True(t, ok)
	require.Equal(t, "/tmp/x", ge.params.OutputPath)
}

type mapCache struct {
	data map[string]*topology.Node
	err  error
}

func (c *mapCache) Get(ctx context.Context, key string) (*topology.Node, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.data == nil {
		return nil, nil
	}
	return c.data[key], nil
}

func (c *mapCache) Set(ctx context.Context, instanceID string, instance *topology.Node) error {
	return nil
}

func (c *mapCache) Delete(ctx context.Context, instanceID string) error {
	return nil
}

type mockInstanceProvider struct {
	byID map[string]topology.Node
	err  error
}

func (p *mockInstanceProvider) GetInstances(ctx context.Context, instanceIDs []string) ([]topology.Node, error) {
	if p.err != nil {
		return nil, p.err
	}
	var out []topology.Node
	for _, id := range instanceIDs {
		if r, ok := p.byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestGenerateOutput(t *testing.T) {
	t.Run("missing instance provider in context", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		// Must request at least one instance ID: with nil/empty cis, missing stays empty and
		// GenerateOutput skips the InstanceProvider branch (returns an empty document, no error).
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		_, herr := eng.GenerateOutput(context.Background(), nil, cis, nil, nil)
		require.NotNil(t, herr)
		require.Equal(t, http.StatusInternalServerError, herr.Code())
		require.Equal(t, "instance provider not found", herr.Error())
	})

	t.Run("provider GetInstances error", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		prov := &mockInstanceProvider{err: os.ErrPermission}
		ctx := context.Background()
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		_, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.NotNil(t, herr)
		require.Equal(t, http.StatusInternalServerError, herr.Code())
		require.ErrorContains(t, herr, os.ErrPermission.Error())
	})

	t.Run("cache Get error", func(t *testing.T) {
		eng := &GraphEngine{
			params: &Params{},
			cache:  &mapCache{err: os.ErrClosed},
		}
		prov := &mockInstanceProvider{}
		ctx := context.Background()
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		_, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.NotNil(t, herr)
		require.Equal(t, http.StatusInternalServerError, herr.Code())
		require.ErrorContains(t, herr, os.ErrClosed.Error())
	})

	t.Run("empty compute instances returns empty document", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		prov := &mockInstanceProvider{}
		ctx := context.Background()
		out, herr := eng.GenerateOutput(ctx, nil, nil, nil, prov)
		require.Nil(t, herr)
		var doc topology.Instances
		require.NoError(t, json.Unmarshal(out, &doc))
		require.Empty(t, doc.Instances)
	})

	t.Run("provider omitting requested instance ID returns error", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{
				"n1": {ID: "n1"},
			},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{
			{Instances: map[string]string{"n1": "h1", "n2": "h2"}},
		}
		_, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.NotNil(t, herr)
		require.Equal(t, http.StatusInternalServerError, herr.Code())
		require.ErrorContains(t, herr, `instance provider did not return data for requested instance ID "n2"`)
	})

	t.Run("fetches missing instances and sorts by id", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{
				"z": {ID: "z", Provider: "p"},
				"a": {ID: "a", Provider: "p"},
			},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{
			{Instances: map[string]string{"z": "hz", "a": "ha"}},
		}
		out, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.Nil(t, herr)
		var doc topology.Instances
		require.NoError(t, json.Unmarshal(out, &doc))
		require.Len(t, doc.Instances, 2)
		require.Equal(t, "a", doc.Instances[0].ID)
		require.Equal(t, "z", doc.Instances[1].ID)
	})

	t.Run("cache hit skips provider for cached id", func(t *testing.T) {
		cached := &topology.Node{ID: "cached", Provider: "from-cache"}
		eng := &GraphEngine{
			params: &Params{},
			cache: &mapCache{
				data: map[string]*topology.Node{
					topology.CacheKey("cached"): cached,
				},
			},
		}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{
				"other": {ID: "other", Provider: "from-provider"},
			},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{
			{Instances: map[string]string{"cached": "h1", "other": "h2"}},
		}
		out, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.Nil(t, herr)
		var doc topology.Instances
		require.NoError(t, json.Unmarshal(out, &doc))
		require.Len(t, doc.Instances, 2)
		require.Equal(t, "cached", doc.Instances[0].ID)
		require.Equal(t, "from-cache", doc.Instances[0].Provider)
		require.Equal(t, "other", doc.Instances[1].ID)
	})

	t.Run("no output path returns JSON bytes", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{}}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{"n1": {ID: "n1"}},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		out, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.Nil(t, herr)
		require.True(t, json.Valid(out))
		require.NotEqual(t, "OK\n", string(out))
	})

	t.Run("output path invalid returns error", func(t *testing.T) {
		eng := &GraphEngine{params: &Params{OutputPath: "/no/such/file/graph-out.json"}}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{"n1": {ID: "n1"}},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		_, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.NotNil(t, herr)
		require.Equal(t, http.StatusBadRequest, herr.Code())
		require.ErrorContains(t, herr, "failed to validate")
		require.ErrorContains(t, herr, "graph-out.json")
	})

	t.Run("output path existing file writes JSON and returns OK", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "graph-out-*.json")
		require.NoError(t, err)
		path := f.Name()
		require.NoError(t, f.Close())

		eng := &GraphEngine{params: &Params{OutputPath: path}}
		prov := &mockInstanceProvider{
			byID: map[string]topology.Node{"n1": {ID: "n1", Region: "r1"}},
		}
		ctx := context.Background()
		cis := []topology.ComputeInstances{{Instances: map[string]string{"n1": "h1"}}}
		out, herr := eng.GenerateOutput(ctx, nil, cis, nil, prov)
		require.Nil(t, herr)
		require.Equal(t, "OK\n", string(out))

		written, err := os.ReadFile(path)
		require.NoError(t, err)
		var doc topology.Instances
		require.NoError(t, json.Unmarshal(written, &doc))
		require.Len(t, doc.Instances, 1)
		require.Equal(t, "n1", doc.Instances[0].ID)
		require.Equal(t, "r1", doc.Instances[0].Region)
	})
}

func TestGetComputeInstances(t *testing.T) {
	eng := &GraphEngine{params: &Params{}}
	cis, herr := eng.GetComputeInstances(context.Background(), nil)
	require.Nil(t, herr)
	require.Nil(t, cis)
}
