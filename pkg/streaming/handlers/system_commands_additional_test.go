package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type stubListsService struct {
	createCmd *lists.CreateListCommand
	updateCmd *lists.UpdateListCommand
	deleteCmd *lists.DeleteListCommand
	addCmd    *lists.AddToListCommand
	removeCmd *lists.RemoveFromListCommand

	createResult *lists.ListResult
	updateResult *lists.ListResult
	addResult    *lists.MembershipResult
	removeResult *lists.MembershipResult

	createErr error
	updateErr error
	deleteErr error
	addErr    error
	removeErr error
}

func (s *stubListsService) CreateList(_ context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error) {
	s.createCmd = cmd
	return s.createResult, s.createErr
}

func (s *stubListsService) UpdateList(_ context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error) {
	s.updateCmd = cmd
	return s.updateResult, s.updateErr
}

func (s *stubListsService) DeleteList(_ context.Context, cmd *lists.DeleteListCommand) error {
	s.deleteCmd = cmd
	return s.deleteErr
}

func (s *stubListsService) AddToList(_ context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error) {
	s.addCmd = cmd
	return s.addResult, s.addErr
}

func (s *stubListsService) RemoveFromList(_ context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error) {
	s.removeCmd = cmd
	return s.removeResult, s.removeErr
}

type stubNotesTimelineService struct {
	lastQuery *notes.GetTimelineQuery
	result    *notes.Result
	err       error
}

func (s *stubNotesTimelineService) GetTimeline(_ context.Context, query *notes.GetTimelineQuery) (*notes.Result, error) {
	s.lastQuery = query
	return s.result, s.err
}

type stubNotificationsService struct {
	lastMarkCmd *notifications.MarkAsReadCommand
	lastClear   *notifications.ClearCommand
	lastList    *notifications.ListNotificationsQuery

	markResult  *notifications.NotificationResult
	clearResult *notifications.ClearResult
	listResult  *notifications.NotificationListResult

	markErr  error
	clearErr error
	listErr  error
}

func (s *stubNotificationsService) MarkAsRead(_ context.Context, cmd *notifications.MarkAsReadCommand) (*notifications.NotificationResult, error) {
	s.lastMarkCmd = cmd
	return s.markResult, s.markErr
}

func (s *stubNotificationsService) ClearNotifications(_ context.Context, cmd *notifications.ClearCommand) (*notifications.ClearResult, error) {
	s.lastClear = cmd
	return s.clearResult, s.clearErr
}

func (s *stubNotificationsService) ListNotifications(_ context.Context, query *notifications.ListNotificationsQuery) (*notifications.NotificationListResult, error) {
	s.lastList = query
	return s.listResult, s.listErr
}

func newSystemHandler(t *testing.T, notesService notesTimelineAPI, listsService listsAPI, notificationsService notificationsAPI) *SystemCommandHandler {
	t.Helper()

	return &SystemCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		notesService:         notesService,
		listsService:         listsService,
		notificationsService: notificationsService,
	}
}

func TestSystemCommandHandler_ListCommands(t *testing.T) {
	t.Parallel()

	stub := &stubListsService{
		createResult: &lists.ListResult{List: &models.List{ID: "list1", Username: "alice", Title: "Friends"}},
		updateResult: &lists.ListResult{List: &models.List{ID: "list1", Username: "alice", Title: "Besties"}},
		addResult:    &lists.MembershipResult{Success: true},
		removeResult: &lists.MembershipResult{Success: false},
	}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, stub, &stubNotificationsService{})

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	ctx := context.Background()

	resp, err := handler.handleCreateList(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"title": "Friends", "replies_policy": "list"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, stub.createCmd) {
		assert.Equal(t, "alice", stub.createCmd.Username)
		assert.Equal(t, "Friends", stub.createCmd.Title)
		assert.Equal(t, "list", stub.createCmd.RepliesPolicy)
		assert.Equal(t, "alice", stub.createCmd.CreatorID)
	}

	resp, err = handler.handleUpdateList(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "list1", "title": "Besties", "replies_policy": "none"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, stub.updateCmd) {
		assert.Equal(t, "list1", stub.updateCmd.ListID)
		assert.Equal(t, "Besties", stub.updateCmd.Title)
		assert.Equal(t, "none", stub.updateCmd.RepliesPolicy)
		assert.Equal(t, "alice", stub.updateCmd.UpdaterID)
	}

	resp, err = handler.handleDeleteList(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "list1"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, stub.deleteCmd) {
		assert.Equal(t, "list1", stub.deleteCmd.ListID)
		assert.Equal(t, "alice", stub.deleteCmd.DeleterID)
	}
	assert.Equal(t, true, resp.Data["deleted"])

	resp, err = handler.handleAddToList(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "list1", "account_id": "bob"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, stub.addCmd) {
		assert.Equal(t, "list1", stub.addCmd.ListID)
		assert.Equal(t, "bob", stub.addCmd.MemberUsername)
		assert.Equal(t, "alice", stub.addCmd.AdderID)
	}
	assert.Equal(t, true, resp.Data["added"])

	resp, err = handler.handleRemoveFromList(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "list1", "account_id": "bob"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, stub.removeCmd) {
		assert.Equal(t, "list1", stub.removeCmd.ListID)
		assert.Equal(t, "bob", stub.removeCmd.MemberUsername)
		assert.Equal(t, "alice", stub.removeCmd.RemoverID)
	}
	assert.Equal(t, false, resp.Data["removed"])
}

func TestSystemCommandHandler_SubscribeUnsubscribeTimeline(t *testing.T) {
	t.Parallel()

	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, &stubNotificationsService{})
	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice", Metadata: map[string]interface{}{"subscriptions": "not-a-slice"}}

	resp, err := handler.handleSubscribeTimeline(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"timeline": "home"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)

	subs, _ := conn.Metadata["subscriptions"].([]string)
	assert.Equal(t, []string{"timeline:home"}, subs)

	resp, err = handler.handleSubscribeTimeline(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"timeline": "home"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	subs, _ = conn.Metadata["subscriptions"].([]string)
	assert.Equal(t, []string{"timeline:home"}, subs)

	resp, err = handler.handleUnsubscribeTimeline(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"timeline": "home"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	subs, _ = conn.Metadata["subscriptions"].([]string)
	assert.Empty(t, subs)
}

func TestSystemCommandHandler_GetServerInfo(t *testing.T) {
	t.Parallel()

	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, &stubNotificationsService{})

	resp, err := handler.handleGetServerInfo(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{ID: "cmd"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Contains(t, resp.Data, "server_name")
	assert.Contains(t, resp.Data, "timestamp")
}

func TestSystemCommandHandler_GetTimeline_SuccessAndConversionError(t *testing.T) {
	t.Parallel()

	notesStub := &stubNotesTimelineService{
		result: &notes.Result{
			Notes: []*models.Status{{StatusID: "s1"}},
		},
	}
	handler := newSystemHandler(t, notesStub, &stubListsService{}, &stubNotificationsService{})

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	resp, err := handler.handleGetTimeline(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"timeline": "home", "limit": 10, "since_id": "a", "max_id": "b"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, notesStub.lastQuery) {
		assert.Equal(t, "alice", notesStub.lastQuery.UserID)
		assert.Equal(t, "home", notesStub.lastQuery.Timeline)
		assert.Equal(t, 10, notesStub.lastQuery.Limit)
		assert.Equal(t, "a", notesStub.lastQuery.SinceID)
		assert.Equal(t, "b", notesStub.lastQuery.MaxID)
	}

	notesStub.result = &notes.Result{
		Events: []*streaming.Event{
			{Type: "x", Stream: "y", Payload: map[string]interface{}{"bad": make(chan int)}, Timestamp: time.Now()},
		},
	}
	resp, err = handler.handleGetTimeline(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"timeline": "home"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
}

func TestSystemCommandHandler_GetNotifications_SuccessAndConversionError(t *testing.T) {
	t.Parallel()

	notifStub := &stubNotificationsService{
		listResult: &notifications.NotificationListResult{
			Notifications: []*models.Notification{{ID: "n1", UserID: "alice"}},
			Pagination:    &interfaces.PaginatedResult[*models.Notification]{HasMore: false},
		},
	}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, notifStub)

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	resp, err := handler.handleGetNotifications(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{
		"limit":         5,
		"max_id":        "cursor",
		"exclude_types": []interface{}{"follow"},
		"include_types": []interface{}{"mention"},
	}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, notifStub.lastList) {
		assert.Equal(t, "alice", notifStub.lastList.UserID)
		assert.Equal(t, 5, notifStub.lastList.Pagination.Limit)
		assert.Equal(t, "cursor", notifStub.lastList.Pagination.Cursor)
		assert.Equal(t, []string{"mention"}, notifStub.lastList.Types)
		assert.Equal(t, []string{"follow"}, notifStub.lastList.ExcludeTypes)
	}

	notifStub.listResult = &notifications.NotificationListResult{
		Notifications: []*models.Notification{{ID: "n1", UserID: "alice", Data: map[string]interface{}{"bad": make(chan int)}}},
	}
	resp, err = handler.handleGetNotifications(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
}

func TestSystemCommandHandler_NotificationCommands(t *testing.T) {
	t.Parallel()

	notifStub := &stubNotificationsService{
		markResult:  &notifications.NotificationResult{Notification: &models.Notification{ID: "n1", UserID: "alice"}},
		clearResult: &notifications.ClearResult{ClearedCount: 2},
	}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, notifStub)

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	ctx := context.Background()

	resp, err := handler.handleMarkNotificationRead(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "n1"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, notifStub.lastMarkCmd) {
		assert.Equal(t, "n1", notifStub.lastMarkCmd.NotificationID)
		assert.Equal(t, "alice", notifStub.lastMarkCmd.UserID)
	}

	resp, err = handler.handleMarkAllNotificationsRead(ctx, conn, &streaming.Command{ID: "cmd"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, notifStub.lastClear) {
		assert.True(t, notifStub.lastClear.ClearAll)
		assert.Equal(t, "alice", notifStub.lastClear.UserID)
	}
	assert.Equal(t, int64(2), resp.Data["marked_count"])
	assert.Equal(t, true, resp.Data["success"])

	notifStub.clearResult = &notifications.ClearResult{ClearedCount: 0}
	resp, err = handler.handleDismissNotification(ctx, conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "n1"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	if assert.NotNil(t, notifStub.lastClear) {
		assert.Equal(t, []string{"n1"}, notifStub.lastClear.NotificationIDs)
		assert.Equal(t, "alice", notifStub.lastClear.UserID)
	}
	assert.Equal(t, false, resp.Data["dismissed"])
}

func TestSystemCommandHandler_HandleCommand_Unsupported(t *testing.T) {
	t.Parallel()

	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, &stubNotificationsService{})
	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{ID: "cmd", Type: "nope"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "UNSUPPORTED_COMMAND", resp.Error.Code)
}

func TestSystemCommandHandler_HandleListMembership_ServiceError(t *testing.T) {
	t.Parallel()

	stub := &stubListsService{
		addErr: errors.New("boom"),
	}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, stub, &stubNotificationsService{})

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	resp, err := handler.handleAddToList(context.Background(), conn, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "list1", "account_id": "bob"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "ADD_TO_LIST_FAILED", resp.Error.Code)
}

func TestSystemCommandHandler_GetSupportedCommands(t *testing.T) {
	t.Parallel()

	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, &stubNotificationsService{})
	commands := handler.GetSupportedCommands()
	assert.Contains(t, commands, streaming.CmdCreateList)
	assert.Contains(t, commands, streaming.CmdGetServerInfo)
}

func TestSystemCommandHandler_HandleCommand_Routes(t *testing.T) {
	t.Parallel()

	listsStub := &stubListsService{
		createResult: &lists.ListResult{List: &models.List{ID: "list1", Username: "alice", Title: "Friends"}},
	}
	notesStub := &stubNotesTimelineService{
		result: &notes.Result{},
	}
	notifStub := &stubNotificationsService{
		clearResult: &notifications.ClearResult{ClearedCount: 1},
	}
	handler := newSystemHandler(t, notesStub, listsStub, notifStub)

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice", Username: "alice"}
	ctx := context.Background()

	resp, err := handler.HandleCommand(ctx, conn, &streaming.Command{ID: "cmd", Type: streaming.CmdGetServerInfo})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{ID: "cmd", Type: streaming.CmdCreateList, Payload: map[string]interface{}{"title": "Friends"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.NotNil(t, listsStub.createCmd)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{ID: "cmd", Type: streaming.CmdSubscribeTimeline, Payload: map[string]interface{}{"timeline": "home"}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)

	resp, err = handler.HandleCommand(ctx, conn, &streaming.Command{ID: "cmd", Type: streaming.CmdMarkAllNotificationsRead})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, true, resp.Data["success"])
}

func TestSystemCommandHandler_MarkAllNotificationsRead_ServiceError(t *testing.T) {
	t.Parallel()

	notifStub := &stubNotificationsService{clearErr: errors.New("boom")}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, notifStub)

	resp, err := handler.handleMarkAllNotificationsRead(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{ID: "cmd"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "MARK_ALL_READ_FAILED", resp.Error.Code)
}

func TestSystemCommandHandler_DismissNotification_DismissedTrue(t *testing.T) {
	t.Parallel()

	notifStub := &stubNotificationsService{clearResult: &notifications.ClearResult{ClearedCount: 1}}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, notifStub)

	resp, err := handler.handleDismissNotification(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Payload: map[string]interface{}{"id": "n1"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, true, resp.Data["dismissed"])
}

func TestSystemCommandHandler_GetTimeline_ServiceError(t *testing.T) {
	t.Parallel()

	notesStub := &stubNotesTimelineService{err: errors.New("boom")}
	handler := newSystemHandler(t, notesStub, &stubListsService{}, &stubNotificationsService{})

	resp, err := handler.handleGetTimeline(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "TIMELINE_FETCH_FAILED", resp.Error.Code)
}

func TestSystemCommandHandler_GetNotifications_ServiceError(t *testing.T) {
	t.Parallel()

	notifStub := &stubNotificationsService{listErr: errors.New("boom")}
	handler := newSystemHandler(t, &stubNotesTimelineService{}, &stubListsService{}, notifStub)

	resp, err := handler.handleGetNotifications(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{ID: "cmd", Payload: map[string]interface{}{}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "NOTIFICATIONS_FETCH_FAILED", resp.Error.Code)
}

func TestSystemCommandHandler_MediaHelpers(t *testing.T) {
	t.Parallel()

	focus, err := parseFocusValue("0.00,0.00")
	assert.NoError(t, err)
	assert.Equal(t, "0.00,0.00", focus)

	_, err = parseFocusValue(123)
	assert.Error(t, err)

	_, err = resolveMediaCategory("unknown-category", "text/plain")
	assert.Error(t, err)

	response := buildWebSocketMediaResponse(&models.Media{
		MediaID:       "m1",
		ContentType:   "image/jpeg",
		FileSize:      12,
		CDNUrl:        "https://cdn.example.com/m1",
		Blurhash:      "LKO2?U%2Tw=w]~RBVZRi};RPxuwH",
		MediaCategory: models.MediaCategoryImage,
		CreatedAt:     time.Now(),
	})
	mediaData := response["media"].(map[string]interface{})
	assert.Equal(t, "m1", mediaData["id"])
	assert.Equal(t, "LKO2?U%2Tw=w]~RBVZRi};RPxuwH", mediaData["blurhash"])
}
