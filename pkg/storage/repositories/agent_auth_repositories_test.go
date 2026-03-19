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
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestAgentAccessLeaseRepository_CreateChallenge_PreparesModel(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	challenge := &models.AgentAccessLeaseChallenge{
		ID:                " challenge-1 ",
		LeaseID:           " lease-1 ",
		Username:          " agent-1 ",
		Action:            " principal_approve ",
		Address:           "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD",
		PrincipalUsername: " owner ",
		PrincipalWallet:   "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD",
		AgentWallet:       "0x0123456789012345678901234567890123456789",
		Message:           "sign me",
		ExpiresAt:         expiresAt,
	}

	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		m, ok := model.(*models.AgentAccessLeaseChallenge)
		if !ok {
			return false
		}
		return m.PK == "AGENT_ACCESS_CHALLENGE#challenge-1" &&
			m.SK == "CHALLENGE" &&
			m.TTL == expiresAt.Unix() &&
			m.Address == "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" &&
			m.PrincipalWallet == "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" &&
			m.AgentWallet == "0x0123456789012345678901234567890123456789"
	})).Return(mockQuery).Once()

	repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
	require.NoError(t, repo.CreateChallenge(context.Background(), challenge))
	require.Equal(t, "AGENT_ACCESS_CHALLENGE#challenge-1", challenge.PK)
	require.Equal(t, "CHALLENGE", challenge.SK)
	require.Equal(t, expiresAt.Unix(), challenge.TTL)
	require.False(t, challenge.IssuedAt.IsZero())
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAgentAccessLeaseRepository_CreateLease_PreparesModel(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	now := time.Now().UTC()
	lease := &models.AgentAccessLease{
		ID:                " lease-1 ",
		Username:          " agent-1 ",
		PrincipalUsername: "owner",
		PrincipalWallet:   "0x1111111111111111111111111111111111111111",
		AgentWallet:       "0x2222222222222222222222222222222222222222",
		Scopes:            []string{"read", "write"},
		DeviceLabel:       "local-agent",
		IdleTimeoutHours:  24,
		IdleExpiresAt:     now.Add(24 * time.Hour),
		AbsoluteExpiresAt: now.Add(48 * time.Hour),
	}

	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		m, ok := model.(*models.AgentAccessLease)
		if !ok {
			return false
		}
		return m.PK == "AGENT_ACCESS_LEASE#agent-1" &&
			m.SK == "LEASE#lease-1" &&
			m.TTL == lease.AbsoluteExpiresAt.Unix() &&
			m.Status == "active" &&
			m.LeaseVersion == 1 &&
			!m.CreatedAt.IsZero() &&
			!m.LastUsedAt.IsZero()
	})).Return(mockQuery).Once()

	repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
	require.NoError(t, repo.CreateLease(context.Background(), lease))
	require.Equal(t, "AGENT_ACCESS_LEASE#agent-1", lease.PK)
	require.Equal(t, "LEASE#lease-1", lease.SK)
	require.Equal(t, lease.AbsoluteExpiresAt.Unix(), lease.TTL)
	require.Equal(t, "active", lease.Status)
	require.Equal(t, 1, lease.LeaseVersion)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAgentAccessLeaseRepository_MarkChallengeUsed_UsesConditionalUpdate(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockUpdate := new(dynamormmocks.MockUpdateBuilder)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		m, ok := model.(*models.AgentAccessLeaseChallenge)
		return ok && m.PK == "AGENT_ACCESS_CHALLENGE#challenge-1" && m.SK == "CHALLENGE"
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_CHALLENGE#challenge-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
	mockUpdate.On("Set", "Used", true).Return(mockUpdate).Once()
	mockUpdate.On("Condition", "Used", "=", false).Return(mockUpdate).Once()
	mockUpdate.On("Condition", "TTL", ">", mock.AnythingOfType("int64")).Return(mockUpdate).Once()
	mockUpdate.On("Execute").Return(nil).Once()

	repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
	require.NoError(t, repo.MarkChallengeUsed(context.Background(), "challenge-1", time.Now().UTC()))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func TestAgentAccessLeaseRepository_UpdatePaths_UseKeyConditions(t *testing.T) {
	t.Run("RevokeLease", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockUpdate := new(dynamormmocks.MockUpdateBuilder)
		now := time.Now().UTC()
		lease := &models.AgentAccessLease{
			ID:       "lease-1",
			Username: "agent-1",
		}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.MatchedBy(func(model any) bool {
			m, ok := model.(*models.AgentAccessLease)
			return ok && m.PK == "AGENT_ACCESS_LEASE#agent-1" && m.SK == "LEASE#lease-1"
		})).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
		mockUpdate.On("Set", "TTL", now.Unix()).Return(mockUpdate).Once()
		mockUpdate.On("Set", "Status", "revoked").Return(mockUpdate).Once()
		mockUpdate.On("Set", "UpdatedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Set", "RevokedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Set", "RevokedBy", "owner").Return(mockUpdate).Once()
		mockUpdate.On("Set", "RevokedReason", "expired").Return(mockUpdate).Once()
		mockUpdate.On("Execute").Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		require.NoError(t, repo.RevokeLease(context.Background(), lease, "owner", "expired", now))
		require.Equal(t, "AGENT_ACCESS_LEASE#agent-1", lease.PK)
		require.Equal(t, "LEASE#lease-1", lease.SK)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockUpdate.AssertExpectations(t)
	})

	t.Run("AuthorizeSessionKey", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockUpdate := new(dynamormmocks.MockUpdateBuilder)
		now := time.Now().UTC()
		lease := &models.AgentAccessLease{PK: "AGENT_ACCESS_LEASE#agent-1", SK: "LEASE#lease-1", ID: "lease-1", Username: "agent-1"}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", lease).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
		mockUpdate.On("Set", "SessionPublicKey", "pub").Return(mockUpdate).Once()
		mockUpdate.On("Set", "SessionKeyType", "ed25519").Return(mockUpdate).Once()
		mockUpdate.On("Set", "SessionKeyCreatedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Set", "UpdatedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Execute").Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		require.NoError(t, repo.AuthorizeSessionKey(context.Background(), lease, "pub", "ed25519", now))
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockUpdate.AssertExpectations(t)
	})

	t.Run("RecordLeaseUse", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockUpdate := new(dynamormmocks.MockUpdateBuilder)
		now := time.Now().UTC()
		idleExpiresAt := now.Add(time.Hour)
		lease := &models.AgentAccessLease{PK: "AGENT_ACCESS_LEASE#agent-1", SK: "LEASE#lease-1", ID: "lease-1", Username: "agent-1"}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", lease).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
		mockUpdate.On("Set", "LastUsedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Set", "IdleExpiresAt", idleExpiresAt).Return(mockUpdate).Once()
		mockUpdate.On("Set", "UpdatedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Set", "SessionKeyLastUsedAt", now).Return(mockUpdate).Once()
		mockUpdate.On("Execute").Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		require.NoError(t, repo.RecordLeaseUse(context.Background(), lease, idleExpiresAt, now, true))
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockUpdate.AssertExpectations(t)
	})
}

func TestAgentAccessLeaseRepository_ReadPaths(t *testing.T) {
	t.Run("GetChallenge", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLeaseChallenge")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_CHALLENGE#challenge-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLeaseChallenge")).
			Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.AgentAccessLeaseChallenge)
				*dest = models.AgentAccessLeaseChallenge{
					ID:       "challenge-1",
					LeaseID:  "lease-1",
					Username: "agent-1",
				}
			}).
			Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		challenge, err := repo.GetChallenge(context.Background(), "challenge-1")
		require.NoError(t, err)
		require.Equal(t, "challenge-1", challenge.ID)
		require.Equal(t, "lease-1", challenge.LeaseID)
		require.Equal(t, "agent-1", challenge.Username)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("GetLease", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLease")).
			Run(func(args mock.Arguments) {
				dest := args.Get(0).(*models.AgentAccessLease)
				*dest = models.AgentAccessLease{
					ID:       "lease-1",
					Username: "agent-1",
					Status:   "active",
				}
			}).
			Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		lease, err := repo.GetLease(context.Background(), "agent-1", "lease-1")
		require.NoError(t, err)
		require.Equal(t, "lease-1", lease.ID)
		require.Equal(t, "agent-1", lease.Username)
		require.Equal(t, "active", lease.Status)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("ListLeases", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", "LEASE#").Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.AgentAccessLease")).
			Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.AgentAccessLease)
				*dest = []models.AgentAccessLease{
					{ID: "lease-1", Username: "agent-1"},
					{ID: "lease-2", Username: "agent-1"},
				}
			}).
			Return(nil).Once()

		repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
		leases, err := repo.ListLeases(context.Background(), "agent-1")
		require.NoError(t, err)
		require.Len(t, leases, 2)
		require.Equal(t, "lease-1", leases[0].ID)
		require.Equal(t, "lease-2", leases[1].ID)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("query errors surface", func(t *testing.T) {
		t.Run("GetChallenge", func(t *testing.T) {
			mockDB := new(dynamormmocks.MockDB)
			mockQuery := new(dynamormmocks.MockQuery)
			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLeaseChallenge")).Return(mockQuery).Once()
			mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_CHALLENGE#challenge-1").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
			mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLeaseChallenge")).Return(errors.New("boom")).Once()

			repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
			_, err := repo.GetChallenge(context.Background(), "challenge-1")
			require.ErrorContains(t, err, "boom")
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("GetLease", func(t *testing.T) {
			mockDB := new(dynamormmocks.MockDB)
			mockQuery := new(dynamormmocks.MockQuery)
			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
			mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
			mockQuery.On("First", mock.AnythingOfType("*models.AgentAccessLease")).Return(errors.New("boom")).Once()

			repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
			_, err := repo.GetLease(context.Background(), "agent-1", "lease-1")
			require.ErrorContains(t, err, "boom")
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})

		t.Run("ListLeases", func(t *testing.T) {
			mockDB := new(dynamormmocks.MockDB)
			mockQuery := new(dynamormmocks.MockQuery)
			mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
			mockDB.On("Model", mock.AnythingOfType("*models.AgentAccessLease")).Return(mockQuery).Once()
			mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
			mockQuery.On("Where", "SK", "BEGINS_WITH", "LEASE#").Return(mockQuery).Once()
			mockQuery.On("All", mock.AnythingOfType("*[]models.AgentAccessLease")).Return(errors.New("boom")).Once()

			repo := NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop())
			_, err := repo.ListLeases(context.Background(), "agent-1")
			require.ErrorContains(t, err, "boom")
			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	})
}

func TestAgentKeyChallengeRepository_Create_PreparesModel(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	challenge := &models.AgentKeyChallenge{
		ID:        " challenge-1 ",
		Username:  " alice ",
		Action:    " register ",
		Nonce:     "nonce",
		Message:   "hello",
		ExpiresAt: expiresAt,
	}

	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		m, ok := model.(*models.AgentKeyChallenge)
		if !ok {
			return false
		}
		return m.PK == "AGENT_KEY_CHALLENGE#challenge-1" &&
			m.SK == "CHALLENGE" &&
			m.TTL == expiresAt.Unix()
	})).Return(mockQuery).Once()

	repo := NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop())
	require.NoError(t, repo.Create(context.Background(), challenge))
	require.Equal(t, "AGENT_KEY_CHALLENGE#challenge-1", challenge.PK)
	require.Equal(t, "CHALLENGE", challenge.SK)
	require.Equal(t, expiresAt.Unix(), challenge.TTL)
	require.False(t, challenge.IssuedAt.IsZero())
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAgentKeyChallengeRepository_Get_Success(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentKeyChallenge")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AGENT_KEY_CHALLENGE#challenge-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.AgentKeyChallenge")).
		Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.AgentKeyChallenge)
			*dest = models.AgentKeyChallenge{
				ID:       "challenge-1",
				Username: "alice",
				Action:   "register",
			}
		}).
		Return(nil).Once()

	repo := NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop())
	challenge, err := repo.Get(context.Background(), "challenge-1")
	require.NoError(t, err)
	require.Equal(t, "challenge-1", challenge.ID)
	require.Equal(t, "alice", challenge.Username)
	require.Equal(t, "register", challenge.Action)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAgentKeyChallengeRepository_Get_Error(t *testing.T) {
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentKeyChallenge")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AGENT_KEY_CHALLENGE#challenge-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.AgentKeyChallenge")).Return(errors.New("boom")).Once()

	repo := NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop())
	_, err := repo.Get(context.Background(), "challenge-1")
	require.ErrorContains(t, err, "boom")
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAgentAuthRepositoryHelpers(t *testing.T) {
	t.Run("constructors default nil logger", func(t *testing.T) {
		leaseRepo := NewAgentAccessLeaseRepository(new(dynamormmocks.MockDB), "table", nil)
		require.NotNil(t, leaseRepo.logger)

		keyRepo := NewAgentKeyChallengeRepository(new(dynamormmocks.MockDB), "table", nil)
		require.NotNil(t, keyRepo.logger)
	})

	t.Run("create helper rejects nil pointer models", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		var nilChallenge *models.AgentKeyChallenge
		err := createPreparedModel(
			context.Background(),
			mockDB,
			zap.NewNop(),
			nilChallenge,
			"failed to prepare",
			"failed to create",
			func(*models.AgentKeyChallenge) []zap.Field { return nil },
		)
		require.ErrorIs(t, err, storage.ErrInvalidInput)
	})

	t.Run("isNilModel handles value types", func(t *testing.T) {
		require.False(t, isNilModel(models.AgentKeyChallenge{}))
	})

	t.Run("mark helper rejects nil db", func(t *testing.T) {
		var nilCoreDB dynamormcore.DB
		err := markChallengeUsed(
			context.Background(),
			nilCoreDB,
			zap.NewNop(),
			"challenge-1",
			time.Time{},
			&models.AgentKeyChallenge{PK: "AGENT_KEY_CHALLENGE#challenge-1", SK: "CHALLENGE"},
			"failed to mark",
		)
		require.ErrorIs(t, err, storage.ErrDatabaseConnectionFailed)
	})

	t.Run("create helper surfaces preparation errors", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		err := createPreparedModel(
			context.Background(),
			mockDB,
			zap.NewNop(),
			&models.AgentKeyChallenge{},
			"failed to prepare",
			"failed to create",
			func(*models.AgentKeyChallenge) []zap.Field { return nil },
		)
		require.ErrorContains(t, err, "missing required fields")
		mockDB.AssertNotCalled(t, "WithContext", mock.Anything)
	})

	t.Run("create helper surfaces storage errors", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.MatchedBy(func(model any) bool {
			challenge, ok := model.(*models.AgentKeyChallenge)
			return ok && challenge.PK == "AGENT_KEY_CHALLENGE#challenge-1" && challenge.SK == "CHALLENGE"
		})).Return(mockQuery).Once()
		mockQuery.On("IfNotExists").Return(mockQuery).Once()
		mockQuery.On("Create").Return(errors.New("boom")).Once()

		err := createPreparedModel(
			context.Background(),
			mockDB,
			zap.NewNop(),
			&models.AgentKeyChallenge{
				ID:        "challenge-1",
				Username:  "alice",
				Action:    "register",
				Message:   "hello",
				ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			},
			"failed to prepare",
			"failed to create",
			func(challenge *models.AgentKeyChallenge) []zap.Field {
				return []zap.Field{zap.String("challenge_id", challenge.ID)}
			},
		)
		require.ErrorContains(t, err, "boom")
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("mark helper surfaces update errors", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockUpdate := new(dynamormmocks.MockUpdateBuilder)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.AgentKeyChallenge")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_KEY_CHALLENGE#challenge-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "CHALLENGE").Return(mockQuery).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()
		mockUpdate.On("Set", "Used", true).Return(mockUpdate).Once()
		mockUpdate.On("Condition", "Used", "=", false).Return(mockUpdate).Once()
		mockUpdate.On("Condition", "TTL", ">", mock.AnythingOfType("int64")).Return(mockUpdate).Once()
		mockUpdate.On("Execute").Return(errors.New("boom")).Once()

		err := markChallengeUsed(
			context.Background(),
			mockDB,
			zap.NewNop(),
			"challenge-1",
			time.Time{},
			&models.AgentKeyChallenge{PK: "AGENT_KEY_CHALLENGE#challenge-1", SK: "CHALLENGE"},
			"failed to mark",
		)
		require.ErrorContains(t, err, "boom")
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
		mockUpdate.AssertExpectations(t)
	})

	t.Run("keyed update builder derives missing keys", func(t *testing.T) {
		mockDB := new(dynamormmocks.MockDB)
		mockQuery := new(dynamormmocks.MockQuery)
		mockUpdate := new(dynamormmocks.MockUpdateBuilder)
		lease := &models.AgentAccessLease{
			ID:       "lease-1",
			Username: "agent-1",
		}

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.MatchedBy(func(model any) bool {
			m, ok := model.(*models.AgentAccessLease)
			return ok && m.PK == "AGENT_ACCESS_LEASE#agent-1" && m.SK == "LEASE#lease-1"
		})).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "AGENT_ACCESS_LEASE#agent-1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "LEASE#lease-1").Return(mockQuery).Once()
		mockQuery.On("UpdateBuilder").Return(mockUpdate).Once()

		update, err := keyedUpdateBuilder(context.Background(), mockDB, lease)
		require.NoError(t, err)
		require.NotNil(t, update)
		require.Equal(t, "AGENT_ACCESS_LEASE#agent-1", lease.PK)
		require.Equal(t, "LEASE#lease-1", lease.SK)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestAgentAuthRepositories_InvalidInputAndStorage(t *testing.T) {
	var nilCoreDB dynamormcore.DB

	leaseRepo := NewAgentAccessLeaseRepository(nilCoreDB, "table", zap.NewNop())
	_, err := leaseRepo.GetLease(context.Background(), "agent", "lease")
	require.ErrorIs(t, err, storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.CreateChallenge(context.Background(), nil), storage.ErrDatabaseConnectionFailed)

	keyRepo := NewAgentKeyChallengeRepository(nilCoreDB, "table", zap.NewNop())
	_, err = keyRepo.Get(context.Background(), "challenge")
	require.ErrorIs(t, err, storage.ErrDatabaseConnectionFailed)

	mockDB := new(dynamormmocks.MockDB)
	_, err = NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).GetChallenge(context.Background(), " ")
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	_, err = NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).GetLease(context.Background(), " ", "lease")
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	_, err = NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).ListLeases(context.Background(), " ")
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).MarkChallengeUsed(context.Background(), " ", time.Time{}), storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop()).Create(context.Background(), nil), storage.ErrInvalidInput)
	_, err = NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop()).Get(context.Background(), " ")
	require.ErrorIs(t, err, storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentKeyChallengeRepository(mockDB, "table", zap.NewNop()).MarkUsed(context.Background(), " ", time.Time{}), storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).AuthorizeSessionKey(context.Background(), &models.AgentAccessLease{}, " ", "ed25519", time.Time{}), storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).AuthorizeSessionKey(context.Background(), &models.AgentAccessLease{}, "pub", " ", time.Time{}), storage.ErrInvalidInput)
	require.ErrorIs(t, NewAgentAccessLeaseRepository(mockDB, "table", zap.NewNop()).RecordLeaseUse(context.Background(), &models.AgentAccessLease{}, time.Time{}, time.Time{}, false), storage.ErrInvalidInput)
}

func TestAgentAuthRepositories_NilReceivers(t *testing.T) {
	var leaseRepo *AgentAccessLeaseRepository
	require.ErrorIs(t, leaseRepo.CreateChallenge(context.Background(), &models.AgentAccessLeaseChallenge{}), storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.CreateLease(context.Background(), &models.AgentAccessLease{}), storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.MarkChallengeUsed(context.Background(), "challenge-1", time.Time{}), storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.RevokeLease(context.Background(), &models.AgentAccessLease{}, "owner", "reason", time.Time{}), storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.AuthorizeSessionKey(context.Background(), &models.AgentAccessLease{}, "pub", "ed25519", time.Time{}), storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, leaseRepo.RecordLeaseUse(context.Background(), &models.AgentAccessLease{}, time.Now().UTC(), time.Time{}, false), storage.ErrDatabaseConnectionFailed)

	var keyRepo *AgentKeyChallengeRepository
	require.ErrorIs(t, keyRepo.Create(context.Background(), &models.AgentKeyChallenge{}), storage.ErrDatabaseConnectionFailed)
	_, err := keyRepo.Get(context.Background(), "challenge-1")
	require.ErrorIs(t, err, storage.ErrDatabaseConnectionFailed)
	require.ErrorIs(t, keyRepo.MarkUsed(context.Background(), "challenge-1", time.Time{}), storage.ErrDatabaseConnectionFailed)
}
