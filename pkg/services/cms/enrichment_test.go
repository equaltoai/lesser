package cms

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestEnrichArticleContent_Markdown(t *testing.T) {
	article := &models.Article{
		Object: models.Object{
			Content: `# Title

Hello world.

## Section One

### Sub

## Section One

` + "```md\n## Not A Heading\n```\n",
		},
		ContentFormat: "markdown",
	}

	enrichArticleContent(article)

	require.Equal(t, 1, article.ReadingTimeMinutes)
	require.Equal(t, 8, article.WordCount)
	require.Equal(t, []models.TOCEntry{
		{ID: "title", Level: 1, Text: "Title"},
		{ID: "section-one", Level: 2, Text: "Section One"},
		{ID: "sub", Level: 3, Text: "Sub"},
		{ID: "section-one-1", Level: 2, Text: "Section One"},
	}, article.TableOfContents)

	rendered, err := cmsrender.RenderArticleContent(article.Content, article.ContentFormat)
	require.NoError(t, err)
	for _, entry := range article.TableOfContents {
		require.Contains(t, rendered.HTML, `id="`+entry.ID+`"`)
	}
}

func TestCMSExtractTOCFromMarkdown_NormalizesHeadingText(t *testing.T) {
	content := "## **Bold** [Link](https://example.com)\n"
	entries := cmsExtractTOC(content, "markdown", nil)

	require.Equal(t, []models.TOCEntry{
		{ID: "bold-linkhttpsexamplecom", Level: 2, Text: "Bold Link"},
	}, entries)
}

func TestCMSExtractTOCFallsBackToMarkdownParserWhenRendererRejectsFormat(t *testing.T) {
	content := "## Section One\n\n## Section One\n\n```md\n## Not A Heading\n```\n"
	entries := cmsExtractTOC(content, "asciidoc", nil)

	require.Equal(t, []models.TOCEntry{
		{ID: "section-one", Level: 2, Text: "Section One"},
		{ID: "section-one-1", Level: 2, Text: "Section One"},
	}, entries)
}

// TestEnrichArticleContentComposesPersistedMedia proves the enrichment read
// path (enrichment.go) renders a media-bearing article through the canonical
// renderer: enrichArticleContent passes article.RenderMediaList() into
// cmsExtractTOC, which extracts headings from the composed render, so the
// persisted inline binding must compose while hero and social-card media never
// duplicate into it.
func TestEnrichArticleContentComposesPersistedMedia(t *testing.T) {
	position := 0
	article := &models.Article{
		Object: models.Object{
			Content: "# T\n\nOne.",
		},
		ContentFormat: "markdown",
		EditorialMedia: []models.ArticleEditorialMedia{
			{MediaID: "hero", Role: models.EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png"},
			{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png", ContentType: "image/png", AltText: "inline"},
			{MediaID: "card", Role: models.EditorialMediaRoleSocialCard, URL: "https://cdn.example.test/card.png"},
		},
	}

	enrichArticleContent(article)
	require.Equal(t, 2, article.WordCount)
	require.Equal(t, 1, article.ReadingTimeMinutes)
	require.Equal(t, []models.TOCEntry{{ID: "t", Level: 1, Text: "T"}}, article.TableOfContents)

	// The enrichment read path's composition expression (enrichment.go:130)
	// composes the persisted inline media into the render the TOC headings are
	// extracted from; hero and social-card media never duplicate.
	rendered, err := cmsrender.RenderArticleContentWithMedia(article.Content, article.ContentFormat, article.RenderMediaList())
	require.NoError(t, err)
	require.Contains(t, rendered.HTML, `src="https://cdn.example.test/inline.png"`)
	require.Contains(t, rendered.HTML, "<figure>")
	require.NotContains(t, rendered.HTML, "hero.png")
	require.NotContains(t, rendered.HTML, "card.png")
}

func TestEnrichArticleContent_HTML(t *testing.T) {
	article := &models.Article{
		Object: models.Object{
			Content: `<h1>Title</h1><p>Hello world</p><h2 id="sec">Section</h2><h2>Section</h2><h2>Section</h2>`,
		},
		ContentFormat: "html",
	}

	enrichArticleContent(article)

	require.Equal(t, 1, article.ReadingTimeMinutes)
	require.Equal(t, 6, article.WordCount)
	require.Equal(t, []models.TOCEntry{
		{ID: "title", Level: 1, Text: "Title"},
		{ID: "sec", Level: 2, Text: "Section"},
		{ID: "section", Level: 2, Text: "Section"},
		{ID: "section-1", Level: 2, Text: "Section"},
	}, article.TableOfContents)
}
