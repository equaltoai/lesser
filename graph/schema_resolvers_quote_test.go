package graph

import (
	"context"
	"errors"
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

func withActorLoaderStub(t *testing.T, fn func(context.Context, string) (*activitypub.Actor, error)) {
	t.Helper()
	original := actorLoaderFunc
	actorLoaderFunc = fn
	t.Cleanup(func() {
		actorLoaderFunc = original
	})
}

func TestResolveQuoteMetadata_TargetLoaded(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	target := &models.Status{
		StatusID:       "target-123",
		AuthorID:       "https://example.com/users/actual-author",
		AuthorUsername: "actual-author",
		QuoteCount:     7,
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
	require.Equal(t, target.AuthorUsername, quoteCtx.OriginalAuthorUsername)
	require.True(t, quoteCtx.QuoteAllowed)
	require.False(t, quoteCtx.Withdrawn)
	stored, ok := quoteCtx.OriginalStatus.(*models.Status)
	require.True(t, ok)
	require.Equal(t, target, stored)
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
	require.Equal(t, status.QuoteTargetAuthorID, quoteCtx.OriginalAuthorUsername)
	require.True(t, quoteCtx.QuoteAllowed)
	require.False(t, quoteCtx.Withdrawn)
}

func TestResolveQuoteMetadata_TargetDeleted(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	target := &models.Status{
		StatusID:       "deleted-target",
		AuthorID:       "author-id",
		AuthorUsername: "author-name",
		QuoteCount:     4,
		Deleted:        true,
	}

	withQuoteLoaderStub(t, func(ctx context.Context, statusID string) (*models.Status, error) {
		require.Equal(t, "deleted-target", statusID)
		return target, nil
	})

	status := &models.Status{
		StatusID:            "quote-del",
		QuoteTargetStatusID: "deleted-target",
	}

	_, quoteCtx := resolver.resolveQuoteMetadata(context.Background(), status)
	require.NotNil(t, quoteCtx)
	require.True(t, quoteCtx.Withdrawn)
	require.False(t, quoteCtx.QuoteAllowed)
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

func TestQuoteContextResolverOriginalAuthorUsesLoader(t *testing.T) {
	r := &quoteContextResolver{Resolver: &Resolver{Logger: zap.NewNop()}}
	ctx := context.Background()
	expected := &activitypub.Actor{
		PreferredUsername: "alice",
		Name:              "Alice",
	}

	withActorLoaderStub(t, func(ctx context.Context, username string) (*activitypub.Actor, error) {
		require.Equal(t, "alice", username)
		return expected, nil
	})

	actor, err := r.OriginalAuthor(ctx, &activitypub.QuoteContext{
		OriginalAuthor:         "https://example.com/users/alice",
		OriginalAuthorUsername: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, expected, actor)
}

func TestQuoteContextResolverOriginalAuthorFallback(t *testing.T) {
	r := &quoteContextResolver{Resolver: &Resolver{Logger: zap.NewNop()}}
	ctx := context.Background()

	withActorLoaderStub(t, func(ctx context.Context, username string) (*activitypub.Actor, error) {
		return nil, errors.New("boom")
	})

	actor, err := r.OriginalAuthor(ctx, &activitypub.QuoteContext{
		OriginalAuthor: "https://remote.example/@bob",
	})
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, "bob", actor.PreferredUsername)
	require.Equal(t, "https://remote.example/@bob", actor.ID)
}

func TestQuoteContextResolverOriginalNoteUsesCachedStatus(t *testing.T) {
	r := &quoteContextResolver{Resolver: &Resolver{Logger: zap.NewNop()}}
	ctx := context.Background()
	now := time.Now()
	status := &models.Status{
		StatusID:  "target-cached",
		CreatedAt: now,
		UpdatedAt: now,
		Content:   "cached",
	}

	note, err := r.OriginalNote(ctx, &activitypub.QuoteContext{
		OriginalNoteID: "target-cached",
		OriginalStatus: status,
	})
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Equal(t, "target-cached", note.ID)
}

func TestQuoteContextResolverOriginalNoteLoadsStatus(t *testing.T) {
	r := &quoteContextResolver{Resolver: &Resolver{Logger: zap.NewNop()}}
	ctx := context.Background()
	now := time.Now()
	target := &models.Status{
		StatusID:  "target-load",
		CreatedAt: now,
		UpdatedAt: now,
		Content:   "loaded",
	}

	withQuoteLoaderStub(t, func(ctx context.Context, statusID string) (*models.Status, error) {
		require.Equal(t, "target-load", statusID)
		return target, nil
	})

	note, err := r.OriginalNote(ctx, &activitypub.QuoteContext{
		OriginalNoteID: "target-load",
	})
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Equal(t, target.StatusID, note.ID)
}

func TestQuoteContextResolverOriginalNoteWithdrawn(t *testing.T) {
	r := &quoteContextResolver{Resolver: &Resolver{Logger: zap.NewNop()}}
	ctx := context.Background()

	note, err := r.OriginalNote(ctx, &activitypub.QuoteContext{
		OriginalNoteID: "target-withdrawn",
		Withdrawn:      true,
	})
	require.NoError(t, err)
	require.Nil(t, note)
}
