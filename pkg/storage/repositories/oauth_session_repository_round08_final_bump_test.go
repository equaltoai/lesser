package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_OAuthSessionRepository_UpdateValidationError(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
	err := repo.UpdateOAuthSession(ctx, &models.OAuthAuthSession{
		SessionID:   "sid",
		ClientID:    "cid",
		RedirectURI: "https://example.com/cb",
		CSRFToken:   "csrf",
		Username:    "user-1",
		FlowStep:    "initiated",
		CreatedAt:   baseTime,
		// UpdatedAt intentionally zero to trigger validation failure.
		ExpiresAt: baseTime.Add(30 * time.Minute).Unix(),
	})
	require.Error(t, err)
}
