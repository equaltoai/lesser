package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

type round08BadChallenge struct{}

func (round08BadChallenge) UpdateKeys() error { return errors.New("bad keys") }

func TestRound08_AuthRepository_MoreCoverage(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 50
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	t.Run("createChallenge UpdateKeys error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repo.createChallenge(ctx, round08BadChallenge{}, "op", "name", "id"))
	})

	t.Run("getChallengeModel error propagation and deleteChallengeModel error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("get failed")).Once()
		mockQuery.On("Delete").Return(errors.New("delete failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		_, err := repo.getWebAuthnChallenge(ctx, "pk", "sk")
		require.Error(t, err)
		require.Error(t, repo.deleteWebAuthnChallenge(ctx, "pk", "sk"))
	})

	t.Run("GetUserWallets query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		_, err := repo.GetUserWallets(ctx, "user-1")
		require.Error(t, err)
	})

	t.Run("DeleteWalletCredential without reverse index type", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		mockQuery.On("First", mock.Anything).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.NoError(t, repo.DeleteWalletCredential(ctx, "user-1", "0xabc"))
	})

	t.Run("GetWalletByAddress reverse index invalid username", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			// Append an index record with empty Username so we hit the validation branch.
			value := reflect.ValueOf(args.Get(0))
			require.True(t, value.Kind() == reflect.Ptr && value.Elem().Kind() == reflect.Slice)
			elemType := value.Elem().Type().Elem()
			elem := reflect.New(elemType).Elem()
			value.Elem().Set(reflect.Append(value.Elem(), elem))
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xabc")
		require.NoError(t, err)
		require.Nil(t, cred)
	})

	t.Run("CreateWebAuthnChallenge with non-byte session data", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.NoError(t, repo.CreateWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
			Challenge:   "c",
			UserID:      "user-1",
			SessionData: "not-bytes",
			ExpiresAt:   baseTime.Add(time.Minute),
			Type:        "authentication",
		}))
	})
}
