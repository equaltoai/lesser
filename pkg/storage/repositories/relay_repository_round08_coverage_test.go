package repositories

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRelayRepository_Round08_CoreOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("StoreRelayInfo success sets domain and ttl and creates item", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		start := time.Now()
		relay := &storage.RelayInfo{
			URL:        "https://www.example.com/relay",
			InboxURL:   "https://www.example.com/inbox",
			Active:     true,
			CreatedAt:  start.Add(-time.Minute),
			LastSeenAt: start.Add(-time.Second),
			Status:     "active",
			ErrorCount: 1,
		}

		err := repo.StoreRelayInfo(ctx, relay)
		require.NoError(t, err)
		assert.Equal(t, "example.com", relay.Domain)
		assert.GreaterOrEqual(t, relay.TTL, start.Unix())

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("StoreRelayInfo create error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.StoreRelayInfo(ctx, &storage.RelayInfo{
			URL:      "https://example.com/relay",
			InboxURL: "https://example.com/inbox",
			Active:   true,
		})
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetRelayInfo success converts model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Relay)
			*dest = models.Relay{
				URL:        "https://example.com/relay",
				InboxURL:   "https://example.com/inbox",
				Active:     true,
				Domain:     "example.com",
				Status:     "active",
				ErrorCount: 2,
				TTL:        123,
			}
		}).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		got, err := repo.GetRelayInfo(ctx, "https://example.com/relay")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "https://example.com/relay", got.URL)
		assert.Equal(t, "example.com", got.Domain)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetRelayInfo db error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetRelayInfo(ctx, "https://example.com/relay")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("RemoveRelayInfo success deletes item", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.RemoveRelayInfo(ctx, "https://example.com/relay")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("RemoveRelayInfo delete error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("Delete").Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.RemoveRelayInfo(ctx, "https://example.com/relay")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestRelayRepository_Round08_QueriesAndHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("GetActiveRelays success converts list", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "ACTIVE_RELAYS").Return(mockQuery).Once()
		mockQuery.On("Limit", 1000).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Relay)
			*dest = []models.Relay{{URL: "u1"}, {URL: "u2"}}
		}).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		relays, err := repo.GetActiveRelays(ctx)
		require.NoError(t, err)
		require.Len(t, relays, 2)
		assert.Equal(t, "u1", relays[0].URL)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetActiveRelays query error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "ACTIVE_RELAYS").Return(mockQuery).Once()
		mockQuery.On("Limit", 1000).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetActiveRelays(ctx)
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetAllRelays invalid cursor continues from start", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi8").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi8PK", "=", "RELAYS").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi8SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Relay)
			*dest = []models.Relay{{PK: "RELAY#a", SK: "INFO", URL: "https://example.com/relay"}}
		}).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		items, next, err := repo.GetAllRelays(ctx, 1, "not-base64")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Empty(t, next)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetAllRelays valid cursor applies where and yields next cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		cursor := encodeCursor(map[string]interface{}{"gsi8SK": "URL#https://example.com/relay#abc"})

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi8").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi8PK", "=", "RELAYS").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi8SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi8SK", ">", "URL#https://example.com/relay#abc").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Relay)
			*dest = []models.Relay{
				{PK: "RELAY#1", SK: "INFO", URL: "u1", GSI8PK: "RELAYS", GSI8SK: "URL#u1"},
				{PK: "RELAY#2", SK: "INFO", URL: "u2", GSI8PK: "RELAYS", GSI8SK: "URL#u2"},
			}
		}).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		items, next, err := repo.GetAllRelays(ctx, 1, cursor)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.NotEmpty(t, next)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetAllRelays query error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi8").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi8PK", "=", "RELAYS").Return(mockQuery).Once()
		mockQuery.On("OrderBy", "gsi8SK", "ASC").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		_, _, err := repo.GetAllRelays(ctx, 1, "")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("helper functions cover parse errors and encoding failures", func(t *testing.T) {
		assert.Equal(t, "bad://[::1", relayExtractDomainFromURL("bad://[::1"))
		assert.Equal(t, "example.com", relayExtractDomainFromURL("https://www.example.com/path"))

		assert.Empty(t, encodeCursor(map[string]interface{}{"bad": func() {}}))

		_, err := decodeCursor("!!!")
		require.Error(t, err)

		_, err = decodeCursor(base64.StdEncoding.EncodeToString([]byte("not-json")))
		require.Error(t, err)

		// Ensure not-found symbol is referenced for older tooling that dedupes imports.
		assert.True(t, dynamormerrors.IsNotFound(dynamormerrors.ErrItemNotFound))
	})
}

func TestRelayRepository_Round08_UpdateAndAliases(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateRelayStatus loads relay and re-creates with updated fields", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryCreate := new(mocks.MockQuery)

		// Get existing relay.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Relay)
			*dest = models.Relay{URL: "https://example.com/relay", InboxURL: "https://example.com/inbox", Active: false, Domain: "example.com"}
			_ = dest.UpdateKeys()
		}).Once()

		// ValidateAndCreate -> Create.
		mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
		mockQueryCreate.On("Create").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateRelayStatus(ctx, "https://example.com/relay", true)
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQueryGet.AssertExpectations(t)
		mockQueryCreate.AssertExpectations(t)
	})

	t.Run("UpdateRelayState updates status and error count", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryCreate := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Relay)
			*dest = models.Relay{URL: "https://example.com/relay", Active: true, Status: "active", ErrorCount: 0}
			_ = dest.UpdateKeys()
		}).Once()

		mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
		mockQueryCreate.On("Create").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateRelayState(ctx, "https://example.com/relay", storage.RelayState{Active: false, Status: "error", ErrorCount: 5})
		require.NoError(t, err)
	})

	t.Run("UpdateRelayStatus maps get failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		err := repo.UpdateRelayStatus(ctx, "https://example.com/relay", true)
		require.Error(t, err)
	})

	t.Run("UpdateRelayState maps create failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryGet := new(mocks.MockQuery)
		mockQueryCreate := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryGet).Once()
		mockQueryGet.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryGet).Twice()
		mockQueryGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Relay)
			*dest = models.Relay{URL: "https://example.com/relay", Active: true, Status: "active", ErrorCount: 0}
			_ = dest.UpdateKeys()
		}).Once()

		mockDB.On("Model", mock.Anything).Return(mockQueryCreate).Once()
		mockQueryCreate.On("Create").Return(assert.AnError).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		err := repo.UpdateRelayState(ctx, "https://example.com/relay", storage.RelayState{Active: true, Status: "active", ErrorCount: 0})
		require.Error(t, err)
	})

	t.Run("CreateRelay sets defaults and stores relay", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		relay := &storage.RelayInfo{URL: "https://www.example.com/relay", InboxURL: "https://www.example.com/inbox"}
		err := repo.CreateRelay(ctx, relay)
		require.NoError(t, err)
		assert.Equal(t, "pending", relay.Status)
		assert.Equal(t, 0, relay.ErrorCount)
		assert.NotZero(t, relay.CreatedAt)
	})

	t.Run("aliases delegate to underlying methods", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// GetRelay -> GetRelayInfo
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Relay)
			*dest = models.Relay{URL: "https://example.com/relay"}
		}).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.GetRelay(ctx, "https://example.com/relay")
		require.NoError(t, err)
	})

	t.Run("ListRelays and DeleteRelay aliases execute", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQueryScan := new(mocks.MockQuery)
		mockQueryDelete := new(mocks.MockQuery)

		// ListRelays -> GetAllRelays -> GSI8 query.
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Twice()
		mockDB.On("Model", mock.Anything).Return(mockQueryScan).Once()
		mockQueryScan.On("Index", "gsi8").Return(mockQueryScan).Once()
		mockQueryScan.On("Where", "gsi8PK", "=", "RELAYS").Return(mockQueryScan).Once()
		mockQueryScan.On("OrderBy", "gsi8SK", "ASC").Return(mockQueryScan).Once()
		mockQueryScan.On("Limit", 1001).Return(mockQueryScan).Once()
		mockQueryScan.On("All", mock.Anything).Return(nil).Once()

		// DeleteRelay -> RemoveRelayInfo -> BaseRepository.Delete.
		mockDB.On("Model", mock.Anything).Return(mockQueryDelete).Once()
		mockQueryDelete.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryDelete).Twice()
		mockQueryDelete.On("Delete").Return(nil).Once()

		repo := NewRelayRepository(mockDB, "table", zap.NewNop(), nil)
		_, err := repo.ListRelays(ctx)
		require.NoError(t, err)
		require.NoError(t, repo.DeleteRelay(ctx, "https://example.com/relay"))
	})
}
