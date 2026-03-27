package repositories

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
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
