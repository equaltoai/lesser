package remotenotes

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplyParentSigningActor(t *testing.T) {
	t.Run("requires author identity", func(t *testing.T) {
		actor, err := replyParentSigningActor(nil, "example.com")
		require.Error(t, err)
		assert.Nil(t, actor)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
	})

	t.Run("fills missing public key from actor id", func(t *testing.T) {
		actor, err := replyParentSigningActor(&storage.Account{
			User: &storage.User{Username: "alice"},
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
				PreferredUsername: "alice",
			},
		}, "example.com")
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.NotNil(t, actor.PublicKey)
		assert.Equal(t, "https://example.com/users/alice#main-key", actor.PublicKey.ID)
		assert.Equal(t, "https://example.com/users/alice", actor.PublicKey.Owner)
	})

	t.Run("synthesizes actor from local domain when actor is absent", func(t *testing.T) {
		actor, err := replyParentSigningActor(&storage.Account{
			User: &storage.User{Username: "alice"},
		}, "example.com")
		require.NoError(t, err)
		require.NotNil(t, actor)
		assert.Equal(t, "alice", actor.PreferredUsername)
		assert.Equal(t, "https://example.com/users/alice", actor.ID)
	})

	t.Run("rejects missing local domain when actor is absent", func(t *testing.T) {
		actor, err := replyParentSigningActor(&storage.Account{
			User: &storage.User{Username: "alice"},
		}, "")
		require.Error(t, err)
		assert.Nil(t, actor)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
	})
}

func TestNormalizeReplyParentURLAndFetchedReplyParentNote(t *testing.T) {
	t.Run("normalizes canonical remote url and strips fragment", func(t *testing.T) {
		normalized, err := normalizeReplyParentURL("https://remote.example/users/steward/statuses/seed-1#context")
		require.NoError(t, err)
		assert.Equal(t, "https://remote.example/users/steward/statuses/seed-1", normalized)
	})

	t.Run("rejects empty or non-url references", func(t *testing.T) {
		_, err := normalizeReplyParentURL("   ")
		require.Error(t, err)
		assert.ErrorContains(t, err, "empty")

		_, err = normalizeReplyParentURL("seed-1")
		require.Error(t, err)
		assert.ErrorContains(t, err, "canonical remote status URL")
	})

	t.Run("rejects invalid fetched note shapes", func(t *testing.T) {
		_, err := fetchedReplyParentNote("not-a-map")
		require.Error(t, err)
		assert.ErrorContains(t, err, "valid ActivityPub object")

		_, err = fetchedReplyParentNote(map[string]any{
			"@context":     []any{"https://www.w3.org/ns/activitystreams"},
			"id":           "https://remote.example/objects/seed",
			"type":         "Article",
			"content":      "seed",
			"attributedTo": "https://remote.example/users/steward",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not a Note")
	})

	t.Run("parses a valid fetched note", func(t *testing.T) {
		note, err := fetchedReplyParentNote(map[string]any{
			"@context":     []any{"https://www.w3.org/ns/activitystreams"},
			"id":           "https://remote.example/users/steward/statuses/seed-1",
			"type":         activitypub.NoteType,
			"content":      "seed",
			"attributedTo": "https://remote.example/users/steward",
			"to":           []any{activitypub.PublicAddress},
			"cc":           []any{"https://remote.example/users/steward/followers"},
		})
		require.NoError(t, err)
		require.NotNil(t, note)
		assert.Equal(t, "https://remote.example/users/steward/statuses/seed-1", note.ID)
		assert.Equal(t, "https://remote.example/users/steward", note.AttributedTo)
	})

	t.Run("rejects fetched notes without an id", func(t *testing.T) {
		_, err := fetchedReplyParentNote(map[string]any{
			"@context":     []any{"https://www.w3.org/ns/activitystreams"},
			"type":         activitypub.NoteType,
			"content":      "seed",
			"attributedTo": "https://remote.example/users/steward",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "empty")
	})
}

func TestReplyParentErrorMappingAndHelpers(t *testing.T) {
	t.Run("maps deadline to timeout", func(t *testing.T) {
		appErr, ok := commonerrors.AsAppError(mapReplyParentFetchError("https://remote.example/statuses/seed-1", context.DeadlineExceeded))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeTimeout, appErr.Code)
	})

	t.Run("maps known app errors to service unavailable or unprocessable", func(t *testing.T) {
		serviceUnavailable, ok := commonerrors.AsAppError(mapReplyParentFetchError(
			"https://remote.example/statuses/seed-1",
			commonerrors.NewAppError(commonerrors.CodeExternalServiceUnavailable, commonerrors.CategoryExternal, "down"),
		))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, serviceUnavailable.Code)

		unusable, ok := commonerrors.AsAppError(mapReplyParentFetchError(
			"https://remote.example/statuses/seed-1",
			commonerrors.RemoteFetchNotFound("https://remote.example/statuses/seed-1"),
		))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, unusable.Code)
	})

	t.Run("falls back to service unavailable for unknown fetch failures", func(t *testing.T) {
		appErr, ok := commonerrors.AsAppError(mapReplyParentFetchError("https://remote.example/statuses/seed-1", stdErrors.New("dial tcp failure")))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
	})

	t.Run("builds stable app errors for helper constructors", func(t *testing.T) {
		badRequest, ok := commonerrors.AsAppError(invalidReplyParentReference("bad", stdErrors.New("boom")))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeBadRequest, badRequest.Code)

		timeout, ok := commonerrors.AsAppError(timeoutReplyParent("https://remote.example/statuses/seed-1", context.DeadlineExceeded))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeTimeout, timeout.Code)

		unavailable, ok := commonerrors.AsAppError(serviceUnavailableReplyParent("https://remote.example/statuses/seed-1", stdErrors.New("down")))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, unavailable.Code)

		unusable, ok := commonerrors.AsAppError(unusableReplyParent("https://remote.example/statuses/seed-1", "not a note"))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, unusable.Code)
	})

	t.Run("tracks not-found classification and parent result projection", func(t *testing.T) {
		assert.True(t, statusLookupNotFound(storage.ErrNotFound))
		assert.True(t, statusLookupNotFound(commonerrors.NotFound("status")))
		assert.False(t, statusLookupNotFound(stdErrors.New("boom")))

		parent := remoteStatus("https://remote.example/users/steward/statuses/seed-1", models.VisibilityPublic)
		result := replyParentResultFromStatus(parent, true, "example.com")
		require.NotNil(t, result)
		assert.True(t, result.Remote)
		assert.True(t, result.Fetched)
		assert.Equal(t, parent.Note.ID, result.CanonicalObjectURL)
		assert.Equal(t, parent.StatusID, result.CanonicalStatusID)
	})

	t.Run("builds local fallback object urls and local parent classification", func(t *testing.T) {
		parent := &models.Status{
			StatusID:       "status-1",
			AuthorID:       "https://example.com/users/alice",
			AuthorUsername: "alice",
			Visibility:     models.VisibilityPublic,
		}

		result := replyParentResultFromStatus(parent, false, "example.com")
		require.NotNil(t, result)
		assert.False(t, result.Remote)
		assert.Equal(t, "https://example.com/users/alice/statuses/status-1", result.CanonicalObjectURL)
	})

	t.Run("covers object url and remote detection edge cases", func(t *testing.T) {
		assert.Empty(t, replyParentObjectURL(nil, "example.com"))
		assert.False(t, replyParentLooksRemote(nil, "", "example.com"))

		urlBacked := &models.Status{
			StatusID:   "status-1",
			AuthorID:   "",
			Visibility: models.VisibilityPublic,
			URLs: []string{
				"mailto:not-used@example.com",
				"https://remote.example/users/steward/statuses/seed-2",
			},
		}
		objectURL := replyParentObjectURL(urlBacked, "example.com")
		assert.Equal(t, "https://remote.example/users/steward/statuses/seed-2", objectURL)
		assert.True(t, replyParentLooksRemote(urlBacked, objectURL, "example.com"))

		statusIDBacked := &models.Status{
			StatusID:   "https://remote.example/users/steward/statuses/seed-3",
			AuthorID:   "",
			Visibility: models.VisibilityPublic,
		}
		assert.Equal(t, statusIDBacked.StatusID, replyParentObjectURL(statusIDBacked, "example.com"))
		assert.True(t, replyParentLooksRemote(statusIDBacked, statusIDBacked.StatusID, "example.com"))

		incompleteLocal := &models.Status{
			StatusID:   "status-4",
			AuthorID:   "https://example.com/users/alice",
			Visibility: models.VisibilityPublic,
		}
		assert.Empty(t, replyParentObjectURL(incompleteLocal, "example.com"))
		assert.False(t, replyParentLooksRemote(incompleteLocal, "not-a-url", "example.com"))
	})
}

func TestResolveStoredParentAndFetchGuards(t *testing.T) {
	t.Run("resolves stored parent from candidate ids", func(t *testing.T) {
		parentURL := "https://remote.example/users/steward/statuses/seed-1"
		stored := remoteStatus(parentURL, models.VisibilityPublic)
		resolver := &Resolver{
			statusRepo:  &stubStatusRepo{byID: map[string]*models.Status{stored.StatusID: stored}},
			localDomain: "example.com",
		}

		parent, err := resolver.resolveStoredParent(context.Background(), parentURL)
		require.NoError(t, err)
		assert.Same(t, stored, parent)
	})

	t.Run("guards fetch availability and domain block repository errors", func(t *testing.T) {
		resolver := &Resolver{}
		appErr, ok := commonerrors.AsAppError(resolver.ensureFetchAllowed(context.Background(), "https://remote.example/statuses/seed-1"))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)

		resolver = &Resolver{
			fetcher:         &stubFetcher{},
			domainBlockRepo: &stubDomainBlockRepo{err: stdErrors.New("lookup failed")},
		}
		appErr, ok = commonerrors.AsAppError(resolver.ensureFetchAllowed(context.Background(), "https://remote.example/statuses/seed-1"))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
	})

	t.Run("returns not found when no stored parent candidates resolve", func(t *testing.T) {
		resolver := &Resolver{
			statusRepo:  &stubStatusRepo{},
			localDomain: "example.com",
		}

		parent, err := resolver.resolveStoredParent(context.Background(), "status-1")
		require.Error(t, err)
		assert.Nil(t, parent)
		assert.True(t, statusLookupNotFound(err))
	})

	t.Run("returns nil for empty reply parent and rejects unresolved local-only ids", func(t *testing.T) {
		resolver := NewReplyParentResolver(
			&stubStatusRepo{},
			&stubObjectRepo{},
			&stubDomainBlockRepo{},
			&stubFetcher{},
			"example.com",
			nil,
		)

		parent, err := resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), "   ", models.VisibilityPublic)
		require.NoError(t, err)
		assert.Nil(t, parent)

		parent, err = resolver.ResolveReplyParent(context.Background(), localAuthorAccount("alice"), "status-1", models.VisibilityPublic)
		require.Error(t, err)
		assert.Nil(t, parent)
		appErr, ok := commonerrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeUnprocessableEntity, appErr.Code)
		assert.Equal(t, 422, appErr.HTTPStatusCode)
	})
}

func TestReplyParentVisibilityAndFailureHelpers(t *testing.T) {
	t.Run("maps supported visibility levels and rejects unknown values", func(t *testing.T) {
		rank, ok := replyVisibilityRank(models.VisibilityPublic)
		require.True(t, ok)
		assert.Equal(t, 0, rank)

		rank, ok = replyVisibilityRank(models.VisibilityUnlisted)
		require.True(t, ok)
		assert.Equal(t, 1, rank)

		rank, ok = replyVisibilityRank(models.VisibilityPrivate)
		require.True(t, ok)
		assert.Equal(t, 2, rank)

		rank, ok = replyVisibilityRank(models.VisibilityDirect)
		require.True(t, ok)
		assert.Equal(t, 3, rank)

		_, ok = replyVisibilityRank("followers")
		assert.False(t, ok)
	})

	t.Run("normalizes parent domains and materialize failure mapping", func(t *testing.T) {
		assert.Equal(t, "remote.example", replyParentDomain("https://remote.example/users/steward/statuses/seed-1"))
		assert.Empty(t, replyParentDomain("not-a-url"))
		assert.Nil(t, materializeReplyParentFailure("https://remote.example/statuses/seed-1", nil))

		appErr, ok := commonerrors.AsAppError(materializeReplyParentFailure("https://remote.example/statuses/seed-1", stdErrors.New("write failed")))
		require.True(t, ok)
		assert.Equal(t, commonerrors.CodeExternalServiceUnavailable, appErr.Code)
	})
}
