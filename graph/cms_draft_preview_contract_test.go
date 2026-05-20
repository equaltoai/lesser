package graph

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestM45DraftPreviewReturnsCanonicalRenderedHTML(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	ctx := round12AuthContext("alice")
	mut := resolver.Mutation()
	qry := resolver.Query()

	title := "Preview Me"
	content := "# Preview Me\n\n<script>alert('x')</script>\n\n[ok](https://example.com) [bad](javascript:alert(1))"
	draft, err := mut.CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       content,
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	require.NotNil(t, draft)

	preview, err := qry.DraftPreview(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.True(t, preview.Success)
	require.Empty(t, preview.Errors)
	require.Equal(t, draft.ID, preview.DraftID)
	require.Equal(t, cmsrender.FormatMarkdown, preview.SourceFormat)
	require.Equal(t, len(content), preview.SourceBytes)
	require.NotNil(t, preview.RenderedHTML)
	require.Equal(t, len(*preview.RenderedHTML), preview.RenderedBytes)
	require.Contains(t, *preview.RenderedHTML, `<h1 id="preview-me">Preview Me</h1>`)
	require.Contains(t, *preview.RenderedHTML, `href="https://example.com"`)
	require.NotContains(t, *preview.RenderedHTML, "<script")
	require.NotContains(t, *preview.RenderedHTML, "javascript:")

	unauthorized, err := qry.DraftPreview(round12AuthContext("bob"), draft.ID)
	require.Error(t, err)
	require.Nil(t, unauthorized)
}

func TestM45DraftPreviewReturnsDeterministicRenderErrors(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	qry := resolver.Query()
	ctx := context.Background()

	cases := []struct {
		name      string
		format    string
		content   string
		wantError string
	}{
		{
			name:      "unsupported format",
			format:    "plaintext",
			content:   "hello",
			wantError: "unsupported article content format",
		},
		{
			name:      "invalid utf8",
			format:    cmsrender.FormatMarkdown,
			content:   string([]byte{0xff, 0xfe, 0xfd}),
			wantError: "valid UTF-8",
		},
		{
			name:      "source size",
			format:    cmsrender.FormatMarkdown,
			content:   strings.Repeat("a", cmsrender.MaxArticleSourceBytes+1),
			wantError: "maximum source size",
		},
		{
			name:      "rendered size",
			format:    cmsrender.FormatMarkdown,
			content:   strings.Repeat("<", cmsrender.MaxArticleSourceBytes),
			wantError: "maximum output size",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			draftID := "preview-" + strings.ReplaceAll(tc.name, " ", "-")
			draft := &models.Draft{
				ID:            draftID,
				AuthorID:      "alice",
				ContentType:   activitypub.ArticleType,
				Content:       tc.content,
				ContentFormat: tc.format,
				Status:        "draft",
				LastSavedAt:   time.Now(),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			require.NoError(t, storage.Draft().CreateDraft(ctx, draft))

			preview, err := qry.DraftPreview(round12AuthContext("alice"), draftID)
			require.NoError(t, err)
			require.NotNil(t, preview)
			require.False(t, preview.Success)
			require.Nil(t, preview.RenderedHTML)
			require.Equal(t, draftID, preview.DraftID)
			require.Equal(t, tc.format, preview.SourceFormat)
			require.Equal(t, len(tc.content), preview.SourceBytes)
			require.Zero(t, preview.RenderedBytes)
			require.Len(t, preview.Errors, 1)
			require.Contains(t, preview.Errors[0], tc.wantError)
		})
	}
}
