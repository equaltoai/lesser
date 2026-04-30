package repositories

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("helpers", func(t *testing.T) {
		require.Equal(t, "alice", activityExtractUsernameFromActorID("https://example.com/users/alice"))
		require.Equal(t, "", activityExtractUsernameFromActorID("bad"))
		require.True(t, isInboxActivity(&activitypub.Activity{Actor: "https://x/users/bob"}, "alice"))
		require.False(t, isInboxActivity(&activitypub.Activity{Actor: "https://x/users/alice"}, "alice"))

		require.Equal(t, "hello", extractContent(activitypub.Activity{Object: map[string]interface{}{"content": "hello"}}))
		require.Equal(t, "", extractContent(activitypub.Activity{Object: map[string]interface{}{"nope": "x"}}))

		encoded := activityEncodeCursor(map[string]string{"PK": "a", "SK": "b"})
		require.NotEmpty(t, encoded)
		_, err := activityDecodeCursor("not-base64")
		require.Error(t, err)

		// Malformed base64 but passes character validation.
		_, err = activityDecodeCursor(base64.URLEncoding.EncodeToString([]byte("{")))
		require.Error(t, err)

		// Valid character set, invalid base64 encoding length.
		_, err = activityDecodeCursor("a")
		require.Error(t, err)
	})

	t.Run("CreateActivity validates required params", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.Error(t, repo.CreateActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: ""}}))
		require.Error(t, repo.CreateActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "id"}, Actor: "bad"}))
	})

	t.Run("CreateActivity success and create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		okAct := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "id", Type: "Create"},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]interface{}{"content": "hello"},
		}
		require.NoError(t, repo.CreateActivity(ctx, okAct))

		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewActivityRepository(mockDBErr, "test-table", zap.NewNop(), nil)
		require.Error(t, repoErr.CreateActivity(ctx, okAct))
	})

	t.Run("GetActivity queries by activity ID", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out, &models.Activity{Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "a2"}}})
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		got, err := repo.GetActivity(ctx, "a2")
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "a2", got.ID)

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.Activity)
			*out = nil
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewActivityRepository(mockDBNF, "test-table", zap.NewNop(), nil)
		missing, err := repoNF.GetActivity(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, missing)
	})

	t.Run("DeleteActivity deletes by activity ID and noops when missing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out, &models.Activity{
					PK: "ACTOR#alice",
					SK: "ACTIVITY#2025-01-01T00:00:00Z#act-1",
				})
			}).
			Return(nil).
			Once()
		mockQuery.On("Delete").Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.DeleteActivity(ctx, "act-1"))

		mockDBMissing := new(mocks.MockDB)
		mockQueryMissing := new(mocks.MockQuery)
		mockQueryMissing.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = nil
			}).
			Return(nil).
			Once()
		setupPermissiveRound08Mocks(mockDBMissing, mockQueryMissing, nil, baseTime)

		repoMissing := NewActivityRepository(mockDBMissing, "test-table", zap.NewNop(), nil)
		require.NoError(t, repoMissing.DeleteActivity(ctx, "missing"))

		mockDBLookupErr := new(mocks.MockDB)
		mockQueryLookupErr := new(mocks.MockQuery)
		mockQueryLookupErr.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBLookupErr, mockQueryLookupErr, nil, baseTime)

		repoLookupErr := NewActivityRepository(mockDBLookupErr, "test-table", zap.NewNop(), nil)
		require.Error(t, repoLookupErr.DeleteActivity(ctx, "lookup-boom"))

		mockDBDeleteErr := new(mocks.MockDB)
		mockQueryDeleteErr := new(mocks.MockQuery)
		mockQueryDeleteErr.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out, &models.Activity{
					PK: "ACTOR#alice",
					SK: "ACTIVITY#2025-01-01T00:00:00Z#act-delete-boom",
				})
			}).
			Return(nil).
			Once()
		mockQueryDeleteErr.On("Delete").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBDeleteErr, mockQueryDeleteErr, nil, baseTime)

		repoDeleteErr := NewActivityRepository(mockDBDeleteErr, "test-table", zap.NewNop(), nil)
		require.Error(t, repoDeleteErr.DeleteActivity(ctx, "act-delete-boom"))
	})

	t.Run("Inbox/outbox pagination and GetCollection branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out,
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#z#1", GSI1PK: "INBOX#alice", GSI1SK: "z", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "1", To: []string{activitypub.PublicAddress}}}},
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#y#2", GSI1PK: "INBOX#alice", GSI1SK: "y", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "2", To: []string{activitypub.PublicAddress}}}},
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#x#3", GSI1PK: "INBOX#alice", GSI1SK: "x", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "3", To: []string{activitypub.PublicAddress}}}},
				)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.costService = newRound09CostService()
		defer func() { _ = repo.costService.Close(ctx) }()

		inbox, cursor, err := repo.GetInboxActivities(ctx, "alice", 2, "invalid@@")
		require.NoError(t, err)
		require.Len(t, inbox, 2)
		require.NotEmpty(t, cursor)

		outbox, cursor2, err := repo.GetOutboxActivities(ctx, "alice", 2, "")
		require.NoError(t, err)
		require.Len(t, outbox, 2)
		require.NotEmpty(t, cursor2)

		page, err := repo.GetCollection(ctx, "alice", activitypub.InboxCollection, 2, "")
		require.NoError(t, err)
		require.NotNil(t, page)

		page, err = repo.GetCollection(ctx, "alice", activitypub.OutboxCollection, 2, "")
		require.NoError(t, err)
		require.NotNil(t, page)

		page, err = repo.GetCollection(ctx, "alice", activitypub.FollowersCollection, 2, "")
		require.Error(t, err)
		require.Nil(t, page)

		page, err = repo.GetCollection(ctx, "alice", "unknown", 2, "")
		require.NoError(t, err)
		require.NotNil(t, page)
	})

	t.Run("clampActivityLimit", func(t *testing.T) {
		require.Equal(t, activityDefaultLimit, clampActivityLimit(0))
		require.Equal(t, activityDefaultLimit, clampActivityLimit(-1))
		require.Equal(t, activityMaxLimit, clampActivityLimit(activityMaxLimit+1))
	})

	t.Run("Weekly and hashtag activity", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out,
					&models.Activity{CreatedAt: baseTime, Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "c1", Type: "Create"}, Object: map[string]interface{}{"content": "hello #Go"}}},
					&models.Activity{CreatedAt: baseTime, Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "c2", Type: "Like"}, Object: map[string]interface{}{"content": "no tag"}}},
				)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.costService = newRound09CostService()
		defer func() { _ = repo.costService.Close(ctx) }()
		weekly, err := repo.GetWeeklyActivity(ctx, baseTime.Unix())
		require.NoError(t, err)
		require.NotNil(t, weekly)
		require.Equal(t, 1, weekly.Statuses)

		acts, err := repo.GetHashtagActivity(ctx, "go", baseTime.Add(-time.Hour))
		require.NoError(t, err)
		require.Len(t, acts, 1)
	})

	t.Run("RecordFederationActivity handles create errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.Error(t, repo.RecordFederationActivity(ctx, &storage.FederationActivity{
			ID:        "id",
			Domain:    "example.com",
			Type:      "inbox",
			Timestamp: baseTime,
		}))
	})

	t.Run("RecordFederationActivity success tracks cost", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
		defer func() { _ = repo.costService.Close(ctx) }()

		require.NoError(t, repo.RecordFederationActivity(ctx, &storage.FederationActivity{
			ID:           "id",
			Domain:       "example.com",
			Type:         "inbox",
			ActivityType: "Create",
			ByteSize:     10,
			Success:      true,
			Timestamp:    baseTime,
		}))
	})

	t.Run("RecordActivity covers direct create path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.RecordActivity(ctx, "type", "actor", baseTime))

		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("Create").Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewActivityRepository(mockDBErr, "test-table", zap.NewNop(), nil)
		require.Error(t, repoErr.RecordActivity(ctx, "type", "actor", baseTime))
	})

	t.Run("GetInboxActivities decodes valid cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		validCursor := activityEncodeCursor(map[string]string{"gsi1PK": "INBOX#alice", "gsi1SK": "x", "PK": "ACTOR#alice", "SK": "ACTIVITY#x#1"})
		_, _, err := repo.GetInboxActivities(ctx, "alice", 1, validCursor)
		require.NoError(t, err)
	})

	t.Run("GetOutboxActivities filters non-public activities", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.
			On("All", mock.Anything).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.Activity)
				*out = append(*out,
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#z#public", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "public", To: []string{activitypub.PublicAddress}}}},
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#y#direct", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "direct", To: []string{"https://example.com/users/bob"}}}},
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#x#unlisted", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "unlisted", CC: []string{activitypub.PublicAddress}}}},
					&models.Activity{PK: "ACTOR#alice", SK: "ACTIVITY#w#object", Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "object"}, Object: &activitypub.Note{BaseObject: activitypub.BaseObject{To: []string{activitypub.PublicAddress}}}}},
				)
			}).
			Return(nil).
			Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		outbox, cursor, err := repo.GetOutboxActivities(ctx, "alice", 10, "")
		require.NoError(t, err)
		require.Empty(t, cursor)
		require.Len(t, outbox, 3)
		require.Equal(t, "public", outbox[0].ID)
		require.Equal(t, "unlisted", outbox[1].ID)
		require.Equal(t, "object", outbox[2].ID)
	})

	t.Run("GetOutboxActivities stops after bounded non-public pages", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		allCalls := 0
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			allCalls++
			out := args.Get(0).(*[]*models.Activity)
			if allCalls <= 6 {
				*out = []*models.Activity{
					{PK: "ACTOR#alice", SK: fmt.Sprintf("ACTIVITY#z%d#private", allCalls), Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: fmt.Sprintf("private-%d-a", allCalls), To: []string{"https://example.com/users/bob"}}}},
					{PK: "ACTOR#alice", SK: fmt.Sprintf("ACTIVITY#y%d#private", allCalls), Activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: fmt.Sprintf("private-%d-b", allCalls), To: []string{"https://example.com/users/bob"}}}},
				}
			}
		}).Return(nil)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		outbox, cursor, err := repo.GetOutboxActivities(ctx, "alice", 1, "")
		require.NoError(t, err)
		require.Empty(t, outbox)
		require.NotEmpty(t, cursor)
		require.Equal(t, 4, allCalls)
	})

	t.Run("GetOutboxActivities decodes valid cursor and handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.Activity)
			*out = nil
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)

		validCursor := activityEncodeCursor(map[string]string{"PK": "ACTOR#alice", "SK": "ACTIVITY#x#1"})
		_, _, err := repo.GetOutboxActivities(ctx, "alice", 1, validCursor)
		require.NoError(t, err)

		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("All", mock.Anything).Return(ErrTestMockError).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewActivityRepository(mockDBErr, "test-table", zap.NewNop(), nil)
		_, _, err = repoErr.GetOutboxActivities(ctx, "alice", 1, "")
		require.Error(t, err)
	})
}
