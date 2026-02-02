package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestObjectRepository_BackgroundFetchAndCache(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 3, 4, 5, 6, 0, time.UTC)

	t.Run("triggerBackgroundFetch + cached remote status present", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.triggerBackgroundFetch(ctx, "status-1"))
		require.NotNil(t, repo.getCachedRemoteStatus(ctx, "status-1"))
	})

	t.Run("cached remote status missing returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusSearchResult")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.Nil(t, repo.getCachedRemoteStatus(ctx, "status-missing"))
	})
}

func TestObjectRepository_ThreadContextFallbackPaths(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 4, 5, 6, 7, 0, time.UTC)

	t.Run("buildThreadContextFromObjects handles missing status", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		result, err := repo.buildThreadContextFromObjects(ctx, "missing-status")
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("updateThreadContext creates context when missing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadContext")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.updateThreadContext(ctx, "note-1", "increment_reply"))
	})
}

func TestObjectRepository_GetOrCreateStatusMetadata_CreatesOnNotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 5, 6, 7, 8, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	metadata, err := repo.getOrCreateStatusMetadata(ctx, "note-1")
	require.NoError(t, err)
	require.NotNil(t, metadata)
}

func TestObjectRepository_IsQuoteAllowed_CoverageMatrix(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 6, 7, 8, 9, 0, time.UTC)

	t.Run("metadata not found defaults to false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)
		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.IsQuoteAllowed(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("non-quotable metadata returns false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Run(func(args mock.Arguments) {
			meta := args.Get(0).(*models.StatusMetadata)
			meta.StatusID = "note-1"
			meta.QuoteType = "followers"
			meta.AllowQuotes = false
			meta.WithdrawnFromQuotes = false
			meta.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)
		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.IsQuoteAllowed(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("followers quote type exercises follower permission path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Run(func(args mock.Arguments) {
			meta := args.Get(0).(*models.StatusMetadata)
			meta.StatusID = "note-1"
			meta.QuoteType = "followers"
			meta.AllowQuotes = true
			meta.WithdrawnFromQuotes = false
			meta.UpdateKeys()
		}).Return(nil).Once()

		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)
		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.IsQuoteAllowed(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("mentioned quote type exercises mention permission path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Run(func(args mock.Arguments) {
			meta := args.Get(0).(*models.StatusMetadata)
			meta.StatusID = "note-1"
			meta.QuoteType = "mentioned"
			meta.AllowQuotes = true
			meta.WithdrawnFromQuotes = false
			meta.UpdateKeys()
		}).Return(nil).Once()

		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
			obj := args.Get(0).(*models.Object)
			*obj = *models.NewObject("note-1", activitypub.NoteType, "bob")
			obj.TagJSON = `[{"type":"Mention","href":"https://example.com/users/alice","name":"@alice"}]`
		}).Return(nil).Once()

		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)
		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.IsQuoteAllowed(ctx, "note-1", "https://example.com/users/alice")
		require.NoError(t, err)
		require.True(t, allowed)
	})

	t.Run("unknown quote type defaults to false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Run(func(args mock.Arguments) {
			meta := args.Get(0).(*models.StatusMetadata)
			meta.StatusID = "note-1"
			meta.QuoteType = "weird"
			meta.AllowQuotes = true
			meta.WithdrawnFromQuotes = false
			meta.UpdateKeys()
		}).Return(nil).Once()

		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)
		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.IsQuoteAllowed(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})
}

func TestObjectRepository_ExtractMentions_CoversMapAndNote(t *testing.T) {
	repo := NewObjectRepository(nil, "test-table", "example.com", zap.NewNop())

	mentions := repo.extractMentions(map[string]any{
		"tag": []any{
			map[string]any{"type": "Mention", "href": "https://example.com/users/alice"},
			map[string]any{"type": "Hashtag", "href": "https://example.com/tags/test"},
		},
	})
	require.Equal(t, []string{"https://example.com/users/alice"}, mentions)

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "note-1",
			Type: activitypub.NoteType,
		},
		Tag: []activitypub.Tag{
			{Type: TagTypeMention, Href: "https://example.com/users/bob", Name: "@bob"},
		},
	}
	require.Equal(t, []string{"https://example.com/users/bob"}, repo.extractMentions(note))
}

func TestObjectRepository_SyncThreadFromRemote_NotFoundPaths(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 7, 8, 9, 10, 0, time.UTC)

	t.Run("not found locally returns cached remote status when available", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		result, err := repo.SyncThreadFromRemote(ctx, "missing-status")
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("not found locally returns not found when cache missing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.StatusSearchResult")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		result, err := repo.SyncThreadFromRemote(ctx, "missing-status")
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestObjectRepository_MarkThreadAsSynced_CreatesWhenMissing(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.MarkThreadAsSynced(ctx, "note-1"))
}

func TestObjectRepository_GetThreadContext_FallbackBuildsFromObjects(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 9, 10, 11, 12, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadContext")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	threadContext, err := repo.GetThreadContext(ctx, "note-1")
	require.NoError(t, err)
	require.NotNil(t, threadContext)
}

func TestObjectRepository_AdditionalNotFoundAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 10, 11, 12, 13, 0, time.UTC)

	t.Run("IsInCollection not found returns false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.CollectionItem")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		inCollection, err := repo.IsInCollection(ctx, "featured", "item-1")
		require.NoError(t, err)
		require.False(t, inCollection)
	})

	t.Run("CountCollectionItems error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Count").Return(int64(0), dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.CountCollectionItems(ctx, "featured")
		require.Error(t, err)
	})

	t.Run("IsQuoted not found returns false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.QuoteRelationship")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		quoted, err := repo.IsQuoted(ctx, "alice", "note-1")
		require.NoError(t, err)
		require.False(t, quoted)
	})

	t.Run("WithdrawQuote not found returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.QuoteRelationship")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.WithdrawQuote(ctx, "missing-quote-note"))
	})

	t.Run("GetQuoteType not found returns disabled", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		quoteType, err := repo.GetQuoteType(ctx, "note-1")
		require.NoError(t, err)
		require.Equal(t, "disabled", quoteType)
	})

	t.Run("IsWithdrawnFromQuotes not found returns false", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		withdrawn, err := repo.IsWithdrawnFromQuotes(ctx, "note-1")
		require.NoError(t, err)
		require.False(t, withdrawn)
	})

	t.Run("CreateTombstone create error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		err := repo.CreateTombstone(ctx, &models.Tombstone{
			ID:         "note-1",
			FormerType: activitypub.NoteType,
			DeletedBy:  "alice",
		})
		require.Error(t, err)
	})

	t.Run("GetTombstone not found returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetTombstone(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("IsTombstoned returns error when GetTombstone errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Tombstone")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		ok, err := repo.IsTombstoned(ctx, "note-1")
		require.Error(t, err)
		require.False(t, ok)
	})

	t.Run("DeleteObject delete error returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.Error(t, repo.DeleteObject(ctx, "note-1"))
	})

	t.Run("GetObject not found returns error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetObject(ctx, "note-missing")
		require.Error(t, err)
	})

	t.Run("UpdateObject not found creates object", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		notePublished := baseTime.Add(-2 * time.Hour)
		noteUpdated := baseTime

		err := repo.UpdateObject(ctx, activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "note-1",
				Type:      activitypub.NoteType,
				Published: &notePublished,
				Updated:   &noteUpdated,
			},
			Content:      "hello",
			AttributedTo: "alice",
		})
		require.NoError(t, err)
	})
}

func TestObjectRepository_GetThreadContext_AncestorsDescendantsErrorFallback(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 11, 12, 13, 14, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.ThreadContext")).Return(dynamormErrors.ErrInvalidModel).Twice()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	threadContext, err := repo.GetThreadContext(ctx, "note-1")
	require.NoError(t, err)
	require.NotNil(t, threadContext)
	require.Empty(t, threadContext.Ancestors)
	require.Empty(t, threadContext.Descendants)
}

func TestObjectRepository_IncrementReplyCount_EdgeBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 12, 13, 14, 15, 0, time.UTC)

	t.Run("object not found returns nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.IncrementReplyCount(ctx, "note-1"))
	})

	t.Run("metadata created and thread context update error is ignored", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadContext")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.NoError(t, repo.IncrementReplyCount(ctx, "note-1"))
	})
}

func TestObjectRepository_WithdawStatusFromQuotes_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 13, 14, 15, 16, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.QuoteRelationship")).Return(dynamormErrors.ErrInvalidModel).Once()
	mockQuery.On("Create").Return(dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.WithdrawStatusFromQuotes(ctx, "note-1"))
}

func TestObjectRepository_UpdateObjectWithHistory_NotFoundPropagatesError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 14, 15, 16, 17, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	notePublished := baseTime.Add(-2 * time.Hour)
	noteUpdated := baseTime
	require.NoError(t, repo.UpdateObjectWithHistory(ctx, activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "note-1",
			Type:      activitypub.NoteType,
			Published: &notePublished,
			Updated:   &noteUpdated,
		},
		Content:      "hello",
		AttributedTo: "alice",
	}, "alice"))
}

func TestObjectRepository_TombstoneObject_UsesMapBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 15, 16, 17, 18, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
		obj := args.Get(0).(*models.Object)
		*obj = *models.NewObject("note-1", "Article", "alice")
	}).Return(nil).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.TombstoneObject(ctx, "note-1", "alice"))
}

func TestObjectRepository_CountQuotes_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 16, 17, 18, 19, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Count").Return(int64(0), dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	_, err := repo.CountQuotes(ctx, "note-1")
	require.Error(t, err)
}

func TestObjectRepository_GetUserStatusCount_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 17, 18, 19, 20, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Count").Return(int64(0), dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	_, err := repo.GetUserStatusCount(ctx, "alice")
	require.Error(t, err)
}

func TestObjectRepository_ReplaceObjectWithTombstone_WarnsOnDeleteError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 18, 19, 20, 21, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Delete").Return(dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.ReplaceObjectWithTombstone(ctx, "note-1", activitypub.NoteType, "alice"))
}

func TestObjectRepository_ErrorPathCoveragePush(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 19, 20, 21, 22, 0, time.UTC)

	t.Run("checkFollowerPermission handles GetObject error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.checkFollowerPermission(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("checkFollowerPermission handles missing author", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
			obj := args.Get(0).(*models.Object)
			*obj = *models.NewObject("note-1", activitypub.NoteType, "")
			obj.AttributedTo = ""
		}).Return(nil).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.checkFollowerPermission(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("checkFollowerPermission handles IsFollowing error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Follow")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.checkFollowerPermission(ctx, "note-1", "alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("checkMentionPermission handles GetObject error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.checkMentionPermission(ctx, "note-1", "https://example.com/users/alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("checkMentionPermission denies when mention missing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
			obj := args.Get(0).(*models.Object)
			*obj = *models.NewObject("note-1", activitypub.NoteType, "bob")
			obj.TagJSON = `[{"type":"Mention","href":"https://example.com/users/other"}]`
		}).Return(nil).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		allowed, err := repo.checkMentionPermission(ctx, "note-1", "https://example.com/users/alice")
		require.NoError(t, err)
		require.False(t, allowed)
	})

	t.Run("GetMissingReplies handles thread sync not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		results, err := repo.GetMissingReplies(ctx, "note-1")
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("GetMissingReplies handles empty missing replies list", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Run(func(args mock.Arguments) {
			sync := args.Get(0).(*models.ThreadSync)
			*sync = *models.NewThreadSync("note-1")
			sync.MissingReplies = []string{}
		}).Return(nil).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		results, err := repo.GetMissingReplies(ctx, "note-1")
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("GetMissingReplies returns error when thread sync lookup errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetMissingReplies(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("CleanupExpiredTombstones returns error when scan errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.CleanupExpiredTombstones(ctx, 2)
		require.Error(t, err)
	})

	t.Run("CleanupExpiredTombstones continues when delete fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		cleaned, err := repo.CleanupExpiredTombstones(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, 1, cleaned)
	})

	t.Run("getOrCreateStatusMetadata returns error when create fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.getOrCreateStatusMetadata(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("GetObjectHistory returns error when update history query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.UpdateHistory")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetObjectHistory(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("CountObjectReplies returns error when query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.Object")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.CountObjectReplies(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("SyncThreadFromRemote returns error on unexpected local query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.Object")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.SyncThreadFromRemote(ctx, "note-1")
		require.Error(t, err)
	})
}

func TestObjectRepository_FinalCoverageNudges(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 20, 21, 22, 23, 0, time.UTC)

	t.Run("IsInCollection returns error on query failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.CollectionItem")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.IsInCollection(ctx, "featured", "item-1")
		require.Error(t, err)
	})

	t.Run("IsQuoted returns error on query failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.QuoteRelationship")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.IsQuoted(ctx, "alice", "note-1")
		require.Error(t, err)
	})

	t.Run("WithdrawQuote returns error on update failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Update", mock.Anything).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		require.Error(t, repo.WithdrawQuote(ctx, "quote-note-1"))
	})
}

type testInvalidJSONMarshaler struct{}

func (testInvalidJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte("{not valid json"), nil
}

func TestObjectRepository_storeEditHistoryForUpdate_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	repo := NewObjectRepository(nil, "test-table", "example.com", logger)

	t.Run("marshal failure returns error", func(t *testing.T) {
		err := repo.storeEditHistoryForUpdate(ctx, "note-1", make(chan int), "alice")
		require.Error(t, err)
	})

	t.Run("unmarshal failure returns error", func(t *testing.T) {
		err := repo.storeEditHistoryForUpdate(ctx, "note-1", testInvalidJSONMarshaler{}, "alice")
		require.Error(t, err)
	})
}

func TestObjectRepository_CountReplies_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 21, 22, 23, 24, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Count").Return(int64(0), dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	_, err := repo.CountReplies(ctx, "note-1")
	require.Error(t, err)
}

func TestObjectRepository_MarkThreadAsSynced_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 22, 23, 24, 25, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.ThreadSync")).Return(dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.Error(t, repo.MarkThreadAsSynced(ctx, "note-1"))
}

func TestObjectRepository_UpdateObjectWithHistory_IgnoresHistoryFailure(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 23, 10, 11, 12, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Create").Return(dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	notePublished := baseTime.Add(-1 * time.Hour)
	noteUpdated := baseTime

	require.NoError(t, repo.UpdateObjectWithHistory(ctx, activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "note-1",
			Type:      activitypub.NoteType,
			Published: &notePublished,
			Updated:   &noteUpdated,
		},
		Content:      "hello",
		AttributedTo: "alice",
	}, "alice"))
}

func TestObjectRepository_TombstoneObject_ReturnsErrorWhenDeleteFails(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 24, 11, 12, 13, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Delete").Return(dynamormErrors.ErrInvalidModel).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.Error(t, repo.TombstoneObject(ctx, "note-1", "alice"))
}

func TestObjectRepository_MoreQueryErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 25, 12, 13, 14, 0, time.UTC)

	t.Run("GetReplies returns error when query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.Object")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, _, err := repo.GetReplies(ctx, "note-1", 10, "")
		require.Error(t, err)
	})

	t.Run("GetThreadContext returns error when lookup fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.ThreadContext")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetThreadContext(ctx, "note-1")
		require.Error(t, err)
	})

	t.Run("GetQuotesOfStatus returns error when query fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.QuoteRelationship")).Return(dynamormErrors.ErrInvalidModel).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		_, err := repo.GetQuotesOfStatus(ctx, "note-1", 10)
		require.Error(t, err)
	})
}
