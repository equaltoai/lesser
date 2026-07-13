package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_DNSCacheRepository_SetGetInvalidate(t *testing.T) {
	ctx := context.Background()

	t.Run("set requires entry", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.ErrorIs(t, repo.SetDNSCache(ctx, nil), ErrDNSCacheEntryRequired)
	})

	t.Run("get not found maps to repository not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound)

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		entry, err := repo.GetDNSCache(ctx, "example.com")
		require.Nil(t, entry)
		require.Error(t, err)
	})

	t.Run("get non-notfound returns wrapped error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom"))

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetDNSCache(ctx, "example.com")
		require.ErrorIs(t, err, ErrDNSCacheGetFailed)
	})

	t.Run("get expired invalidates and returns not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dns := args.Get(0).(*models.DNSCache)
			dns.Hostname = "example.com"
			dns.IPs = []string{"127.0.0.1"}
			dns.ResolvedAt = time.Now().Add(-time.Minute)
			dns.TTL = 1
			dns.ExpiresAt = time.Now().Add(-time.Second).Unix()
			_ = dns.UpdateKeys()
		}).Return(nil)
		mockQuery.On("Delete").Return(nil)

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		entry, err := repo.GetDNSCache(ctx, "example.com")
		require.Nil(t, entry)
		require.Error(t, err)
	})

	t.Run("get returns storage model when fresh", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dns := args.Get(0).(*models.DNSCache)
			dns.Hostname = "example.com"
			dns.IPs = []string{"127.0.0.1"}
			dns.ResolvedAt = time.Now().Add(-time.Minute)
			dns.TTL = 60
			dns.ExpiresAt = time.Now().Add(10 * time.Minute).Unix()
			_ = dns.UpdateKeys()
		}).Return(nil)

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		entry, err := repo.GetDNSCache(ctx, "example.com")
		require.NoError(t, err)
		require.Equal(t, "example.com", entry.Hostname)
		require.Equal(t, int64(60), entry.TTL)
	})

	t.Run("set create error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("boom"))

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		err := repo.SetDNSCache(ctx, &storage.DNSCacheEntry{Hostname: "example.com", IPs: []string{"127.0.0.1"}, ResolvedAt: time.Now(), TTL: 60})
		require.ErrorIs(t, err, ErrDNSCacheSetFailed)
	})

	t.Run("invalidate notfound is ignored; other errors are wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		mockQuery.On("Delete").Return(errors.New("boom")).Once()

		repo := NewDNSCacheRepository(mockDB, "test-table", zap.NewNop(), nil)
		require.NoError(t, repo.InvalidateDNSCache(ctx, "example.com"))
		require.ErrorIs(t, repo.InvalidateDNSCache(ctx, "example.com"), ErrDNSCacheInvalidateFailed)
	})
}
