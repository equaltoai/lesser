package severance

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_GetSeveredRelationships_DefaultsLimitAndHandlesRepoErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()

	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, logger, "example.com")

	mockRepo.On("ListSeveredRelationships", ctx, "example.com", mock.Anything, 20, "cursor").
		Return(nil, "", errors.New("list failed")).Once()

	_, _, err := service.GetSeveredRelationships(ctx, GetSeveredRelationshipsFilters{}, 0, "cursor")
	require.Error(t, err)
}

func TestService_GetSeveredRelationship_ReturnsNotFoundWhenRepoReturnsNil(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(nil, nil).Once()

	_, err := service.GetSeveredRelationship(ctx, "sev-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSeveranceNotFound)
}

func TestService_GetSeveredRelationship_PropagatesRepositoryErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return((*models.SeveredRelationship)(nil), errors.New("boom")).Once()

	_, err := service.GetSeveredRelationship(ctx, "sev-1")
	require.Error(t, err)
}

func TestService_GetAffectedRelationships_ValidationAndRepoErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	_, _, err := service.GetAffectedRelationships(ctx, "", 20, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid severance ID")

	mockRepo.On("GetAffectedRelationships", ctx, "sev-1", 20, "cursor").Return(nil, "", errors.New("boom")).Once()
	_, _, err = service.GetAffectedRelationships(ctx, "sev-1", 200, "cursor")
	require.Error(t, err)
}

func TestService_AcknowledgeSeverance_ReturnsUpdatedStatusWhenFollowupFetchFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	severance := &models.SeveredRelationship{
		ID:             "sev-1",
		LocalInstance:  "example.com",
		RemoteInstance: "remote.com",
		Status:         models.SeveranceStatusActive,
	}

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(severance, nil).Once()
	mockRepo.On("UpdateSeveranceStatus", ctx, "sev-1", models.SeveranceStatusAcknowledged).Return(nil).Once()
	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return((*models.SeveredRelationship)(nil), errors.New("boom")).Once()

	out, err := service.AcknowledgeSeverance(ctx, "sev-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, models.SeveranceStatusAcknowledged, out.Status)
}

func TestService_AcknowledgeSeverance_ReturnsErrorOnUpdateFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	severance := &models.SeveredRelationship{
		ID:             "sev-1",
		LocalInstance:  "example.com",
		RemoteInstance: "remote.com",
		Status:         models.SeveranceStatusActive,
	}

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(severance, nil).Once()
	mockRepo.On("UpdateSeveranceStatus", ctx, "sev-1", models.SeveranceStatusAcknowledged).Return(errors.New("update failed")).Once()

	_, err := service.AcknowledgeSeverance(ctx, "sev-1", "user-1")
	require.Error(t, err)
}

func TestService_AttemptReconnection_AssumesReachableWithoutFederationAndHandlesUpdateErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	severance := &models.SeveredRelationship{
		ID:                "sev-1",
		LocalInstance:     "example.com",
		RemoteInstance:    "remote.com",
		Reversible:        true,
		AffectedFollowers: 4,
		AffectedFollowing: 2,
	}

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(severance, nil).Once()
	mockRepo.On("CreateReconnectionAttempt", ctx, mock.Anything).Return(nil).Once()
	// Called twice: once for in-progress (warn on error), once for completed (error on failure)
	mockRepo.On("UpdateReconnectionAttempt", ctx, mock.Anything).Return(errors.New("update failed")).Twice()
	mockRepo.On("CreateSeveredRelationship", ctx, mock.Anything).Return(nil).Once()

	out, err := service.AttemptReconnection(ctx, "sev-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.True(t, out.Success)
	require.Equal(t, 3, out.SuccessCount)
	require.Equal(t, 6, out.SuccessCount+out.FailureCount)
}

func TestService_AttemptReconnection_MarksFailedWhenRemoteUnreachable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	mockFed := new(mockFederationService)
	service := NewService(mockRepo, mockFed, nil, nil, zap.NewNop(), "example.com")

	severance := &models.SeveredRelationship{
		ID:                "sev-1",
		LocalInstance:     "example.com",
		RemoteInstance:    "remote.com",
		Reversible:        true,
		AffectedFollowers: 2,
		AffectedFollowing: 1,
	}

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(severance, nil).Once()
	mockRepo.On("CreateReconnectionAttempt", ctx, mock.Anything).Return(nil).Once()
	mockRepo.On("UpdateReconnectionAttempt", ctx, mock.Anything).Return(nil).Maybe()
	mockRepo.On("CreateSeveredRelationship", ctx, mock.Anything).Return(nil).Once()
	mockFed.On("CheckInstanceReachability", ctx, "remote.com").Return(false, nil).Once()

	out, err := service.AttemptReconnection(ctx, "sev-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, out.Success)
	require.Equal(t, 3, out.FailureCount)
	require.NotEmpty(t, out.Errors)
}

func TestService_emitSeveranceEvent_SkipsWithoutPublisher(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil, nil, zap.NewNop(), "example.com")
	service.emitSeveranceEvent(context.Background(), "sev-1", "EVENT", "user-1", "remote.com")
}

func TestService_emitSeveranceEvent_PublishesAndHandlesPublishErrors(t *testing.T) {
	t.Parallel()

	publisher := new(mockEventPublisher)
	service := NewService(nil, nil, nil, publisher, zap.NewNop(), "example.com")

	publisher.On("PublishEvent", mock.Anything, mock.Anything).Return(errors.New("publish failed")).Once()
	service.emitSeveranceEvent(context.Background(), "sev-1", "EVENT", "user-1", "remote.com")
	publisher.AssertExpectations(t)
}

func TestService_DetectSeverance_SetsSeverityAndPersists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	mockRepo.On("CreateSeveredRelationship", ctx, mock.MatchedBy(func(sev *models.SeveredRelationship) bool {
		return sev != nil && sev.Severity == "high" && sev.AutoDetected
	})).Return(nil).Once()

	out, err := service.DetectSeverance(ctx, "remote.com", models.SeveranceReasonDomainBlock, 1001, 0, "details")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "high", out.Severity)
}
