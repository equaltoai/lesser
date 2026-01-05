package cms

import (
	"testing"

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
		{ID: "section-one-2", Level: 2, Text: "Section One"},
	}, article.TableOfContents)
}

func TestCMSExtractTOCFromMarkdown_NormalizesHeadingText(t *testing.T) {
	content := "## **Bold** [Link](https://example.com)\n"
	entries := cmsExtractTOC(content, "markdown")

	require.Equal(t, []models.TOCEntry{
		{ID: "bold-link", Level: 2, Text: "Bold Link"},
	}, entries)
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
		{ID: "section-2", Level: 2, Text: "Section"},
	}, article.TableOfContents)
}
