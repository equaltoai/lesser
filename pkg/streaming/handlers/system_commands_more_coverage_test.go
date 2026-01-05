package handlers

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func newSystemHandlerWithMedia(
	t *testing.T,
	notesService notesTimelineAPI,
	listsService listsAPI,
	mediaService mediaUploader,
	notificationsService notificationsAPI,
) *SystemCommandHandler {
	t.Helper()

	return &SystemCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		notesService:         notesService,
		listsService:         listsService,
		mediaService:         mediaService,
		notificationsService: notificationsService,
	}
}

func TestSystemCommandHandler_HandleCommand_RoutesAdditionalCommands(t *testing.T) {
	t.Parallel()

	listsStub := &stubListsService{
		updateResult: &lists.ListResult{List: &models.List{ID: "list1", Username: "alice", Title: "Besties"}},
		addResult:    &lists.MembershipResult{Success: true},
		removeResult: &lists.MembershipResult{Success: true},
	}
	notesStub := &stubNotesTimelineService{
		result: &notes.Result{Notes: []*models.Status{{StatusID: "s1"}}},
	}
	notifStub := &stubNotificationsService{
		markResult:  &notifications.NotificationResult{Notification: &models.Notification{ID: "n1", UserID: "alice"}},
		clearResult: &notifications.ClearResult{ClearedCount: 1},
		listResult: &notifications.NotificationListResult{
			Notifications: []*models.Notification{{ID: "n1", UserID: "alice"}},
			Pagination:    &interfaces.PaginatedResult[*models.Notification]{HasMore: false},
		},
	}
	mediaStub := &stubMediaService{result: &mediasvc.Result{Media: newTestMediaRecord()}}

	handler := newSystemHandlerWithMedia(t, notesStub, listsStub, mediaStub, notifStub)
	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice", Username: "alice"}
	ctx := context.Background()

	resp, err := handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdUpdateList,
		Payload: map[string]interface{}{"id": "list1", "title": "Besties"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdDeleteList,
		Payload: map[string]interface{}{"id": "list1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdAddToList,
		Payload: map[string]interface{}{"id": "list1", "account_id": "bob"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdRemoveFromList,
		Payload: map[string]interface{}{"id": "list1", "account_id": "bob"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdMarkNotificationRead,
		Payload: map[string]interface{}{"id": "n1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdDismissNotification,
		Payload: map[string]interface{}{"id": "n1"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdUnsubscribeTimeline,
		Payload: map[string]interface{}{"timeline": "home"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdGetTimeline,
		Payload: map[string]interface{}{"timeline": "home"},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdGetNotifications,
		Payload: map[string]interface{}{},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{
		ID:   "cmd",
		Type: streaming.CmdUploadMedia,
		Payload: map[string]interface{}{
			"file_data": "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString([]byte("hello world")),
			"file_name": "greeting.txt",
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
}

func TestSystemCommandHandler_HandleUploadMedia_ErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated rejected", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{
			ID:      "cmd",
			Payload: map[string]interface{}{"file_data": base64.StdEncoding.EncodeToString([]byte("hello"))},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
	})

	t.Run("media service missing", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, nil, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Payload: map[string]interface{}{"file_data": base64.StdEncoding.EncodeToString([]byte("hello"))},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_UNAVAILABLE", resp.Error.Code)
	})

	t.Run("missing file_data rejected by validation", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Payload: map[string]interface{}{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	})

	t.Run("empty file_data rejected by parser", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Payload: map[string]interface{}{"file_data": ""},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_INVALID", resp.Error.Code)
	})

	t.Run("empty decoded payload rejected", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Payload: map[string]interface{}{"file_data": "data:application/octet-stream;base64,"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_INVALID", resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "empty")
	})

	t.Run("invalid spoiler text rejected", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID: "cmd",
			Payload: map[string]interface{}{
				"file_data":     base64.StdEncoding.EncodeToString([]byte("hello")),
				"spoiler_text":  strings.Repeat("a", common.MaxStatusSpoiler+1),
				"content_type":  "text/plain",
				"file_name":     "greeting.txt",
				"media_type":    "document",
				"description":   "wave",
				"sensitive":     true,
				"spoilerText":   "ignored",
				"contentType":   "ignored",
				"filename":      "ignored",
				"mediaType":     "ignored",
				"description_2": "ignored",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_INVALID", resp.Error.Code)
	})

	t.Run("invalid media category rejected", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID: "cmd",
			Payload: map[string]interface{}{
				"file_data":   base64.StdEncoding.EncodeToString([]byte("hello")),
				"file_name":   "greeting.txt",
				"mime_type":   "text/plain",
				"media_type":  "nope",
				"description": "wave",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_INVALID", resp.Error.Code)
		assert.Contains(t, resp.Error.Details, "unsupported media category")
	})

	t.Run("invalid focus rejected", func(t *testing.T) {
		handler := NewSystemCommandHandler(nil, nil, &stubMediaService{}, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID: "cmd",
			Payload: map[string]interface{}{
				"file_data":  base64.StdEncoding.EncodeToString([]byte("hello")),
				"file_name":  "greeting.txt",
				"mime_type":  "text/plain",
				"media_type": "document",
				"focus":      map[string]interface{}{"x": 0.0},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_INVALID", resp.Error.Code)
	})

	t.Run("empty result rejected", func(t *testing.T) {
		stub := &stubMediaService{result: nil}
		handler := NewSystemCommandHandler(nil, nil, stub, nil, zaptest.NewLogger(t))
		resp, err := handler.handleUploadMedia(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID: "cmd",
			Payload: map[string]interface{}{
				"file_data":  base64.StdEncoding.EncodeToString([]byte("hello")),
				"file_name":  "greeting.txt",
				"mime_type":  "text/plain",
				"media_type": "document",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPLOAD_MEDIA_FAILED", resp.Error.Code)
	})
}

func TestEnsureFilenameForMime_BlankRejected(t *testing.T) {
	t.Parallel()

	_, err := ensureFilenameForMime("   ", "text/plain")
	assert.Error(t, err)
}
