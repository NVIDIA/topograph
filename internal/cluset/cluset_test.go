package cluset

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactExpand(t *testing.T) {
	testCases := []struct {
		name                string
		expanded, compacted []string
	}{
		{
			name: "Case 1: empty list",
		},
		{
			name:      "Case 2: ranges",
			expanded:  []string{"abc0507", "abc0509", "abc0482", "124", "abc0483", "abc0508", "abc0484", "123"},
			compacted: []string{"[123-124]", "abc[0482-0484,0507-0509]"},
		},
		{
			name:      "Case 3: singles",
			expanded:  []string{"abc0507", "abc0509", "xyz0482"},
			compacted: []string{"abc[0507,0509]", "xyz0482"},
		},
		{
			name:      "Case 4: mix1",
			expanded:  []string{"abc0507", "abc0509", "def", "abc0482", "abc0508"},
			compacted: []string{"abc[0482,0507-0509]", "def"},
		},
		{
			name:      "Case 5: mix2",
			expanded:  []string{"abc0507", "abc0509", "abc0508", "abc0482", "xyz8", "xyz9", "xyz10"},
			compacted: []string{"abc[0482,0507-0509]", "xyz[8-10]"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.compacted, Compact(tc.expanded))
			require.Equal(t, toMap(tc.expanded), toMap(Expand(tc.compacted)))
		})
	}
}

func toMap(arr []string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, str := range arr {
		m[str] = struct{}{}
	}
	return m
}
