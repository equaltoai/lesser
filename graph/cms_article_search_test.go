package graph

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestCMSArticleMatchesSearchUsesOnlyPublicArticleText(t *testing.T) {
	t.Parallel()

	article := &models.Article{
		Object: models.Object{
			Name:    "A Federated Publishing Guide",
			Summary: "Canonical long-form content",
			Content: "# ActivityPub\n\nRendered by Lesser.",
		},
		Slug:           "federated-publishing",
		Subtitle:       "For independent publishers",
		Excerpt:        "A practical introduction",
		EditorNotes:    "confidential launch phrase",
		ReviewStatus:   "internal-only-state",
		SEOTitle:       "Federated publishing on Lesser",
		SEODescription: "Find ActivityPub articles",
	}

	for _, search := range []string{
		"federated publishing",
		"CANONICAL LONG-FORM",
		"activitypub",
		"rendered by lesser",
		"independent publishers",
		"practical introduction",
		"federated-publishing",
		"publishing on lesser",
		"find activitypub",
	} {
		require.True(t, cmsArticleMatchesSearch(article, search), search)
	}

	require.True(t, cmsArticleMatchesSearch(article, ""), "empty search preserves the unfiltered query")
	require.False(t, cmsArticleMatchesSearch(article, "confidential launch phrase"), "private editor notes must not affect public search results")
	require.False(t, cmsArticleMatchesSearch(article, "internal-only-state"), "private review state must not affect public search results")
	require.False(t, cmsArticleMatchesSearch(nil, "activitypub"))
}
