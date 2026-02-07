package htmlsafe

import (
	"bytes"
	"html"
	"html/template"

	"github.com/equaltoai/lesser/pkg/common"
)

// Escape escapes untrusted text for safe inclusion into HTML contexts.
func Escape(s string) string {
	return html.EscapeString(s)
}

// SanitizeHTMLByContract sanitizes HTML that is meant to be rendered as HTML by clients (e.g. Mastodon fields).
func SanitizeHTMLByContract(content string) string {
	return common.SanitizeContent(content)
}

// RenderTemplate renders an html/template to a string.
func RenderTemplate(name string, tmpl string, data any) (string, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
