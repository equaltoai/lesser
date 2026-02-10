package htmlsafe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscape(t *testing.T) {
	require.Equal(t, "&lt;div&gt;hello&lt;/div&gt;", Escape("<div>hello</div>"))
}

func TestSanitizeHTMLByContract_PlainTextIsStable(t *testing.T) {
	require.Equal(t, "hello", SanitizeHTMLByContract("hello"))
}

func TestRenderTemplate(t *testing.T) {
	out, err := RenderTemplate("ok", "Hello, {{.Name}}", struct {
		Name string
	}{Name: "world"})
	require.NoError(t, err)
	require.Equal(t, "Hello, world", out)

	_, err = RenderTemplate("parse-error", "{{", nil)
	require.Error(t, err)

	_, err = RenderTemplate("exec-error", "{{.Missing}}", struct {
		Name string
	}{Name: "world"})
	require.Error(t, err)
}
