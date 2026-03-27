package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRepository_WebAuthnCredentialOps(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 1
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	t.Run("CreateWebAuthnCredential success and error", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			err := repo.CreateWebAuthnCredential(ctx, &storage.WebAuthnCredential{
				ID:         "cred-1",
				UserID:     "user-1",
				PublicKey:  []byte("pk"),
				CreatedAt:  time.Time{},
				LastUsedAt: time.Time{},
			})
			require.NoError(t, err)
		})

		t.Run("create error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("Create").Return(errors.New("create failed")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			err := repo.CreateWebAuthnCredential(ctx, &storage.WebAuthnCredential{
				ID:     "cred-1",
				UserID: "user-1",
			})
			require.Error(t, err)
		})
	})

	t.Run("GetWebAuthnCredential scan error / empty / success", func(t *testing.T) {
		t.Run("query error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			_, err := repo.GetWebAuthnCredential(ctx, "cred-1")
			require.Error(t, err)
		})

		t.Run("empty results", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.WebAuthnCredential)
				*out = nil
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			cred, err := repo.GetWebAuthnCredential(ctx, "cred-1")
			require.NoError(t, err)
			require.Nil(t, cred)
		})

		t.Run("success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.WebAuthnCredential)
				*out = append(*out, models.WebAuthnCredential{
					ID:         "cred-1",
					UserID:     "user-1",
					PublicKey:  []byte("pk"),
					CreatedAt:  baseTime,
					LastUsedAt: baseTime,
					Type:       "WebAuthnCredential",
				})
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			cred, err := repo.GetWebAuthnCredential(ctx, "cred-1")
			require.NoError(t, err)
			require.Equal(t, "user-1", cred.UserID)
		})
	})

	t.Run("GetUserWebAuthnCredentials paginates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		call := 0
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			call++
			out := args.Get(0).(*[]*models.WebAuthnCredential)
			if call == 1 {
				for i := 0; i < 101; i++ {
					cred := &models.WebAuthnCredential{
						ID:     "cred",
						UserID: "user-1",
					}
					_ = cred.BeforeCreate()
					cred.SK = "WEBAUTHN_CRED#cred-" + string(rune('a'+(i%26)))
					*out = append(*out, cred)
				}
				return
			}
			// Second page: empty results stops loop.
		}).Return(nil).Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		creds, err := repo.GetUserWebAuthnCredentials(ctx, "user-1")
		require.NoError(t, err)
		require.NotEmpty(t, creds)
	})

	t.Run("DeleteWebAuthnCredential and UpdateWebAuthnLastUsed", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)

		require.NoError(t, repo.UpdateWebAuthnLastUsed(ctx, "cred-1", 2))
		require.NoError(t, repo.DeleteWebAuthnCredential(ctx, "cred-1"))

		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("Delete").Return(errors.New("delete failed")).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewAuthRepositoryWithCostTracking(mockDBErr, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repoErr.DeleteWebAuthnCredential(ctx, "cred-1"))
	})
}

func TestRound08_AuthRepository_ChallengeAndWalletOps(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 1
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	t.Run("WebAuthn challenge create/get/delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)

		var (
			challenge *storage.WebAuthnChallenge
			err       error
		)

		require.NoError(t, repo.CreateWebAuthnChallenge(ctx, &storage.WebAuthnChallenge{
			Challenge:   "c",
			UserID:      "user-1",
			SessionData: []byte("s"),
			ExpiresAt:   time.Now().Add(time.Minute),
			Type:        "authentication",
		}))

		// Not found returns nil, nil.
		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAuthRepositoryWithCostTracking(mockDBNF, "test-table", zaptest.NewLogger(t), costSvc)
		challenge, err = repoNF.GetWebAuthnChallenge(ctx, "missing")
		require.NoError(t, err)
		require.Nil(t, challenge)

		// Expired challenge triggers cleanup.
		mockDBExpired := new(mocks.MockDB)
		mockQueryExpired := new(mocks.MockQuery)
		mockQueryExpired.On("First", mock.Anything).Run(func(args mock.Arguments) {
			ch := args.Get(0).(*models.WebAuthnChallenge)
			ch.Challenge = "c"
			ch.UserID = "user-1"
			ch.ExpiresAt = baseTime.Add(-time.Minute)
			ch.Type = "authentication"
			_ = ch.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDBExpired, mockQueryExpired, nil, baseTime)
		repoExpired := NewAuthRepositoryWithCostTracking(mockDBExpired, "test-table", zaptest.NewLogger(t), costSvc)
		challenge, err = repoExpired.GetWebAuthnChallenge(ctx, "c")
		require.NoError(t, err)
		require.Nil(t, challenge)

		require.NoError(t, repo.DeleteWebAuthnChallenge(ctx, "c"))
	})

	t.Run("StoreWalletCredential covers retry helper branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// First Create (wallet credential) succeeds; second Create (reverse index) fails with non-recoverable error.
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(errors.New("validation exception")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.NoError(t, repo.StoreWalletCredential(ctx, &storage.WalletCredential{
			Username: "user-1",
			Address:  "0xAbC",
			ChainID:  1,
			Type:     "ethereum",
		}))

		// Directly exercise ctx.Done branch for recoverable errors.
		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Create").Return(errors.New("throttling")).Once()
		setupPermissiveRound08Mocks(mockDB2, mockQuery2, nil, baseTime)
		repo2 := NewAuthRepositoryWithCostTracking(mockDB2, "test-table", zaptest.NewLogger(t), costSvc)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := repo2.createReverseIndexWithRetry(cancelCtx, &models.WalletCredential{PK: "pk", SK: "sk"}, "user-1", "0xabc")
		require.Error(t, err)
	})

	t.Run("isRecoverableIndexError helper", func(t *testing.T) {
		repo := NewAuthRepository(new(mocks.MockDB), "test-table", zaptest.NewLogger(t))
		require.False(t, repo.isRecoverableIndexError(nil))
		require.False(t, repo.isRecoverableIndexError(errors.New("access denied")))
		require.True(t, repo.isRecoverableIndexError(errors.New("some unknown error")))
	})

	t.Run("GetWalletByAddress covers reverse index and fallback", func(t *testing.T) {
		t.Run("reverse index success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			// First All() call returns an index record with a username.
			mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
				value := reflect.ValueOf(args.Get(0))
				require.True(t, value.Kind() == reflect.Ptr && value.Elem().Kind() == reflect.Slice)
				elemType := value.Elem().Type().Elem()
				elem := reflect.New(elemType).Elem()
				usernameField := elem.FieldByName("Username")
				if usernameField.IsValid() && usernameField.CanSet() {
					usernameField.SetString("user-1")
				}
				value.Elem().Set(reflect.Append(value.Elem(), elem))
			}).Return(nil).Once()

			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xAbC")
			require.NoError(t, err)
			require.Equal(t, "user-1", cred.Username)
		})

		t.Run("fallback to scan then not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Return(nil).Once() // index empty
			mockQuery.On("Scan", mock.Anything).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
			repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
			cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xAbC")
			require.NoError(t, err)
			require.Nil(t, cred)
		})
	})

	t.Run("queryWalletCredentials clamps limits and sets next cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.WalletCredential)
			for i := 0; i < 26; i++ {
				w := models.WalletCredential{Username: "user-1", Address: "0xabc", Type: "ethereum"}
				_ = w.UpdateKeys()
				w.SK = w.SK + "-" + string(rune('a'+(i%26)))
				*out = append(*out, w)
			}
		}).Return(nil).Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)

		items, next, err := repo.queryWalletCredentials(ctx, "USER#user-1", "WALLET#", 0, "")
		require.NoError(t, err)
		require.Len(t, items, 25)
		require.NotEmpty(t, next)

		require.Equal(t, 25, clampWalletCredentialLimit(0))
		require.Equal(t, 100, clampWalletCredentialLimit(1000))
	})

	t.Run("DeleteWalletCredential handles delete error and reverse index cleanup", func(t *testing.T) {
		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockQueryErr.On("Delete").Return(errors.New("delete failed")).Once()
		setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
		repoErr := NewAuthRepositoryWithCostTracking(mockDBErr, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repoErr.DeleteWalletCredential(ctx, "user-1", "0xAbC"))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			w := args.Get(0).(*models.WalletCredential)
			w.Type = "ethereum"
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.NoError(t, repo.DeleteWalletCredential(ctx, "user-1", "0xAbC"))
	})

	t.Run("WalletChallenge store/get/delete", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)

		var (
			challenge *storage.WalletChallenge
			err       error
		)

		require.NoError(t, repo.StoreWalletChallenge(ctx, &storage.WalletChallenge{
			ID:                    "wc",
			Username:              "user-1",
			Address:               "0xabc",
			ChainID:               1,
			Nonce:                 "n",
			Message:               "m",
			IssuedAt:              time.Time{},
			ExpiresAt:             time.Now().Add(time.Minute),
			RegistrationCompleted: true,
		}))

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewAuthRepositoryWithCostTracking(mockDBNF, "test-table", zaptest.NewLogger(t), costSvc)
		challenge, err = repoNF.GetWalletChallenge(ctx, "missing")
		require.NoError(t, err)
		require.Nil(t, challenge)

		require.NoError(t, repo.DeleteWalletChallenge(ctx, "wc"))
	})
}
