package notes

import (
	"context"
	stdErrors "errors"
	"fmt"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestServiceRound16_CreateNote_LogsRootCausesForStatusPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	accountRepo := testingmocks.NewMockAccountRepository()
	accountRepo.
		On("GetAccount", ctx, "alice").
		Return(&storage.Account{User: &storage.User{Username: "alice"}}, nil).
		Once()

	noteRepo := testingmocks.NewMockStatusRepositoryInterface()
	noteRepo.
		On("CreateStatus", ctx, mock.AnythingOfType("*models.Status")).
		Return(apperrors.FailedToCreate("item", stdErrors.New("dynamo conditional check failed"))).
		Once()
	objectRepo := testingmocks.NewMockObjectRepository()
	objectRepo.
		On("CreateObject", ctx, mock.Anything).
		Return(nil).
		Once()
	objectRepo.
		On("DeleteObject", ctx, mock.AnythingOfType("string")).
		Return(nil).
		Once()

	service := NewService(
		noteRepo,
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		objectRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		logger,
		"example.com",
	)

	created, err := service.CreateNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "status contract root cause test",
		Visibility: models.VisibilityPublic,
	})
	require.Nil(t, created)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreateStatus)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "failed to persist created status", entries[0].Message)

	fields := entries[0].ContextMap()
	require.NotEmpty(t, fields["status_id"])
	require.Equal(t, fields["status_id"], fields["conversation_id"])
	require.Contains(t, fmt.Sprint(fields["root_causes"]), "dynamo conditional check failed")
	require.Contains(t, fmt.Sprint(fields["error"]), "Failed to create status")
}
