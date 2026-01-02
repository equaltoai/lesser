package streaming

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCommandHandler struct {
	supported []string
	handle    func(ctx context.Context, conn *ConnectionInfo, cmd *Command) (*CommandResponse, error)
}

func (h *stubCommandHandler) HandleCommand(ctx context.Context, conn *ConnectionInfo, cmd *Command) (*CommandResponse, error) {
	if h.handle == nil {
		return nil, nil
	}
	return h.handle(ctx, conn, cmd)
}

func (h *stubCommandHandler) GetSupportedCommands() []string { return h.supported }

func TestCommandRouter_HandleCommand_Unsupported(t *testing.T) {
	router := NewCommandRouter(nil)

	resp, err := router.HandleCommand(context.Background(), &ConnectionInfo{ConnectionID: "c1"}, &Command{
		ID:   "cmd1",
		Type: "unknown_command",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "UNSUPPORTED_COMMAND", resp.Error.Code)
	assert.Contains(t, resp.Error.Details, "none")
}

func TestCommandRouter_HandleCommand_Unsupported_IncludesSupportedList(t *testing.T) {
	router := NewCommandRouter(nil)
	router.RegisterHandler(&stubCommandHandler{supported: []string{"a", "b"}})

	resp, err := router.HandleCommand(context.Background(), &ConnectionInfo{ConnectionID: "c1"}, &Command{
		ID:   "cmd1",
		Type: "unknown_command",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Contains(t, resp.Error.Details, "a")
	assert.Contains(t, resp.Error.Details, "b")
}

func TestCommandRouter_HandleCommand_Success(t *testing.T) {
	router := NewCommandRouter(nil)

	handler := &stubCommandHandler{
		supported: []string{"hello"},
		handle: func(_ context.Context, _ *ConnectionInfo, cmd *Command) (*CommandResponse, error) {
			return &CommandResponse{
				ID:      cmd.ID,
				Type:    "command_result",
				Success: true,
				Data:    map[string]interface{}{"ok": true},
			}, nil
		},
	}
	router.RegisterHandler(handler)

	resp, err := router.HandleCommand(context.Background(), &ConnectionInfo{ConnectionID: "c1"}, &Command{
		ID:   "cmd1",
		Type: "hello",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, true, resp.Data["ok"])
}

func TestCommandRouter_HandleCommand_HandlerError_NilResponse(t *testing.T) {
	router := NewCommandRouter(nil)

	handler := &stubCommandHandler{
		supported: []string{"boom"},
		handle: func(context.Context, *ConnectionInfo, *Command) (*CommandResponse, error) {
			return nil, errors.New("kaboom")
		},
	}
	router.RegisterHandler(handler)

	resp, err := router.HandleCommand(context.Background(), &ConnectionInfo{ConnectionID: "c1"}, &Command{
		ID:   "cmd1",
		Type: "boom",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "HANDLER_ERROR", resp.Error.Code)
	assert.Contains(t, resp.Error.Details, "kaboom")
}

func TestCommandRouter_HandleCommand_HandlerError_WithResponse(t *testing.T) {
	router := NewCommandRouter(nil)

	handler := &stubCommandHandler{
		supported: []string{"boom"},
		handle: func(context.Context, *ConnectionInfo, *Command) (*CommandResponse, error) {
			return &CommandResponse{
				ID:      "cmd1",
				Type:    "command_error",
				Success: false,
				Error:   &CommandError{Code: "CUSTOM"},
			}, errors.New("kaboom")
		},
	}
	router.RegisterHandler(handler)

	resp, err := router.HandleCommand(context.Background(), &ConnectionInfo{ConnectionID: "c1"}, &Command{
		ID:   "cmd1",
		Type: "boom",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "CUSTOM", resp.Error.Code)
}

func TestCommandRouter_GetSupportedCommands(t *testing.T) {
	router := NewCommandRouter(nil)
	router.RegisterHandler(&stubCommandHandler{supported: []string{"a", "b"}})

	supported := router.GetSupportedCommands()
	assert.Len(t, supported, 2)
	assert.Contains(t, supported, "a")
	assert.Contains(t, supported, "b")
}

func TestBaseCommandHandler_RequireAuth(t *testing.T) {
	handler := NewBaseCommandHandler(nil)

	resp := handler.RequireAuth(&ConnectionInfo{}, "cmd1")
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)

	resp = handler.RequireAuth(&ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, "cmd1")
	assert.Nil(t, resp)
}

func TestBaseCommandHandler_ValidatePayload(t *testing.T) {
	handler := NewBaseCommandHandler(nil)

	resp := handler.ValidatePayload(map[string]interface{}{"a": 1}, []string{"a", "b"}, "cmd1")
	require.NotNil(t, resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)

	resp = handler.ValidatePayload(map[string]interface{}{"a": 1, "b": 2}, []string{"a", "b"}, "cmd1")
	assert.Nil(t, resp)
}

func TestBaseCommandHandler_PayloadHelpers(t *testing.T) {
	handler := NewBaseCommandHandler(nil)

	payload := map[string]interface{}{
		"s": "hello",
		"i": 12,
		"f": float64(7),
		"b": true,
		"a": []interface{}{"x", 1, "y"},
	}

	assert.Equal(t, "hello", handler.GetString(payload, "s", ""))
	assert.Equal(t, "default", handler.GetString(payload, "missing", "default"))

	assert.Equal(t, 12, handler.GetInt(payload, "i", 0))
	assert.Equal(t, 7, handler.GetInt(payload, "f", 0))
	assert.Equal(t, 7, handler.GetInt(payload, "missing", 7))
	assert.Equal(t, 9, handler.GetInt(map[string]interface{}{"i": "not-an-int"}, "i", 9))

	assert.True(t, handler.GetBool(payload, "b", false))
	assert.True(t, handler.GetBool(payload, "missing", true))

	assert.Equal(t, []string{"x", "y"}, handler.GetStringSlice(payload, "a"))
	assert.Nil(t, handler.GetStringSlice(payload, "missing"))
}

func TestBaseCommandHandler_ConvertToJSON(t *testing.T) {
	handler := NewBaseCommandHandler(nil)
	assert.Equal(t, handler.logger, handler.Logger())

	type sample struct {
		Name string `json:"name"`
	}

	data, err := handler.ConvertToJSON(sample{Name: "alice"})
	require.NoError(t, err)
	assert.Equal(t, "alice", data["name"])

	_, err = handler.ConvertToJSON(make(chan int))
	require.Error(t, err)

	_, err = handler.ConvertToJSON([]string{"a"})
	require.Error(t, err)
}
