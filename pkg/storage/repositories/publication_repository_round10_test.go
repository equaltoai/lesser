package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_PublicationRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewPublicationRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	pub := &models.Publication{ID: "pub-1", Name: "Publication", Slug: "publication", ActorID: "actor-1"}
	require.NoError(t, repo.CreatePublication(ctx, pub))

	got, err := repo.GetPublication(ctx, "pub-1")
	require.NoError(t, err)
	require.NotNil(t, got)
}
