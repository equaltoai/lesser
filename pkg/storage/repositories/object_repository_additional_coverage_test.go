package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
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

func TestObjectRepository_ModelToActivityPubObjectPreservesVisibility(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	note := objectModelToActivityPubNote(&models.Object{
		ID:           "note-visibility",
		Type:         activitypub.NoteType,
		Content:      "private note",
		AttributedTo: "https://example.com/users/bob",
		Published:    now,
		Updated:      now,
		Visibility:   models.VisibilityPrivate,
	})
	require.Equal(t, models.VisibilityPrivate, note.Visibility)

	repo := &ObjectRepository{}
	generic, err := repo.modelToActivityPubObject(context.Background(), &models.Object{
		ID:           "question-visibility",
		Type:         "Question",
		Content:      "private question",
		AttributedTo: "https://example.com/users/bob",
		Published:    now,
		Updated:      now,
		Visibility:   models.VisibilityDirect,
	})
	require.NoError(t, err)
	genericMap, ok := generic.(map[string]any)
	require.True(t, ok)
	require.Equal(t, models.VisibilityDirect, genericMap["visibility"])
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

func TestObjectRepositoryGetQuoteTypesUsesOneBatchRead(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.StatusMetadata")).Return(mockQuery).Once()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		keys := args.Get(0).([]any)
		require.Len(t, keys, 3)
		rows := args.Get(1).(*[]*models.StatusMetadata)
		*rows = []*models.StatusMetadata{
			{StatusID: "one", QuoteType: "followers"},
			{StatusID: "two", QuoteType: "disabled"},
			nil,
			{},
		}
	}).Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())
	quoteTypes, err := repo.GetQuoteTypes(context.Background(), []string{"one", "two", "missing", "one"})
	require.NoError(t, err)
	require.Equal(t, "followers", quoteTypes["one"])
	require.Equal(t, "disabled", quoteTypes["two"])
	require.Equal(t, models.VisibilityPublic, quoteTypes["missing"])
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)

	empty, err := repo.GetQuoteTypes(context.Background(), []string{"", " "})
	require.NoError(t, err)
	require.Empty(t, empty)

	errorDB := new(mocks.MockDB)
	errorQuery := new(mocks.MockQuery)
	errorDB.On("WithContext", mock.Anything).Return(errorDB).Once()
	errorDB.On("Model", mock.AnythingOfType("*models.StatusMetadata")).Return(errorQuery).Once()
	errorQuery.On("BatchGet", mock.Anything, mock.Anything).Return(dynamormErrors.ErrInvalidModel).Once()
	errorRepo := NewObjectRepository(errorDB, "test-table", "example.com", zap.NewNop())
	_, err = errorRepo.GetQuoteTypes(context.Background(), []string{"one"})
	require.Error(t, err)
}

func TestObjectRepository_CreateObject_PreservesRemoteNoteFields(t *testing.T) {
	ctx := context.Background()
	publishedAt := time.Date(2025, 1, 20, 10, 11, 12, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var captured *models.Object

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
		captured = args.Get(0).(*models.Object)
	}).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "dev.simulacrum.greater.website", zap.NewNop())
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/1",
			Type:      activitypub.NoteType,
			Published: &publishedAt,
			To:        []string{activitypub.PublicAddress},
			BTo:       []string{"https://remote.example/users/bob/bto"},
			BCC:       []string{"https://remote.example/users/bob/bcc"},
			Summary:   "spoiler",
		},
		Content:        "hello from remote",
		AttributedTo:   "https://remote.example/users/bob",
		ConversationID: "conv-1",
		Visibility:     models.VisibilityPrivate,
	}

	require.NoError(t, repo.CreateObject(ctx, note))
	require.NotNil(t, captured)
	require.True(t, captured.IsRemote)
	require.Equal(t, "conv-1", captured.ConversationID)
	require.Equal(t, models.VisibilityPrivate, captured.Visibility)
	require.Equal(t, []string{"https://remote.example/users/bob/bto"}, captured.BTo)
	require.Equal(t, []string{"https://remote.example/users/bob/bcc"}, captured.BCC)
	require.Equal(t, "spoiler", captured.Summary)
	require.Equal(t, publishedAt, captured.Published)
}

func TestObjectRepository_CreateObject_PreservesProvidedStorageModelFields(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2025, 1, 21, 11, 12, 13, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var captured *models.Object

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
		captured = args.Get(0).(*models.Object)
	}).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	repo := NewObjectRepository(mockDB, "test-table", "dev.simulacrum.greater.website", zap.NewNop())
	source := &models.Object{
		ID:             "https://remote.example/users/bob/statuses/2",
		Type:           activitypub.NoteType,
		AttributedTo:   "https://remote.example/users/bob",
		Content:        "already modeled",
		ConversationID: "conv-2",
		Visibility:     models.VisibilityDirect,
		IsRemote:       true,
		Published:      now,
		Updated:        now,
		CreatedAt:      now,
	}

	require.NoError(t, repo.CreateObject(ctx, source))
	require.NotNil(t, captured)
	require.True(t, captured.IsRemote)
	require.Equal(t, "conv-2", captured.ConversationID)
	require.Equal(t, models.VisibilityDirect, captured.Visibility)
	require.Equal(t, source.Content, captured.Content)
	require.Equal(t, source.Published, captured.Published)
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
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.CollectionItem")).Return(&core.PaginatedResult{}, dynamormErrors.ErrInvalidModel).Once()
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

	t.Run("GetQuoteType not found applies no per-note restriction", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		quoteType, err := repo.GetQuoteType(ctx, "note-1")
		require.NoError(t, err)
		require.Equal(t, models.VisibilityPublic, quoteType)
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

type invalidObjectJSONMarshaler struct{}

func (invalidObjectJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte("{"), nil
}

func TestObjectRepository_CreateObject_EarlyErrorPaths(t *testing.T) {
	ctx := context.Background()
	repo := NewObjectRepository(nil, "test-table", "example.com", zap.NewNop())

	err := repo.CreateObject(ctx, map[string]any{"bad": make(chan int)})
	require.Error(t, err)

	err = repo.CreateObject(ctx, invalidObjectJSONMarshaler{})
	require.Error(t, err)
}

func TestObjectRepository_ObjectModelHelperCoverage(t *testing.T) {
	repo := NewObjectRepository(nil, "test-table", "local.example", zap.NewNop())
	publishedAt := time.Date(2025, 2, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := publishedAt.Add(time.Minute)

	t.Run("populateObjectModelDefaults fills missing fields and infers remote metadata", func(t *testing.T) {
		objModel := &models.Object{}
		baseObj := activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/1",
			Type:      activitypub.NoteType,
			Context:   []any{"https://www.w3.org/ns/activitystreams"},
			Published: &publishedAt,
			Updated:   &updatedAt,
		}

		repo.populateObjectModelDefaults(objModel, baseObj, "https://remote.example/users/bob")

		require.Equal(t, baseObj.ID, objModel.ID)
		require.Equal(t, activitypub.NoteType, objModel.Type)
		require.Equal(t, "https://remote.example/users/bob", objModel.AttributedTo)
		require.Equal(t, publishedAt, objModel.Published)
		require.Equal(t, updatedAt, objModel.Updated)
		require.False(t, objModel.CreatedAt.IsZero())
		require.True(t, objModel.IsRemote)
		require.NotEmpty(t, objModel.ContextJSON)
	})

	t.Run("populateObjectModelDefaults preserves existing values and uses now fallback", func(t *testing.T) {
		objModel := &models.Object{
			ID:           "keep-id",
			Type:         activitypub.ArticleType,
			AttributedTo: "keep-actor",
			IsRemote:     true,
		}

		repo.populateObjectModelDefaults(objModel, activitypub.BaseObject{}, "")
		repo.populateObjectModelDefaults(nil, activitypub.BaseObject{}, "")

		require.Equal(t, "keep-id", objModel.ID)
		require.Equal(t, activitypub.ArticleType, objModel.Type)
		require.Equal(t, "keep-actor", objModel.AttributedTo)
		require.False(t, objModel.Published.IsZero())
		require.Equal(t, objModel.Published, objModel.Updated)
		require.False(t, objModel.CreatedAt.IsZero())
		require.True(t, objModel.IsRemote)
	})

	t.Run("cloneInputObjectModel copies slices and rejects unsupported inputs", func(t *testing.T) {
		source := &models.Object{
			To:  []string{"to-1"},
			CC:  []string{"cc-1"},
			BTo: []string{"bto-1"},
			BCC: []string{"bcc-1"},
		}

		clonedPtr, ok := cloneInputObjectModel(source)
		require.True(t, ok)
		require.NotSame(t, source, clonedPtr)

		clonedValue, ok := cloneInputObjectModel(*source)
		require.True(t, ok)

		source.To[0] = "changed"
		source.CC[0] = "changed"
		source.BTo[0] = "changed"
		source.BCC[0] = "changed"

		require.Equal(t, []string{"to-1"}, clonedPtr.To)
		require.Equal(t, []string{"cc-1"}, clonedPtr.CC)
		require.Equal(t, []string{"bto-1"}, clonedPtr.BTo)
		require.Equal(t, []string{"bcc-1"}, clonedPtr.BCC)
		require.Equal(t, []string{"to-1"}, clonedValue.To)

		_, ok = cloneInputObjectModel((*models.Object)(nil))
		require.False(t, ok)

		_, ok = cloneInputObjectModel(activitypub.Note{})
		require.False(t, ok)
	})

	t.Run("object reference helpers normalize hosts and detect remote references", func(t *testing.T) {
		require.Equal(t, "remote.example", normalizeObjectReferenceHost(" HTTPS://Remote.Example:443/users/bob "))
		require.Equal(t, "remote.example", normalizeObjectReferenceHost("remote.example/path"))
		require.Equal(t, "", normalizeObjectReferenceHost("   "))

		require.True(t, isRemoteObjectReference("https://remote.example/users/bob", "local.example"))
		require.False(t, isRemoteObjectReference("https://local.example/users/alice", "local.example"))
		require.False(t, isRemoteObjectReference("", "local.example"))
		require.False(t, isRemoteObjectReference("https://remote.example/users/bob", ""))
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
		*obj = *models.NewObject("note-1", "Page", "alice")
	}).Return(nil).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.TombstoneObject(ctx, "note-1", "alice"))
}

func TestObjectRepository_TombstoneObject_ArticleType(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)
	articleID := "https://example.com/articles/tombstone-article"

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var capturedTombstone *models.Tombstone
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		tombstone, ok := model.(*models.Tombstone)
		if ok {
			capturedTombstone = tombstone
		}
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Object")).Run(func(args mock.Arguments) {
		obj := args.Get(0).(*models.Object)
		*obj = *models.NewObject(articleID, activitypub.ArticleType, "https://example.com/users/alice")
		obj.Name = "Tombstone Article"
		obj.Summary = "Article tombstone summary"
	}).Return(nil).Once()
	setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

	require.NoError(t, repo.TombstoneObject(ctx, articleID, "https://example.com/users/alice"))
	require.NotNil(t, capturedTombstone)
	require.Equal(t, "OBJECT#"+articleID, capturedTombstone.PK)
	require.Equal(t, "TOMBSTONE", capturedTombstone.SK)
	require.Equal(t, "Tombstone", capturedTombstone.Type)
	require.Equal(t, activitypub.ArticleType, capturedTombstone.FormerType)
	require.NotZero(t, capturedTombstone.TTL)

	require.Equal(t, articleID, capturedTombstone.ID)
}

func TestObjectRepository_CountQuotes_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 16, 17, 18, 19, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.QuoteRelationship")).Return(&core.PaginatedResult{}, dynamormErrors.ErrInvalidModel).Once()
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
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Return(&core.PaginatedResult{}, dynamormErrors.ErrInvalidModel).Once()
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

	require.NoError(t, repo.ReplaceObjectWithTombstone(ctx, "note-1", activitypub.NoteType, "alice", "alice", true))
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
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		cleaned, err := repo.CleanupExpiredTombstones(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, 0, cleaned)
	})

	t.Run("CleanupExpiredTombstones continues when delete fails", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveObjectRepoMocks(mockDB, mockQuery, baseTime)

		repo := NewObjectRepository(mockDB, "test-table", "example.com", zap.NewNop())

		cleaned, err := repo.CleanupExpiredTombstones(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, 0, cleaned)
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
		mockQuery.On("Limit", 500).Return(mockQuery).Once()
		mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Return(&core.PaginatedResult{HasMore: false}, dynamormErrors.ErrInvalidModel).Once()
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
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Object")).Return(&core.PaginatedResult{}, dynamormErrors.ErrInvalidModel).Once()
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
