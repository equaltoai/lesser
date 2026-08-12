package cms

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestCMSDefaultLocalAttributionActorID(t *testing.T) {
	t.Parallel()

	require.Empty(t, cmsDefaultLocalAttributionActorID("", "alice"), "missing domain yields no actor id")
	require.Empty(t, cmsDefaultLocalAttributionActorID("example.com", "  "), "missing username yields no actor id")
	require.Equal(t, "https://example.com/users/alice", cmsDefaultLocalAttributionActorID(" example.com ", " alice "))
}

func TestCMSNormalizeDraftAttributionActedBy(t *testing.T) {
	t.Parallel()

	cmsNormalizeDraftAttribution(nil) // must not panic

	draft := &models.Draft{ActedBy: "  https://example.com/users/alice  "}
	cmsNormalizeDraftAttribution(draft)
	require.Equal(t, "https://example.com/users/alice", draft.ActedBy, "actedBy is trimmed like the other attribution carriers")
}

func TestCMSNormalizeArticleAttributionPreservesExistingActedBy(t *testing.T) {
	t.Parallel()

	article := &models.Article{}
	existing := &models.Article{ActedBy: " https://example.com/users/alice "}
	cmsNormalizeArticleAttribution(article, existing)
	require.Equal(t, "https://example.com/users/alice", article.ActedBy, "empty actedBy inherits the existing attribution")

	article = &models.Article{ActedBy: "https://example.com/users/bob"}
	cmsNormalizeArticleAttribution(article, existing)
	require.Equal(t, "https://example.com/users/bob", article.ActedBy, "explicit actedBy is never overwritten by the existing row")
}

func TestCMSApplyDraftAttributionToArticleActedBy(t *testing.T) {
	t.Parallel()

	cmsApplyDraftAttributionToArticle(nil, &models.Draft{}, "example.com", "agent-one", true) // must not panic
	cmsApplyDraftAttributionToArticle(&models.Article{}, nil, "example.com", "agent-one", true)

	article := &models.Article{}
	draft := &models.Draft{ActedBy: "https://example.com/users/alice"}
	cmsApplyDraftAttributionToArticle(article, draft, "example.com", "agent-one", true)
	require.Equal(t, "https://example.com/users/alice", article.ActedBy, "draft actedBy flows to the published article")
	require.Equal(t, "https://example.com/users/agent-one", article.PublishedBy)

	article = &models.Article{ActedBy: "https://example.com/users/carol"}
	cmsApplyDraftAttributionToArticle(article, &models.Draft{}, "example.com", "agent-one", true)
	require.Equal(t, "https://example.com/users/carol", article.ActedBy, "preserveExisting keeps prior actedBy when the draft carries none")

	cmsApplyDraftAttributionToArticle(article, &models.Draft{}, "example.com", "agent-one", false)
	require.Empty(t, article.ActedBy, "non-preserving publish clears actedBy when the draft carries none")
}
