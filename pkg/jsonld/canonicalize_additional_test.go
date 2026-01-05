package jsonld

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentifierIssuer_GetIDAndClone(t *testing.T) {
	t.Parallel()

	issuer := NewIdentifierIssuer("_:b")
	id1 := issuer.GetID("_:x")
	id2 := issuer.GetID("_:y")
	id1Again := issuer.GetID("_:x")

	require.Equal(t, "_:b0", id1)
	require.Equal(t, "_:b1", id2)
	require.Equal(t, id1, id1Again)

	clone := issuer.Clone()
	require.Equal(t, id1, clone.GetID("_:x"))
	require.Equal(t, id2, clone.GetID("_:y"))
	require.Equal(t, "_:b2", clone.GetID("_:z"))
}

func TestCanonicalize_NQuadsGeneration_Deterministic(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{SkipExpansion: true})
	out, err := c.Canonicalize(map[string]interface{}{
		"@id":    "https://example.com/subject",
		"name":   "Alice",
		"age":    int64(42),
		"active": true,
		"profile": map[string]interface{}{
			"@id": "https://example.com/profile",
			"bio": "hi",
		},
		"tags": []interface{}{"a", "b"},
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.ElementsMatch(t, []string{
		`<https://example.com/profile> <bio> "hi" .`,
		`<https://example.com/subject> <active> "true"^^<http://www.w3.org/2001/XMLSchema#boolean> .`,
		`<https://example.com/subject> <age> "42"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		`<https://example.com/subject> <name> "Alice" .`,
		`<https://example.com/subject> <profile> <https://example.com/profile> .`,
		`<https://example.com/subject> <tags> "a" .`,
		`<https://example.com/subject> <tags> "b" .`,
	}, lines)
}

func TestCanonicalize_NormalizeInputErrorPaths(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{SkipExpansion: true})

	_, err := c.Canonicalize([]byte("{invalid"))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNormalizeInput))

	_, err = c.Canonicalize("{invalid")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNormalizeInput))

	_, err = c.Canonicalize(make(chan int))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNormalizeInput))
}

func TestCanonicalize_UtilityEscapesAndLiterals(t *testing.T) {
	t.Parallel()

	c := NewCanonicalizer(CanonicalizeOptions{})

	require.Equal(t, "<https://example.com>", c.escapeNQuadsValue("https://example.com"))
	require.Equal(t, "_:c14n0", c.canonicalIssuer.GetID("_:b"))
	require.Equal(t, "<name>", c.escapeNQuadsValue("name"))

	require.Equal(t, `"a\"b\nc"`, `"`+c.escapeStringLiteral("a\"b\nc")+`"`)
	require.Contains(t, c.valueToLiteral(int64(1)), "XMLSchema#integer")
	require.Contains(t, c.valueToLiteral(float64(1.5)), "XMLSchema#double")
	require.Contains(t, c.valueToLiteral(true), "XMLSchema#boolean")
}

func TestNormalizeUnicode_InvalidUTF8_ReturnsAsIs(t *testing.T) {
	t.Parallel()

	invalid := string([]byte{0xff, 0xfe, 0xfd})
	require.Equal(t, invalid, NormalizeUnicode(invalid))
}
