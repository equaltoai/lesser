package bulk

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	serviceerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_GetOperation(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")

	_, err := svc.GetOperation(context.Background(), &GetOperationQuery{OperationID: "missing", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, serviceerrors.ErrBulkOperationNotFound))

	svc.operations.Store("bad", "not-an-operation")
	_, err = svc.GetOperation(context.Background(), &GetOperationQuery{OperationID: "bad", Username: "alice"})
	require.Error(t, err)
	require.True(t, errors.Is(err, serviceerrors.ErrBulkOperationInvalidData))

	op := &Operation{ID: "op-1", Username: "alice", Type: "bulk_follow", Total: 10, Processed: 1, Succeeded: 1}
	svc.operations.Store(op.ID, op)

	_, err = svc.GetOperation(context.Background(), &GetOperationQuery{OperationID: op.ID, Username: "bob"})
	require.Error(t, err)
	var appErr common.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, 403, appErr.StatusCode)
	require.True(t, errors.Is(appErr.InternalError, serviceerrors.ErrBulkOperationUnauthorizedAccess))

	res, err := svc.GetOperation(context.Background(), &GetOperationQuery{OperationID: op.ID, Username: "alice"})
	require.NoError(t, err)
	require.Equal(t, op.ID, res.Operation.ID)
	require.Equal(t, op.Username, res.Operation.Username)
}

func TestService_createOperationEvent(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")
	op := &Operation{ID: "op-1", Username: "alice"}

	ev := svc.createOperationEvent("bulk_operation.started", op)
	require.Equal(t, "bulk_operation.started", ev.Type)
	require.Equal(t, "user", ev.Stream)
	require.Equal(t, op, ev.Payload["operation"])
	require.False(t, ev.Timestamp.IsZero())
}

func TestService_createProgressEvent_and_shouldEmitProgress(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")

	op := &Operation{ID: "op-1", Type: "bulk_follow", Total: 20, Processed: 10, Succeeded: 9, Failed: 1}
	require.True(t, svc.shouldEmitProgress(op))

	ev := svc.createProgressEvent(op)
	require.Equal(t, "bulk_operation.progress", ev.Type)
	require.Equal(t, "user", ev.Stream)
	require.Equal(t, "op-1", ev.Payload["operation_id"])
	require.Equal(t, "bulk_follow", ev.Payload["type"])
	require.Equal(t, 10, ev.Payload["processed"])
	require.Equal(t, 20, ev.Payload["total"])
	require.Equal(t, 9, ev.Payload["succeeded"])
	require.Equal(t, 1, ev.Payload["failed"])
	require.Equal(t, float64(50), ev.Payload["percent"])

	require.True(t, svc.shouldEmitProgress(&Operation{Total: 20, Processed: 0}))
	require.True(t, svc.shouldEmitProgress(&Operation{Total: 20, Processed: 20}))
	require.False(t, svc.shouldEmitProgress(&Operation{Total: 20, Processed: 1}))
}

func TestService_activityHelpers(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil, nil, zap.NewNop(), "example.com")

	follow := svc.createFollowActivity("alice", "bob")
	require.Equal(t, "Follow", follow.Type)
	require.Equal(t, "https://example.com/users/alice", follow.Actor)
	require.Equal(t, "https://example.com/users/bob", follow.Object)
	require.Equal(t, "https://example.com/users/alice/follows/bob", follow.ID)

	del := svc.createDeleteActivity("alice", "status1")
	require.Equal(t, "Delete", del.Type)
	require.Equal(t, "https://example.com/users/alice", del.Actor)
	require.Equal(t, "https://example.com/users/alice/statuses/status1", del.Object)
	require.Equal(t, "https://example.com/users/alice/delete/status1", del.ID)

	undo := svc.createUndoActivity("alice", "bob", "Follow")
	require.Equal(t, "Undo", undo.Type)
	require.Equal(t, "https://example.com/users/alice", undo.Actor)
	require.Equal(t, "https://example.com/users/alice/undo/follow/bob", undo.ID)

	original, ok := undo.Object.(*activitypub.Activity)
	require.True(t, ok)
	require.Equal(t, "Follow", original.Type)
	require.Equal(t, "https://example.com/users/alice/follows/bob", original.ID)

	block := svc.createBlockActivity("alice", "bob")
	require.Equal(t, "Block", block.Type)
	require.Equal(t, "https://example.com/users/alice", block.Actor)
	require.Equal(t, "https://example.com/users/bob", block.Object)
	require.Equal(t, "https://example.com/users/alice/blocks/bob", block.ID)
}
