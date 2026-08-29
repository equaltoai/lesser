package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_OAuthSessionRepository_GetByStateError(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
	_, err := repo.GetOAuthSessionByState(ctx, "state")
	require.Error(t, err)
}
