package cmsrender

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func inlineMedia(position int, url string) ArticleMedia {
	return ArticleMedia{Role: ArticleMediaRoleInline, InlinePosition: position, URL: url, AltText: "inline alt"}
}

func TestComposeInlineMediaAtPositions(t *testing.T) {
	source := strings.Join([]string{
		"# Title",
		"",
		"First paragraph.",
		"",
		"Second paragraph.",
		"",
		"Third paragraph.",
	}, "\n")

	t.Run("position zero leads the first block", func(t *testing.T) {
		rendered, err := RenderArticleContentWithMedia(source, FormatMarkdown, []ArticleMedia{inlineMedia(0, "https://cdn.example.test/a.png")})
		require.NoError(t, err)
		imgIndex := strings.Index(rendered.HTML, `<img src="https://cdn.example.test/a.png"`)
		titleIndex := strings.Index(rendered.HTML, `<h1 id="title">Title</h1>`)
		require.Greater(t, imgIndex, -1)
		require.Greater(t, titleIndex, -1)
		require.Less(t, imgIndex, titleIndex, "position 0 inserts before the first block")
	})

	t.Run("position one inserts after the first block", func(t *testing.T) {
		rendered, err := RenderArticleContentWithMedia(source, FormatMarkdown, []ArticleMedia{inlineMedia(1, "https://cdn.example.test/a.png")})
		require.NoError(t, err)
		imgIndex := strings.Index(rendered.HTML, `<img src="https://cdn.example.test/a.png"`)
		require.Greater(t, imgIndex, strings.Index(rendered.HTML, "</h1>"), "position 1 inserts after the title block")
		require.Less(t, imgIndex, strings.Index(rendered.HTML, "First paragraph"), "position 1 inserts before the second block")
	})

	t.Run("position past the block count appends at the end", func(t *testing.T) {
		rendered, err := RenderArticleContentWithMedia(source, FormatMarkdown, []ArticleMedia{inlineMedia(99, "https://cdn.example.test/a.png")})
		require.NoError(t, err)
		imgIndex := strings.Index(rendered.HTML, `<img src="https://cdn.example.test/a.png"`)
		require.Greater(t, imgIndex, -1)
		lastBlock := strings.LastIndex(rendered.HTML, "</p>")
		require.Greater(t, imgIndex, lastBlock, "out-of-range position appends after the last block")
	})
}

func TestComposeMultipleInlineOrderedByPosition(t *testing.T) {
	source := "# T\n\nOne.\n\nTwo.\n\nThree."
	media := []ArticleMedia{
		inlineMedia(2, "https://cdn.example.test/two.png"),
		inlineMedia(0, "https://cdn.example.test/zero.png"),
		inlineMedia(1, "https://cdn.example.test/one.png"),
	}
	rendered, err := RenderArticleContentWithMedia(source, FormatMarkdown, media)
	require.NoError(t, err)

	zero := strings.Index(rendered.HTML, "zero.png")
	one := strings.Index(rendered.HTML, "one.png")
	two := strings.Index(rendered.HTML, "two.png")
	require.True(t, zero < one && one < two, "inline media composes in position order: %d < %d < %d", zero, one, two)

	// Insertion point N places the figure before block N: position 0 leads the
	// title, position 1 sits between the title and the first paragraph, and
	// position 2 between the first and second paragraphs.
	titleEnd := strings.Index(rendered.HTML, "</h1>")
	firstPara := strings.Index(rendered.HTML, "One.")
	secondPara := strings.Index(rendered.HTML, "Two.")
	require.True(t, zero < strings.Index(rendered.HTML, "<h1"), "position 0 leads the title block")
	require.True(t, one > titleEnd && one < firstPara, "position 1 inserts between title and first paragraph")
	require.True(t, two > firstPara && two < secondPara, "position 2 inserts between first and second paragraphs")
}

func TestComposeHeroLeadsArticle(t *testing.T) {
	source := "# T\n\nBody paragraph."
	media := []ArticleMedia{
		inlineMedia(0, "https://cdn.example.test/inline.png"),
		{Role: ArticleMediaRoleHero, URL: "https://cdn.example.test/hero.png", AltText: "hero alt"},
	}
	rendered, err := RenderArticleContentWithMedia(source, FormatMarkdown, media)
	require.NoError(t, err)

	hero := strings.Index(rendered.HTML, "hero.png")
	inline := strings.Index(rendered.HTML, "inline.png")
	title := strings.Index(rendered.HTML, "<h1")
	require.Greater(t, hero, -1)
	require.True(t, hero < title && hero < inline, "hero is the leading image before inline media and the first block")
}

func TestComposeSocialCardNeverEmitsIntoBody(t *testing.T) {
	rendered, err := RenderArticleContentWithMedia(
		"# T\n\nBody.",
		FormatMarkdown,
		[]ArticleMedia{{Role: ArticleMediaRoleSocialCard, URL: "https://cdn.example.test/card.png"}},
	)
	require.NoError(t, err)
	require.NotContains(t, rendered.HTML, "card.png", "social-card media never composes into the article body")
}

func TestComposeFigureEmitsCaptionAndCreditAsText(t *testing.T) {
	media := []ArticleMedia{{
		Role:           ArticleMediaRoleInline,
		InlinePosition: 0,
		URL:            "https://cdn.example.test/a.png",
		AltText:        "a rocket",
		Caption:        "Launch artwork",
		CreditLine:     "Illustration by Alice",
		Width:          640,
		Height:         480,
	}}
	rendered, err := RenderArticleContentWithMedia("# T\n\nBody.", FormatMarkdown, media)
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, "<figure>")
	require.Contains(t, rendered.HTML, `alt="a rocket"`)
	require.Contains(t, rendered.HTML, `width="640"`)
	require.Contains(t, rendered.HTML, `height="480"`)
	require.Contains(t, rendered.HTML, "<figcaption>")
	require.Contains(t, rendered.HTML, "Launch artwork")
	require.Contains(t, rendered.HTML, "Illustration by Alice")
	require.Contains(t, rendered.HTML, "Launch artwork — Illustration by Alice")
}

func TestComposeSanitizesComposedHTML(t *testing.T) {
	media := []ArticleMedia{{
		Role:           ArticleMediaRoleInline,
		InlinePosition: 0,
		URL:            "https://cdn.example.test/a.png",
		AltText:        `<img src=x onerror="alert(1)">`,
		Caption:        `<script>alert("xss")</script>caption`,
	}}
	rendered, err := RenderArticleContentWithMedia("# T\n\nBody.", FormatMarkdown, media)
	require.NoError(t, err)
	require.NotContains(t, rendered.HTML, "<script")
	// The raw unescaped attribute form must never appear; the escaped literal
	// text of the alt value is inert.
	require.NotContains(t, rendered.HTML, ` onerror="`)
	// Escaped text survives as literal text.
	require.Contains(t, rendered.HTML, "caption")
	require.Contains(t, rendered.HTML, "alt=")
}

func TestComposeOnlyEmitsMintedHTTPSURLs(t *testing.T) {
	media := []ArticleMedia{
		inlineMedia(0, "javascript:alert(1)"),
		inlineMedia(1, "https://cdn.example.test/good.png"),
	}
	rendered, err := RenderArticleContentWithMedia("# T\n\nBody.", FormatMarkdown, media)
	require.NoError(t, err)
	require.NotContains(t, rendered.HTML, "javascript:")
	require.Contains(t, rendered.HTML, "good.png")
}

func TestComposeWithMediaMatchesPlainRenderWhenEmpty(t *testing.T) {
	source := strings.Join([]string{
		"# Hello",
		"",
		"Some **bold** text with [a link](https://example.com).",
		"",
		"<script>alert('x')</script>",
	}, "\n")
	for _, format := range []string{FormatMarkdown, FormatHTML} {
		plain, err := RenderArticleContent(source, format)
		require.NoError(t, err)
		composed, err := RenderArticleContentWithMedia(source, format, nil)
		require.NoError(t, err)
		require.Equal(t, plain, composed, "empty media composition must be byte-identical to the plain render for %s", format)
	}
}

func TestComposeHTMLFormat(t *testing.T) {
	source := `<h2 id="intro">Intro</h2><p>Body <em>ok</em></p>`
	rendered, err := RenderArticleContentWithMedia(source, FormatHTML, []ArticleMedia{inlineMedia(1, "https://cdn.example.test/a.png")})
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, `<h2 id="intro">Intro</h2>`)
	require.Contains(t, rendered.HTML, `<p>Body <em>ok</em></p>`)
	require.Contains(t, rendered.HTML, "a.png")
	imgIndex := strings.Index(rendered.HTML, "a.png")
	require.Greater(t, imgIndex, strings.Index(rendered.HTML, "</h2>"), "position 1 inserts after the first block (h2)")
	require.Less(t, imgIndex, strings.Index(rendered.HTML, "Body"), "position 1 inserts before the second block (p)")
}

func TestComposeRespectsRenderedSizeCap(t *testing.T) {
	// Blockquoted emphasis expands ~4x when rendered; a source under the
	// 256 KiB source cap renders past the 512 KiB output cap once composed.
	source := strings.Repeat("> **word**\n\n", 15000)
	media := []ArticleMedia{inlineMedia(0, "https://cdn.example.test/a.png")}
	_, err := RenderArticleContentWithMedia(source, FormatMarkdown, media)
	require.ErrorIs(t, err, ErrArticleRenderedContentTooLarge)
	require.IsType(t, &LimitError{}, err)
}
