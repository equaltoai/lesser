package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestStatusRepository_PrepareStatusCreate_NormalizesNoteContract(t *testing.T) {
	repo := NewStatusRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	original := notecontract.PublicFixtureNote()
	status := &models.Status{
		StatusID: "status-note-contract",
		Note:     original,
	}

	require.NoError(t, repo.PrepareStatusCreate(status))
	require.NotNil(t, status.Note)
	require.NotSame(t, original, status.Note)
	require.Equal(t, original.ConversationID, status.ConversationID)
	require.Equal(t, original.AttributedTo, status.AuthorID)
	require.Len(t, status.Note.Attachment, 1)
	require.Len(t, status.Note.Tag, 2)
	require.False(t, status.PublishedAt.IsZero())
}

func TestStatusRepository_CountStatusesByAuthorUsesStatusActorIDGSIContract(t *testing.T) {
	ctx := context.Background()
	repo := NewStatusRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	note := notecontract.PublicFixtureNote()
	actorID := note.AttributedTo
	status := &models.Status{
		StatusID: "status-actor-id-count-contract",
		Note:     note,
	}

	require.NoError(t, repo.PrepareStatusCreate(status))
	require.Equal(t, actorID, status.AuthorID)
	require.Equal(t, "AUTHOR#"+actorID, status.GSI1PK)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", status.GSI1PK).Return(mockQuery).Once()
	mockQuery.On("Count").Return(int64(1), nil).Once()

	countRepo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	count, err := countRepo.CountStatusesByAuthor(ctx, actorID)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
