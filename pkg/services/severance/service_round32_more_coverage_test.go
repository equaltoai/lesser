package severance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNewService_NilLoggerInitializesNop(t *testing.T) {
	t.Parallel()

	service := NewService(new(mockSeveranceRepository), nil, nil, nil, nil, "example.com")
	require.NotNil(t, service)
	require.NotNil(t, service.logger)
}

func TestAcknowledgeSeverance_ReturnsOriginalWhenRefreshFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockSeveranceRepository)
	service := NewService(repo, nil, nil, nil, nil, "example.com")

	now := time.Now()
	severance := &models.SeveredRelationship{
		ID:             "sev-1",
		LocalInstance:  "example.com",
		RemoteInstance: "remote.com",
		Status:         models.SeveranceStatusActive,
		DetectedAt:     now,
	}

	repo.On("GetSeveredRelationship", ctx, "sev-1").Return(severance, nil).Once()
	repo.On("UpdateSeveranceStatus", ctx, "sev-1", models.SeveranceStatusAcknowledged).Return(nil).Once()
	repo.On("GetSeveredRelationship", ctx, "sev-1").Return((*models.SeveredRelationship)(nil), errors.New("boom")).Once()

	out, err := service.AcknowledgeSeverance(ctx, "sev-1", "user-1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, models.SeveranceStatusAcknowledged, out.Status)
}
