package conversations

import (
	"context"
	stdErrors "errors"
	"fmt"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestServiceRound45_PrepareTransactionalDirectMessageStatusWrite_LogsRootCauses(t *testing.T) {
	core, observed := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	conversationRepo := &mockConversationRepository{transactionalDirectMessageSendMode: true}
	noteRepo := &mockNoteRepository{}
	status := &models.Status{
		StatusID:       "status-1",
		ConversationID: "conv-1",
	}

	noteRepo.
		On("PrepareStatusCreate", status).
		Return(apperrors.FailedToCreate("status", stdErrors.New("note contract normalization failed"))).
		Once()

	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, logger, "example.com")
	stageFn, err := service.prepareTransactionalDirectMessageStatusWrite(status)
	require.Nil(t, stageFn)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreateDirectMessage)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "failed to prepare direct message status for persistence", entries[0].Message)

	fields := entries[0].ContextMap()
	require.Equal(t, "prepare_status", fields["phase"])
	require.Equal(t, "status-1", fields["status_id"])
	require.Equal(t, "conv-1", fields["conversation_id"])
	require.Contains(t, fmt.Sprint(fields["root_causes"]), "note contract normalization failed")
	require.Contains(t, fmt.Sprint(fields["error"]), "Failed to create direct message")
}

func TestServiceRound45_ApplyDirectMessageSendTransition_LogsStageRootCause(t *testing.T) {
	ctx := context.Background()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	conversationRepo := &mockConversationRepository{}
	status := &models.Status{
		StatusID:       "status-1",
		ConversationID: "conv-1",
		PublishedAt:    time.Now().UTC(),
	}
	stageErr := fmt.Errorf(
		"stage direct message status create %s: %w",
		status.StatusID,
		apperrors.FailedToCreate("status", stdErrors.New("dynamo attribute type mismatch")),
	)

	conversationRepo.
		On("ApplyDirectMessageSend", ctx, mock.AnythingOfType("*models.DirectMessageSendTransition"), mock.Anything).
		Return(stageErr).
		Once()

	service := NewService(conversationRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger, "example.com")

	err := service.applyDirectMessageSendTransition(
		ctx,
		&models.Conversation{ID: "conv-1", Participants: []string{"alice", "bob"}},
		true,
		"alice",
		"bob",
		nil,
		nil,
		status,
		true,
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreateDirectMessage)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "failed to apply direct message send transition", entries[0].Message)

	fields := entries[0].ContextMap()
	require.Equal(t, "apply_transition", fields["phase"])
	require.Equal(t, "status-1", fields["status_id"])
	require.Equal(t, "conv-1", fields["conversation_id"])
	require.Contains(t, fmt.Sprint(fields["root_causes"]), "dynamo attribute type mismatch")
	require.Contains(t, fmt.Sprint(fields["error"]), "stage direct message status create status-1")
}

func TestServiceRound45_FinalizeDirectMessageStatusWrite_LogsRootCauseForNonTransactionalCreate(t *testing.T) {
	ctx := context.Background()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	conversationRepo := &mockConversationRepository{}
	noteRepo := &mockNoteRepository{}
	status := &models.Status{
		StatusID:       "status-2",
		ConversationID: "conv-2",
	}

	noteRepo.
		On("CreateStatus", ctx, status).
		Return(apperrors.FailedToCreate("status", stdErrors.New("dynamo put item failed"))).
		Once()

	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, logger, "example.com")
	err := service.finalizeDirectMessageStatusWrite(ctx, status)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreateDirectMessage)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "failed to persist direct message status", entries[0].Message)

	fields := entries[0].ContextMap()
	require.Equal(t, "create_status", fields["phase"])
	require.Contains(t, fmt.Sprint(fields["root_causes"]), "dynamo put item failed")
	require.Contains(t, fmt.Sprint(fields["error"]), "Failed to create direct message")
}

func TestServiceRound45_FinalizeDirectMessageStatusWrite_LogsRootCauseForTransactionalFinalize(t *testing.T) {
	ctx := context.Background()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	conversationRepo := &mockConversationRepository{transactionalDirectMessageSendMode: true}
	noteRepo := &mockNoteRepository{}
	status := &models.Status{
		StatusID:       "status-3",
		ConversationID: "conv-3",
	}

	noteRepo.
		On("FinalizeCreatedStatus", ctx, status).
		Return(apperrors.FailedToCreate("status", stdErrors.New("supplemental status index write failed"))).
		Once()

	service := NewService(conversationRepo, noteRepo, nil, nil, nil, nil, nil, nil, nil, nil, logger, "example.com")
	err := service.finalizeDirectMessageStatusWrite(ctx, status)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCreateDirectMessage)

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "failed to finalize direct message status persistence", entries[0].Message)

	fields := entries[0].ContextMap()
	require.Equal(t, "finalize_status", fields["phase"])
	require.Contains(t, fmt.Sprint(fields["root_causes"]), "supplemental status index write failed")
	require.Contains(t, fmt.Sprint(fields["error"]), "Failed to create direct message")
}
