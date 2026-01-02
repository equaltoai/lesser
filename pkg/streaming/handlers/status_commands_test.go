package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type stubNotesService struct {
	createCalls int
	deleteCalls int

	lastCreate *notes.CreateNoteCommand
	lastDelete *notes.DeleteNoteCommand

	lastLike       *notes.LikeNoteCommand
	lastUnlike     *notes.UnlikeNoteCommand
	lastReblog     *notes.ReblogNoteCommand
	lastUnreblog   *notes.UnreblogNoteCommand
	lastBookmark   *notes.BookmarkNoteCommand
	lastUnbookmark *notes.UnbookmarkNoteCommand
	lastMute       *notes.MuteNoteCommand
	lastUnmute     *notes.UnmuteNoteCommand
	lastPin        *notes.PinNoteCommand
	lastUnpin      *notes.UnpinNoteCommand

	errFor map[string]error
}

func (s *stubNotesService) CreateNote(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
	s.createCalls++
	s.lastCreate = cmd
	if err := s.errFor[streaming.CmdCreateStatus]; err != nil {
		return nil, err
	}
	return &notes.NoteResult{Note: &models.Status{StatusID: "s1"}}, nil
}

func (s *stubNotesService) DeleteNote(_ context.Context, cmd *notes.DeleteNoteCommand) error {
	s.deleteCalls++
	s.lastDelete = cmd
	return s.errFor[streaming.CmdDeleteStatus]
}

func (s *stubNotesService) LikeNote(_ context.Context, cmd *notes.LikeNoteCommand) (*notes.LikeResult, error) {
	s.lastLike = cmd
	if err := s.errFor[streaming.CmdFavoriteStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) UnlikeNote(_ context.Context, cmd *notes.UnlikeNoteCommand) (*notes.LikeResult, error) {
	s.lastUnlike = cmd
	if err := s.errFor[streaming.CmdUnfavoriteStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) ReblogNote(_ context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
	s.lastReblog = cmd
	if err := s.errFor[streaming.CmdReblogStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) UnreblogNote(_ context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
	s.lastUnreblog = cmd
	if err := s.errFor[streaming.CmdUnreblogStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) BookmarkNote(_ context.Context, cmd *notes.BookmarkNoteCommand) (*notes.BookmarkResult, error) {
	s.lastBookmark = cmd
	if err := s.errFor[streaming.CmdBookmarkStatus]; err != nil {
		return nil, err
	}
	return &notes.BookmarkResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) UnbookmarkNote(_ context.Context, cmd *notes.UnbookmarkNoteCommand) (*notes.BookmarkResult, error) {
	s.lastUnbookmark = cmd
	if err := s.errFor[streaming.CmdUnbookmarkStatus]; err != nil {
		return nil, err
	}
	return &notes.BookmarkResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) MuteNote(_ context.Context, cmd *notes.MuteNoteCommand) (*notes.LikeResult, error) {
	s.lastMute = cmd
	if err := s.errFor[streaming.CmdMuteStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) UnmuteNote(_ context.Context, cmd *notes.UnmuteNoteCommand) (*notes.LikeResult, error) {
	s.lastUnmute = cmd
	if err := s.errFor[streaming.CmdUnmuteStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) PinNote(_ context.Context, cmd *notes.PinNoteCommand) (*notes.LikeResult, error) {
	s.lastPin = cmd
	if err := s.errFor[streaming.CmdPinStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func (s *stubNotesService) UnpinNote(_ context.Context, cmd *notes.UnpinNoteCommand) (*notes.LikeResult, error) {
	s.lastUnpin = cmd
	if err := s.errFor[streaming.CmdUnpinStatus]; err != nil {
		return nil, err
	}
	return &notes.LikeResult{Status: &models.Status{StatusID: cmd.StatusID}}, nil
}

func newStatusHandler(t *testing.T, notesService notesAPI) *StatusCommandHandlerV2 {
	t.Helper()

	handler := &StatusCommandHandlerV2{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		notesService:       notesService,
		executors:          make(map[string]CommandExecutor),
	}
	handler.initializeExecutors()
	return handler
}

func TestStatusCommandHandlerV2_UnsupportedCommand(t *testing.T) {
	t.Parallel()

	handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{}})
	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    "nope",
		Payload: map[string]interface{}{},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "UNSUPPORTED_COMMAND", resp.Error.Code)
}

func TestNewStatusCommandHandlerV2_GetSupportedCommands(t *testing.T) {
	t.Parallel()

	handler := NewStatusCommandHandlerV2(nil, zaptest.NewLogger(t))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.BaseCommandHandler)

	commands := handler.GetSupportedCommands()
	assert.Contains(t, commands, streaming.CmdCreateStatus)
	assert.Contains(t, commands, streaming.CmdDeleteStatus)
}

func TestStatusCommandHandlerV2_AuthAndValidation(t *testing.T) {
	t.Parallel()

	handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{}})

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdFavoriteStatus,
		Payload: map[string]interface{}{"id": "s1"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)

	resp, err = handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdCreateStatus,
		Payload: map[string]interface{}{},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestStatusCommandHandlerV2_ServiceError(t *testing.T) {
	t.Parallel()

	handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{
		streaming.CmdFavoriteStatus: errors.New("boom"),
	}})

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdFavoriteStatus,
		Payload: map[string]interface{}{"id": "s1"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "FAVORITE_FAILED", resp.Error.Code)
}

func TestStatusCommandHandlerV2_CreateDefaultsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		stub := &stubNotesService{errFor: map[string]error{}}
		handler := newStatusHandler(t, stub)

		resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Type:    streaming.CmdCreateStatus,
			Payload: map[string]interface{}{"status": "hello"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		if assert.NotNil(t, stub.lastCreate) {
			assert.Equal(t, "public", stub.lastCreate.Visibility)
			assert.False(t, stub.lastCreate.Sensitive)
			assert.Empty(t, stub.lastCreate.MediaIDs)
			assert.Empty(t, stub.lastCreate.Language)
			assert.Empty(t, stub.lastCreate.SpoilerText)
			assert.Empty(t, stub.lastCreate.InReplyToID)
		}
	})

	t.Run("create_error", func(t *testing.T) {
		handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{
			streaming.CmdCreateStatus: errors.New("boom"),
		}})

		resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Type:    streaming.CmdCreateStatus,
			Payload: map[string]interface{}{"status": "hello"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "CREATE_FAILED", resp.Error.Code)
	})
}

func TestStatusCommandHandlerV2_DeleteError(t *testing.T) {
	t.Parallel()

	handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{
		streaming.CmdDeleteStatus: errors.New("boom"),
	}})

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdDeleteStatus,
		Payload: map[string]interface{}{"id": "s1"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "DELETE_FAILED", resp.Error.Code)
}

func TestStatusCommandHandlerV2_SuccessPaths(t *testing.T) {
	t.Parallel()

	stub := &stubNotesService{errFor: map[string]error{}}
	handler := newStatusHandler(t, stub)

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	ctx := context.Background()

	tests := []struct {
		name    string
		cmdType string
		payload map[string]interface{}
	}{
		{
			name:    "create",
			cmdType: streaming.CmdCreateStatus,
			payload: map[string]interface{}{
				"status":       "hello",
				"media_ids":    []interface{}{"m1", "m2"},
				"sensitive":    true,
				"spoiler_text": "cw",
				"visibility":   "unlisted",
				"language":     "en",
			},
		},
		{
			name:    "delete",
			cmdType: streaming.CmdDeleteStatus,
			payload: map[string]interface{}{"id": "s1"},
		},
		{name: "favourite", cmdType: streaming.CmdFavoriteStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "unfavourite", cmdType: streaming.CmdUnfavoriteStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "reblog", cmdType: streaming.CmdReblogStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "unreblog", cmdType: streaming.CmdUnreblogStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "bookmark", cmdType: streaming.CmdBookmarkStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "unbookmark", cmdType: streaming.CmdUnbookmarkStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "mute", cmdType: streaming.CmdMuteStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "unmute", cmdType: streaming.CmdUnmuteStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "pin", cmdType: streaming.CmdPinStatus, payload: map[string]interface{}{"id": "s1"}},
		{name: "unpin", cmdType: streaming.CmdUnpinStatus, payload: map[string]interface{}{"id": "s1"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handler.HandleCommand(ctx, conn, &streaming.Command{ID: tc.name, Type: tc.cmdType, Payload: tc.payload})
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.True(t, resp.Success)
			assert.NotNil(t, resp.Data)
		})
	}

	assert.Equal(t, 1, stub.createCalls)
	assert.Equal(t, 1, stub.deleteCalls)
	assert.NotNil(t, stub.lastCreate)
	assert.Equal(t, "alice", stub.lastCreate.AuthorID)
	assert.Equal(t, "hello", stub.lastCreate.Content)
	assert.Equal(t, []string{"m1", "m2"}, stub.lastCreate.MediaIDs)
	assert.True(t, stub.lastCreate.Sensitive)
	assert.Equal(t, "cw", stub.lastCreate.SpoilerText)
	assert.Equal(t, "unlisted", stub.lastCreate.Visibility)
	assert.Equal(t, "en", stub.lastCreate.Language)
}

func TestStatusCommandHandlerV2_InternalHelpers(t *testing.T) {
	t.Parallel()

	handler := newStatusHandler(t, &stubNotesService{errFor: map[string]error{}})

	assert.Equal(t, "COMMAND_FAILED", getErrorCodeForCommand("unknown"))
	assert.Nil(t, handler.buildCommand(createPayloadHelpers(), &streaming.ConnectionInfo{UserID: "alice"}, map[string]interface{}{"id": "s1"}, "id", "UnknownCommand"))

	_, err := handler.callServiceMethod(context.Background(), nil, func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) { return nil, nil })
	assert.Error(t, err)

	assert.Equal(t, "x", handler.extractResponseFromResult("x"))
}

func TestPayloadHelpers_DefaultBranches(t *testing.T) {
	t.Parallel()

	h := createPayloadHelpers()

	assert.Equal(t, "default", h.getString(map[string]interface{}{}, "missing", "default"))
	assert.Equal(t, true, h.getBool(map[string]interface{}{}, "missing", true))
	assert.Equal(t, []string{}, h.getStringSlice(map[string]interface{}{"media_ids": "not-slice"}, "media_ids"))

	values := h.getStringSlice(map[string]interface{}{"media_ids": []interface{}{1, "m1", true, "m2"}}, "media_ids")
	assert.Equal(t, []string{"m1", "m2"}, values)
}
