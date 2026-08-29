package transformations

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// TestStorageArticleToActivityPubComposesPersistedMedia proves the AP article
// read path composes the persisted inline bindings into content and attaches
// the minted servings as Document attachments, while the hero stays on
// featuredImage (never duplicated into the body).
func TestStorageArticleToActivityPubComposesPersistedMedia(t *testing.T) {
	position := 0
	article := &storagemodels.Article{
		Object: storagemodels.Object{
			ID:      "https://example.com/articles/1",
			Type:    activitypub.ArticleType,
			Content: "# Title\n\nBody.",
			Name:    "Title",
		},
		ContentFormat: "markdown",
		EditorialMedia: []storagemodels.ArticleEditorialMedia{
			{MediaID: "hero", Role: storagemodels.EditorialMediaRoleHero, URL: "https://cdn.example.test/hero.png", ContentType: "image/png", AltText: "hero", Width: 1200, Height: 630},
			{MediaID: "inline", Role: storagemodels.EditorialMediaRoleInline, InlinePosition: &position, URL: "https://cdn.example.test/inline.png", ContentType: "image/jpeg", AltText: "inline", Width: 640, Height: 480},
			{MediaID: "card", Role: storagemodels.EditorialMediaRoleSocialCard, URL: "https://cdn.example.test/card.png", ContentType: "image/png"},
		},
	}

	apArticle, err := StorageArticleToActivityPub(article)
	require.NoError(t, err)
	require.Equal(t, activitypub.ArticleType, apArticle.Type)

	// Inline media composes into the sanitized content; hero and social-card
	// media never duplicate into the body.
	require.Contains(t, apArticle.Content, `src="https://cdn.example.test/inline.png"`)
	require.NotContains(t, apArticle.Content, "hero.png")
	require.NotContains(t, apArticle.Content, "card.png")

	// The minted servings attach as Documents (hero + inline, in binding order);
	// the social card stays off the article object.
	require.Len(t, apArticle.Attachment, 2)
	require.Equal(t, "Document", apArticle.Attachment[0].Type)
	require.Equal(t, "https://cdn.example.test/hero.png", apArticle.Attachment[0].URL)
	require.Equal(t, "https://cdn.example.test/inline.png", apArticle.Attachment[1].URL)
	require.Equal(t, "image/jpeg", apArticle.Attachment[1].MediaType)
	require.Equal(t, 640, apArticle.Attachment[1].Width)
	require.Equal(t, 480, apArticle.Attachment[1].Height)
}

// TestStorageArticleToActivityPubWithoutMediaIsUnchanged proves the plain path
// keeps its exact behavior: no media, no composition, no attachments.
func TestStorageArticleToActivityPubWithoutMediaIsUnchanged(t *testing.T) {
	article := &storagemodels.Article{
		Object: storagemodels.Object{
			ID:      "https://example.com/articles/1",
			Type:    activitypub.ArticleType,
			Content: "# Title\n\nBody",
			Name:    "Title",
		},
		ContentFormat: "markdown",
	}
	apArticle, err := StorageArticleToActivityPub(article)
	require.NoError(t, err)
	require.Contains(t, apArticle.Content, `<h1 id="title">Title</h1>`)
	require.Len(t, apArticle.Attachment, 0)
}
