package factories

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBaseBuilder_Generators(t *testing.T) {
	b := &BaseBuilder{
		domain:   "example.com",
		sequence: 1,
		baseTime: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	require.Equal(t, 1, b.NextSequence())
	require.Equal(t, 2, b.NextSequence())

	// After two NextSequence calls, sequence is now 3.
	require.Equal(t, "https://example.com/users/3", b.GenerateID("users"))

	ts := b.GenerateTimestamp()
	require.Equal(t, time.Date(2025, 1, 2, 0, 4, 0, 0, time.UTC), ts)
}

func TestWithDefaults(t *testing.T) {
	out := WithDefaults(
		map[string]interface{}{
			"a": "",
			"b": 2,
			"c": nil,
			"d": false,
		},
		map[string]interface{}{
			"a": "x",
			"b": 1,
		},
	)

	require.Equal(t, "x", out["a"])
	require.Equal(t, 2, out["b"])
	require.Equal(t, false, out["d"])
	_, ok := out["c"]
	require.False(t, ok)
}

func TestFluentSetter(t *testing.T) {
	type thing struct {
		Name string
		N    int
	}

	target := &thing{}
	got := NewFluentSetter(target).
		Set(func(t *thing) { t.Name = "ok" }).
		Set(func(t *thing) { t.N = 42 }).
		Build()

	require.Same(t, target, got)
	require.Equal(t, "ok", got.Name)
	require.Equal(t, 42, got.N)
}
