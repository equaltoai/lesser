package jsonld

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNQuads_ArrayOfObjects_ExercisesValueToNQuadsMapBranch(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{SkipExpansion: true})
	out, err := c.Canonicalize(map[string]interface{}{
		"@id": "https://example.com/root",
		"items": []interface{}{
			map[string]interface{}{
				"name": "x",
			},
		},
	})
	require.NoError(t, err)

	// Nested item has no @id, so it should be represented as a blank node with a linking quad.
	require.Contains(t, string(out), "<https://example.com/root> <items> _:")
	require.Contains(t, string(out), "<name> \"x\" .")
}

func TestEscapeHelpers_CoverMoreBranches(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{})
	require.Equal(t, "_:b0", c.escapeNQuadsValue("_:b0"))

	escaped := c.escapeStringLiteral("a\\b\r\t" + string([]byte{0x1F}))
	require.Contains(t, escaped, `\\`)
	require.Contains(t, escaped, `\r`)
	require.Contains(t, escaped, `\t`)
	require.Contains(t, escaped, `\u`)
}

func TestNormalizeValue_NumberConversions(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{})
	require.Equal(t, int64(5), c.normalizeValue(float64(5)))
	require.Equal(t, float64(5.5), c.normalizeValue(float64(5.5)))
	require.Equal(t, int64(3), c.normalizeValue(int(3)))
	require.Equal(t, int64(3), c.normalizeValue(int32(3)))
}

func TestRemoveSignatureFields_Slices(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{
		RemoveSignatureFields: true,
		SignatureFields:       []string{"signature"},
	})

	out := c.removeSignatureFields([]interface{}{
		map[string]interface{}{"signature": "x", "keep": "y"},
	})

	items, ok := out.([]interface{})
	require.True(t, ok)
	require.Len(t, items, 1)
	item, ok := items[0].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, item, "signature")
	require.Equal(t, "y", item["keep"])
}

func TestCanonicalizeToJSON_MarshalErrorSurfaces(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{SkipExpansion: true})
	_, err := c.CanonicalizeToJSON(map[string]interface{}{"bad": make(chan int)})
	require.Error(t, err)
}

func TestToNQuads_IgnoresUnsupportedRootTypes(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{})
	quads, err := c.toNQuads("not a map or slice")
	require.NoError(t, err)
	require.Len(t, quads, 0)
}

func TestCanonicalize_AddsTrailingNewlineWhenNonEmpty(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{SkipExpansion: true})
	out, err := c.Canonicalize(map[string]interface{}{"k": "v"})
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(out), "\n"))
}
