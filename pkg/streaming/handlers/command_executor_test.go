package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
)

type stubCommandExecutor struct {
	requiresAuth   bool
	requiredFields []string

	buildFn  func(*streaming.ConnectionInfo, map[string]interface{}) interface{}
	execFn   func(context.Context, interface{}) (interface{}, error)
	formatFn func(interface{}) (map[string]interface{}, error)
}

func (s *stubCommandExecutor) RequiresAuth() bool {
	return s.requiresAuth
}

func (s *stubCommandExecutor) RequiredFields() []string {
	return s.requiredFields
}

func (s *stubCommandExecutor) BuildCommand(conn *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
	if s.buildFn != nil {
		return s.buildFn(conn, payload)
	}
	return payload
}

func (s *stubCommandExecutor) Execute(ctx context.Context, serviceCmd interface{}) (interface{}, error) {
	if s.execFn != nil {
		return s.execFn(ctx, serviceCmd)
	}
	return nil, nil
}

func (s *stubCommandExecutor) FormatResponse(result interface{}) (map[string]interface{}, error) {
	if s.formatFn != nil {
		return s.formatFn(result)
	}
	if data, ok := result.(map[string]interface{}); ok {
		return data, nil
	}
	return map[string]interface{}{"result": result}, nil
}

func TestSimpleStatusExecutor_FormatResponse_Custom(t *testing.T) {
	t.Parallel()

	exec := NewSimpleStatusExecutor(
		true,
		nil,
		func(*streaming.ConnectionInfo, map[string]interface{}) interface{} { return nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		"",
	)

	data, err := exec.FormatResponse(map[string]interface{}{"ok": true})
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"ok": true}, data)

	data, err = exec.FormatResponse("hi")
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"result": "hi"}, data)
}

func TestSimpleStatusExecutor_FormatResponse_ExtractsKey(t *testing.T) {
	t.Parallel()

	exec := NewSimpleStatusExecutor(
		false,
		nil,
		func(*streaming.ConnectionInfo, map[string]interface{}) interface{} { return nil },
		func(context.Context, interface{}) (interface{}, error) { return nil, nil },
		"Status",
	)

	data, err := exec.FormatResponse(map[string]interface{}{
		"Status": map[string]interface{}{"id": "s1"},
	})
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"id": "s1"}, data)

	data, err = exec.FormatResponse(map[string]interface{}{"other": "x"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"result": map[string]interface{}{"other": "x"}}, data)
}

func TestExecuteGenericCommand_AuthRequired(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)
	exec := &stubCommandExecutor{requiresAuth: true}

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
}

func TestExecuteGenericCommand_ValidationError(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)
	exec := &stubCommandExecutor{
		requiresAuth:   false,
		requiredFields: []string{"id"},
	}

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
}

func TestExecuteGenericCommand_ExecuteError(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)

	exec := &stubCommandExecutor{
		requiresAuth:   true,
		requiredFields: []string{"id"},
		buildFn: func(_ *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return payload["id"]
		},
		execFn: func(context.Context, interface{}) (interface{}, error) {
			return nil, errors.New("boom")
		},
	}

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{"id": "x"},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "DOIT_FAILED", resp.Error.Code)
	assert.Equal(t, "Failed to execute doit", resp.Error.Message)
	assert.Equal(t, "boom", resp.Error.Details)
}

func TestExecuteGenericCommand_FormatResponseError(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)
	exec := &stubCommandExecutor{
		execFn: func(context.Context, interface{}) (interface{}, error) {
			return "ok", nil
		},
		formatFn: func(interface{}) (map[string]interface{}, error) {
			return nil, errors.New("bad format")
		},
	}

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
}

func TestExecuteGenericCommand_Success(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)
	exec := &stubCommandExecutor{
		execFn: func(context.Context, interface{}) (interface{}, error) {
			return map[string]interface{}{"id": "x"}, nil
		},
		formatFn: func(interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	}

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, map[string]interface{}{"ok": true}, resp.Data)
}

func TestExecuteGenericCommand_UsesSimpleStatusExecutor(t *testing.T) {
	t.Parallel()

	base := streaming.NewBaseCommandHandler(nil)
	exec := NewSimpleStatusExecutor(
		false,
		[]string{"id"},
		func(_ *streaming.ConnectionInfo, payload map[string]interface{}) interface{} {
			return payload["id"]
		},
		func(context.Context, interface{}) (interface{}, error) {
			return map[string]interface{}{"id": "x"}, nil
		},
		"",
	)

	resp, err := ExecuteGenericCommand(context.Background(), base, &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "u1"}, &streaming.Command{
		ID:      "cmd",
		Type:    "doit",
		Payload: map[string]interface{}{"id": "x"},
	}, exec, "DOIT_FAILED")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, map[string]interface{}{"id": "x"}, resp.Data)
}
