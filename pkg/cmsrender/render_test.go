package cmsrender

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderArticleContentMarkdownSanitizesUnsafeHTML(t *testing.T) {
	source := strings.Join([]string{
		"# Hello Lesser",
		"",
		"Welcome to **federated** publishing with [a link](https://example.com/path).",
		"",
		"![alt text](https://cdn.example.com/image.png)",
		"",
		"```go",
		"fmt.Println(\"hi\")",
		"```",
		"",
		"<script>alert('xss')</script>",
		"<a href=\"javascript:alert(1)\" onclick=\"alert(2)\">bad</a>",
	}, "\n")

	rendered, err := RenderArticleContent(source, FormatMarkdown)
	require.NoError(t, err)
	require.Equal(t, FormatMarkdown, rendered.SourceFormat)
	require.Contains(t, rendered.HTML, `<h1 id="hello-lesser">Hello Lesser</h1>`)
	require.Contains(t, rendered.HTML, `<strong>federated</strong>`)
	require.Contains(t, rendered.HTML, `<a href="https://example.com/path"`)
	require.Contains(t, rendered.HTML, `<img src="https://cdn.example.com/image.png" alt="alt text"`) // goldmark preserves safe image markup
	require.Contains(t, rendered.HTML, `<pre><code class="language-go">`)
	require.NotContains(t, rendered.HTML, "<script")
	require.NotContains(t, rendered.HTML, "onclick")
	require.NotContains(t, rendered.HTML, "javascript:")
}

func TestRenderArticleContentHTMLPassesThroughSanitizer(t *testing.T) {
	source := `<h2 id="intro" onclick="evil()">Intro</h2><p>Body <em>ok</em></p><img src="https://cdn.example.com/a.png" onerror="evil()"><iframe src="https://evil.example"></iframe>`

	rendered, err := RenderArticleContent(source, "HTML")
	require.NoError(t, err)
	require.Equal(t, FormatHTML, rendered.SourceFormat)
	require.Contains(t, rendered.HTML, `<h2 id="intro">Intro</h2>`)
	require.Contains(t, rendered.HTML, `<p>Body <em>ok</em></p>`)
	require.Contains(t, rendered.HTML, `<img src="https://cdn.example.com/a.png">`)
	require.NotContains(t, rendered.HTML, "onclick")
	require.NotContains(t, rendered.HTML, "onerror")
	require.NotContains(t, rendered.HTML, "iframe")
}

func TestRenderArticleContentDeterministicErrors(t *testing.T) {
	_, err := RenderArticleContent("body", "asciidoc")
	require.ErrorIs(t, err, ErrUnsupportedContentFormat)
	require.Contains(t, err.Error(), "asciidoc")

	_, err = RenderArticleContent(strings.Repeat("a", MaxArticleSourceBytes+1), FormatMarkdown)
	require.ErrorIs(t, err, ErrArticleContentTooLarge)
	var limitErr *LimitError
	require.True(t, errors.As(err, &limitErr))
	require.Equal(t, MaxArticleSourceBytes, limitErr.Limit)
	require.Equal(t, MaxArticleSourceBytes+1, limitErr.Actual)
}

func TestRenderArticleContentEmptyAndMalformed(t *testing.T) {
	rendered, err := RenderArticleContent("", FormatMarkdown)
	require.NoError(t, err)
	require.Empty(t, rendered.HTML)

	rendered, err = RenderArticleContent("<p><strong>open", FormatHTML)
	require.NoError(t, err)
	require.Equal(t, "<p><strong>open", rendered.HTML)
}
