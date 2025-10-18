package severance

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockSeveranceRepository struct {
	mock.Mock
}

// Compile-time check to ensure mockSeveranceRepository implements Repository
var _ Repository = (*mockSeveranceRepository)(nil)

func (m *mockSeveranceRepository) CreateSeveredRelationship(ctx context.Context, severance *models.SeveredRelationship) error {
	args := m.Called(ctx, severance)
	return args.Error(0)
}

func (m *mockSeveranceRepository) GetSeveredRelationship(ctx context.Context, id string) (*models.SeveredRelationship, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*models.SeveredRelationship), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSeveranceRepository) ListSeveredRelationships(ctx context.Context, localInstance string, filters repositories.SeveranceFilters, limit int, cursor string) ([]*models.SeveredRelationship, string, error) {
	args := m.Called(ctx, localInstance, filters, limit, cursor)
	if v := args.Get(0); v != nil {
		return v.([]*models.SeveredRelationship), args.String(1), args.Error(2)
	}
	return nil, args.String(1), args.Error(2)
}

func (m *mockSeveranceRepository) UpdateSeveranceStatus(ctx context.Context, id string, status models.SeveranceStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockSeveranceRepository) CreateAffectedRelationship(ctx context.Context, affected *models.AffectedRelationship) error {
	args := m.Called(ctx, affected)
	return args.Error(0)
}

func (m *mockSeveranceRepository) GetAffectedRelationships(ctx context.Context, severanceID string, limit int, cursor string) ([]*models.AffectedRelationship, string, error) {
	args := m.Called(ctx, severanceID, limit, cursor)
	if v := args.Get(0); v != nil {
		return v.([]*models.AffectedRelationship), args.String(1), args.Error(2)
	}
	return nil, args.String(1), args.Error(2)
}

func (m *mockSeveranceRepository) CreateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *mockSeveranceRepository) UpdateReconnectionAttempt(ctx context.Context, attempt *models.SeveranceReconnectionAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *mockSeveranceRepository) GetReconnectionAttempt(ctx context.Context, severanceID, attemptID string) (*models.SeveranceReconnectionAttempt, error) {
	args := m.Called(ctx, severanceID, attemptID)
	if v := args.Get(0); v != nil {
		return v.(*models.SeveranceReconnectionAttempt), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSeveranceRepository) GetReconnectionAttempts(ctx context.Context, severanceID string) ([]*models.SeveranceReconnectionAttempt, error) {
	args := m.Called(ctx, severanceID)
	if v := args.Get(0); v != nil {
		return v.([]*models.SeveranceReconnectionAttempt), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockFederationService struct {
	mock.Mock
}

func (m *mockFederationService) CheckInstanceReachability(ctx context.Context, instance string) (bool, error) {
	args := m.Called(ctx, instance)
	return args.Bool(0), args.Error(1)
}

type mockNotificationService struct {
	mock.Mock
}

func (m *mockNotificationService) NotifySeverance(ctx context.Context, userID string, severanceID string) error {
	args := m.Called(ctx, userID, severanceID)
	return args.Error(0)
}

type mockEventPublisher struct {
	mock.Mock
}

func (m *mockEventPublisher) PublishEvent(ctx context.Context, event *models.StreamingEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func TestGetSeveredRelationships(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("returns severed relationships successfully", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		now := time.Now()
		severances := []*models.SeveredRelationship{
			{
				ID:                "test_remote_123",
				LocalInstance:     "example.com",
				RemoteInstance:    "remote.com",
				Reason:            models.SeveranceReasonDomainBlock,
				Status:            models.SeveranceStatusActive,
				AffectedFollowers: 5,
				AffectedFollowing: 3,
				DetectedAt:        now,
			},
		}

		mockRepo.On("ListSeveredRelationships", ctx, "example.com", mock.Anything, 20, "").
			Return(severances, "", nil)

		filters := GetSeveredRelationshipsFilters{}
		result, cursor, err := service.GetSeveredRelationships(ctx, filters, 20, "")

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "", cursor)
		assert.Equal(t, "remote.com", result[0].RemoteInstance)
		mockRepo.AssertExpectations(t)
	})

	t.Run("filters by instance", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		mockRepo.On("ListSeveredRelationships", ctx, "example.com", mock.MatchedBy(func(f repositories.SeveranceFilters) bool {
			return f.Instance == "remote.com"
		}), 20, "").
			Return([]*models.SeveredRelationship{}, "", nil)

		filters := GetSeveredRelationshipsFilters{Instance: "remote.com"}
		result, _, err := service.GetSeveredRelationships(ctx, filters, 20, "")

		require.NoError(t, err)
		assert.Len(t, result, 0)
		mockRepo.AssertExpectations(t)
	})
}

func TestGetSeveredRelationship(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("returns single severed relationship", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		now := time.Now()
		severance := &models.SeveredRelationship{
			ID:                "test_remote_123",
			LocalInstance:     "example.com",
			RemoteInstance:    "remote.com",
			Reason:            models.SeveranceReasonDomainBlock,
			Status:            models.SeveranceStatusActive,
			AffectedFollowers: 5,
			DetectedAt:        now,
		}

		mockRepo.On("GetSeveredRelationship", ctx, "test_remote_123").
			Return(severance, nil)

		result, err := service.GetSeveredRelationship(ctx, "test_remote_123")

		require.NoError(t, err)
		assert.Equal(t, "test_remote_123", result.ID)
		assert.Equal(t, "remote.com", result.RemoteInstance)
		mockRepo.AssertExpectations(t)
	})

	t.Run("returns error for invalid ID", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		_, err := service.GetSeveredRelationship(ctx, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid severance ID")
		mockRepo.AssertExpectations(t)
	})
}

func TestAcknowledgeSeverance(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("acknowledges severance successfully", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		now := time.Now()
		severance := &models.SeveredRelationship{
			ID:             "test_remote_123",
			LocalInstance:  "example.com",
			RemoteInstance: "remote.com",
			Status:         models.SeveranceStatusActive,
			DetectedAt:     now,
		}

		mockRepo.On("GetSeveredRelationship", ctx, "test_remote_123").
			Return(severance, nil).Twice()
		mockRepo.On("UpdateSeveranceStatus", ctx, "test_remote_123", models.SeveranceStatusAcknowledged).
			Return(nil)

		result, err := service.AcknowledgeSeverance(ctx, "test_remote_123", "user123")

		require.NoError(t, err)
		assert.Equal(t, "test_remote_123", result.ID)
		mockRepo.AssertExpectations(t)
	})

	t.Run("returns error for non-existent severance", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		mockRepo.On("GetSeveredRelationship", ctx, "nonexistent").
			Return(nil, nil)

		_, err := service.AcknowledgeSeverance(ctx, "nonexistent", "user123")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSeveranceNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestAttemptReconnection(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("attempts reconnection successfully", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		mockFederation := new(mockFederationService)
		service := NewService(mockRepo, mockFederation, nil, nil, logger, "example.com")

		now := time.Now()
		severance := &models.SeveredRelationship{
			ID:                "test_remote_123",
			LocalInstance:     "example.com",
			RemoteInstance:    "remote.com",
			Reversible:        true,
			AffectedFollowers: 10,
			AffectedFollowing: 5,
			DetectedAt:        now,
		}

		mockRepo.On("GetSeveredRelationship", ctx, "test_remote_123").
			Return(severance, nil)
		mockRepo.On("CreateReconnectionAttempt", ctx, mock.Anything).
			Return(nil)
		mockRepo.On("UpdateReconnectionAttempt", ctx, mock.Anything).
			Return(nil)
		mockRepo.On("CreateSeveredRelationship", ctx, mock.Anything).
			Return(nil)
		mockFederation.On("CheckInstanceReachability", ctx, "remote.com").
			Return(true, nil)

		result, err := service.AttemptReconnection(ctx, "test_remote_123", "user123")

		require.NoError(t, err)
		assert.NotEmpty(t, result.AttemptID)
		assert.True(t, result.Success || result.FailureCount > 0)
		mockRepo.AssertExpectations(t)
		mockFederation.AssertExpectations(t)
	})

	t.Run("returns error for non-reversible severance", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		now := time.Now()
		severance := &models.SeveredRelationship{
			ID:             "test_remote_123",
			LocalInstance:  "example.com",
			RemoteInstance: "remote.com",
			Reversible:     false,
			DetectedAt:     now,
		}

		mockRepo.On("GetSeveredRelationship", ctx, "test_remote_123").
			Return(severance, nil)

		_, err := service.AttemptReconnection(ctx, "test_remote_123", "user123")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not reversible")
		mockRepo.AssertExpectations(t)
	})
}

func TestGetAffectedRelationships(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("returns affected relationships successfully", func(t *testing.T) {
		mockRepo := new(mockSeveranceRepository)
		service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

		now := time.Now()
		affected := []*models.AffectedRelationship{
			{
				SeveranceID:      "test_remote_123",
				ActorID:          "actor1",
				ActorHandle:      "@user1@remote.com",
				ActorDomain:      "remote.com",
				RelationshipType: "follower",
				EstablishedAt:    now,
			},
			{
				SeveranceID:      "test_remote_123",
				ActorID:          "actor2",
				ActorHandle:      "@user2@remote.com",
				ActorDomain:      "remote.com",
				RelationshipType: "following",
				EstablishedAt:    now,
			},
		}

		mockRepo.On("GetAffectedRelationships", ctx, "test_remote_123", 20, "").
			Return(affected, "", nil)

		result, cursor, err := service.GetAffectedRelationships(ctx, "test_remote_123", 20, "")

		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "", cursor)
		assert.Equal(t, "actor1", result[0].ActorID)
		assert.Equal(t, "follower", result[0].RelationshipType)
		mockRepo.AssertExpectations(t)
	})
}
