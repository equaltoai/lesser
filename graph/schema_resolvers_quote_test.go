package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func withQuoteLoaderStub(t *testing.T, fn func(context.Context, string) (*models.Status, error)) {
	t.Helper()
	original := quoteTargetStatusLoaderFunc
	quoteTargetStatusLoaderFunc = fn
	t.Cleanup(func() {
		quoteTargetStatusLoaderFunc = original
	})
}

func TestResolveQuoteMetadata_TargetLoaded(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	target := &models.Status{
		StatusID:   "target-123",
		AuthorID:   "actual-author",
		QuoteCount: 7,
		Note: &models.NoteField{
			Note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID: "https://example.com/users/alice/statuses/target-123",
				},
			},
		},
	}

	status := &models.Status{
		StatusID:            "quote-1",
		QuoteTargetStatusID: "target-123",
		QuoteTargetAuthorID: "stored-author",
	}

	withQuoteLoaderStub(t, func(ctx context.Context, statusID string) (*models.Status, error) {
		require.Equal(t, "target-123", statusID)
		return target, nil
	})

	quoteURL, quoteCtx := resolver.resolveQuoteMetadata(context.Background(), status)
	require.NotNil(t, quoteURL)
	require.Equal(t, target.Note.Get().ID, *quoteURL)
	require.NotNil(t, quoteCtx)
	require.Equal(t, status.QuoteTargetStatusID, quoteCtx.OriginalNoteID)
	require.Equal(t, target.AuthorID, quoteCtx.OriginalAuthor)
	require.Equal(t, target.QuoteCount, quoteCtx.QuoteCount)
}

func TestResolveQuoteMetadata_TargetMissing(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	status := &models.Status{
		StatusID:            "quote-2",
		QuoteTargetStatusID: "missing-target",
		QuoteTargetAuthorID: "author-from-reference",
	}

	withQuoteLoaderStub(t, func(ctx context.Context, statusID string) (*models.Status, error) {
		require.Equal(t, "missing-target", statusID)
		return nil, nil
	})

	quoteURL, quoteCtx := resolver.resolveQuoteMetadata(context.Background(), status)
	require.NotNil(t, quoteURL)
	require.Equal(t, status.QuoteTargetStatusID, *quoteURL)
	require.NotNil(t, quoteCtx)
	require.Equal(t, status.QuoteTargetAuthorID, quoteCtx.OriginalAuthor)
	require.Equal(t, 0, quoteCtx.QuoteCount)
}

func TestResolveQuoteMetadata_NoReference(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	status := &models.Status{StatusID: "regular"}

	url, ctxData := resolver.resolveQuoteMetadata(context.Background(), status)
	require.Nil(t, url)
	require.Nil(t, ctxData)
}

func TestConvertStatusToObjectIncludesQuoteMetadata(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	target := &models.Status{
		StatusID:   "target-obj",
		AuthorID:   "target-author",
		QuoteCount: 2,
		Note: &models.NoteField{Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/bob/statuses/target-obj",
			},
		}},
	}

	withQuoteLoaderStub(t, func(ctx context.Context, statusID string) (*models.Status, error) {
		if statusID == "target-obj" {
			return target, nil
		}
		return nil, nil
	})

	now := time.Now()
	status := &models.Status{
		StatusID:            "quote-obj",
		Content:             "quoting status",
		CreatedAt:           now,
		UpdatedAt:           now,
		Note:                &models.NoteField{Note: &activitypub.Note{Quoteable: false}},
		QuoteTargetStatusID: "target-obj",
	}

	obj := resolver.convertStatusToObject(context.Background(), status)
	require.NotNil(t, obj)
	require.True(t, obj.Quoteable)
	require.NotNil(t, obj.QuoteURL)
	require.Equal(t, target.Note.Get().ID, *obj.QuoteURL)
	require.NotNil(t, obj.QuoteContext)
	require.Equal(t, status.QuoteTargetStatusID, obj.QuoteContext.OriginalNoteID)
	require.Equal(t, target.AuthorID, obj.QuoteContext.OriginalAuthor)
}
