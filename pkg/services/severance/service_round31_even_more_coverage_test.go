package severance

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_emitSeveranceEvent_PublishesSuccess(t *testing.T) {
	t.Parallel()

	publisher := new(mockEventPublisher)
	service := NewService(nil, nil, nil, publisher, zap.NewNop(), "example.com")

	publisher.On("PublishEvent", mock.Anything, mock.Anything).Return(nil).Once()
	service.emitSeveranceEvent(context.Background(), "sev-1", "EVENT", "user-1", "remote.com")
	publisher.AssertExpectations(t)
}

func TestService_AttemptReconnection_ReturnsNotFoundWhenSeveranceMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	mockRepo.On("GetSeveredRelationship", ctx, "sev-1").Return(nil, nil).Once()

	_, err := service.AttemptReconnection(ctx, "sev-1", "user-1")
	require.ErrorIs(t, err, ErrSeveranceNotFound)
}

func TestService_DetectSeverance_SetsSeverityMediumAndLow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockRepo := new(mockSeveranceRepository)
	service := NewService(mockRepo, nil, nil, nil, zap.NewNop(), "example.com")

	mockRepo.On("CreateSeveredRelationship", ctx, mock.MatchedBy(func(sev *models.SeveredRelationship) bool {
		return sev != nil && sev.Severity == "medium"
	})).Return(nil).Once()
	_, err := service.DetectSeverance(ctx, "remote.com", models.SeveranceReasonDomainBlock, 101, 0, "details")
	require.NoError(t, err)

	mockRepo.On("CreateSeveredRelationship", ctx, mock.MatchedBy(func(sev *models.SeveredRelationship) bool {
		return sev != nil && sev.Severity == "low"
	})).Return(nil).Once()
	_, err = service.DetectSeverance(ctx, "remote.com", models.SeveranceReasonDomainBlock, 10, 0, "details")
	require.NoError(t, err)
}

func TestService_convertModelToService_ReturnsNilForNilInput(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil, nil, zap.NewNop(), "example.com")
	require.Nil(t, service.convertModelToService(nil))
}
