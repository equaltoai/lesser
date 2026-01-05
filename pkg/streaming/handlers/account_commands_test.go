package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type stubAccountsService struct {
	updateProfileCalls     int
	updatePreferencesCalls int

	lastProfileCmd     *accounts.UpdateProfileCommand
	lastPreferencesCmd *accounts.UpdatePreferencesCommand

	profileResult     *accounts.AccountResult
	profileErr        error
	preferencesResult *accounts.PreferencesResult
	preferencesErr    error
}

func (s *stubAccountsService) UpdateProfile(_ context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
	s.updateProfileCalls++
	s.lastProfileCmd = cmd
	return s.profileResult, s.profileErr
}

func (s *stubAccountsService) UpdatePreferences(_ context.Context, cmd *accounts.UpdatePreferencesCommand) (*accounts.PreferencesResult, error) {
	s.updatePreferencesCalls++
	s.lastPreferencesCmd = cmd
	return s.preferencesResult, s.preferencesErr
}

func TestAccountCommandHandler_UnsupportedCommand(t *testing.T) {
	t.Parallel()

	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    &stubAccountsService{},
	}

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{ID: "cmd", Type: "nope"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "UNSUPPORTED_COMMAND", resp.Error.Code)
}

func TestNewAccountCommandHandler_GetSupportedCommands(t *testing.T) {
	t.Parallel()

	handler := NewAccountCommandHandler(nil, zaptest.NewLogger(t))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.BaseCommandHandler)

	commands := handler.GetSupportedCommands()
	assert.Contains(t, commands, streaming.CmdUpdateProfile)
	assert.Contains(t, commands, streaming.CmdUpdatePreferences)
}

func TestAccountCommandHandler_UpdateProfile_AuthRequired(t *testing.T) {
	t.Parallel()

	stub := &stubAccountsService{}
	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    stub,
	}

	resp, err := handler.handleUpdateProfile(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdUpdateProfile,
		Payload: map[string]interface{}{"display_name": "Alice"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
	assert.Equal(t, 0, stub.updateProfileCalls)
}

func TestAccountCommandHandler_HandleCommand_Routes(t *testing.T) {
	t.Parallel()

	stub := &stubAccountsService{
		profileResult: &accounts.AccountResult{Account: &storage.Account{User: &storage.User{Username: "alice"}}},
		preferencesResult: &accounts.PreferencesResult{
			Preferences: map[string]interface{}{"lang": "en"},
		},
	}
	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    stub,
	}

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}

	resp, err := handler.HandleCommand(context.Background(), conn, &streaming.Command{ID: "cmd", Type: streaming.CmdUpdateProfile, Payload: map[string]interface{}{}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)

	resp, err = handler.HandleCommand(context.Background(), conn, &streaming.Command{ID: "cmd", Type: streaming.CmdUpdatePreferences, Payload: map[string]interface{}{}})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)

	assert.Equal(t, 1, stub.updateProfileCalls)
	assert.Equal(t, 1, stub.updatePreferencesCalls)
}

func TestAccountCommandHandler_UpdateProfile_ServiceError(t *testing.T) {
	t.Parallel()

	stub := &stubAccountsService{profileErr: errors.New("boom")}
	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    stub,
	}

	resp, err := handler.handleUpdateProfile(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdUpdateProfile,
		Payload: map[string]interface{}{"display_name": "Alice"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "UPDATE_FAILED", resp.Error.Code)
	assert.Equal(t, 1, stub.updateProfileCalls)
}

func TestAccountCommandHandler_UpdateProfile_ConversionError(t *testing.T) {
	t.Parallel()

	stub := &stubAccountsService{
		profileResult: &accounts.AccountResult{
			Account: &storage.Account{
				User: &storage.User{
					Username: "alice",
					Metadata: map[string]interface{}{"bad": make(chan int)},
				},
			},
		},
	}
	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    stub,
	}

	resp, err := handler.handleUpdateProfile(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdUpdateProfile,
		Payload: map[string]interface{}{"display_name": "Alice"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
}

func TestAccountCommandHandler_UpdateProfile_Success(t *testing.T) {
	t.Parallel()

	stub := &stubAccountsService{
		profileResult: &accounts.AccountResult{
			Account: &storage.Account{
				User: &storage.User{
					Username: "alice",
					Metadata: map[string]interface{}{"ok": true},
				},
			},
		},
	}
	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		accountsService:    stub,
	}

	resp, err := handler.handleUpdateProfile(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:   "cmd",
		Type: streaming.CmdUpdateProfile,
		Payload: map[string]interface{}{
			"display_name":  "Alice",
			"bio":           "hi",
			"avatar":        "a.png",
			"header":        "h.png",
			"locked":        true,
			"bot":           true,
			"discoverable":  false,
			"unexpectedKey": "ignored",
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, 1, stub.updateProfileCalls)
	assert.NotNil(t, stub.lastProfileCmd)
	assert.Equal(t, "alice", stub.lastProfileCmd.Username)
	assert.Equal(t, "Alice", stub.lastProfileCmd.DisplayName)
	assert.Equal(t, "hi", stub.lastProfileCmd.Bio)
	assert.Equal(t, "a.png", stub.lastProfileCmd.Avatar)
	assert.Equal(t, "h.png", stub.lastProfileCmd.Header)
	assert.True(t, stub.lastProfileCmd.Locked)
	assert.True(t, stub.lastProfileCmd.Bot)
	assert.False(t, stub.lastProfileCmd.Discoverable)
	assert.NotEmpty(t, resp.Data)
	assert.Contains(t, resp.Data, "user")
}

func TestAccountCommandHandler_UpdatePreferences_SuccessAndConversionError(t *testing.T) {
	t.Parallel()

	handler := &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
	}

	t.Run("success", func(t *testing.T) {
		stub := &stubAccountsService{
			preferencesResult: &accounts.PreferencesResult{
				Preferences: map[string]interface{}{"lang": "en"},
			},
		}
		handler.accountsService = stub

		resp, err := handler.handleUpdatePreferences(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:   "cmd",
			Type: streaming.CmdUpdatePreferences,
			Payload: map[string]interface{}{
				"default_privacy":   "unlisted",
				"default_sensitive": true,
				"default_language":  "en",
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, stub.updatePreferencesCalls)
		assert.NotNil(t, stub.lastPreferencesCmd)
		assert.Equal(t, "alice", stub.lastPreferencesCmd.Username)
		assert.Equal(t, "unlisted", stub.lastPreferencesCmd.DefaultPostingVisibility)
		assert.True(t, stub.lastPreferencesCmd.DefaultMediaSensitive)
		assert.Equal(t, "en", stub.lastPreferencesCmd.Language)
		assert.Equal(t, "alice", stub.lastPreferencesCmd.UpdaterID)
	})

	t.Run("auth_required", func(t *testing.T) {
		stub := &stubAccountsService{}
		handler.accountsService = stub

		resp, err := handler.handleUpdatePreferences(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{
			ID:   "cmd",
			Type: streaming.CmdUpdatePreferences,
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
		assert.Equal(t, 0, stub.updatePreferencesCalls)
	})

	t.Run("service_error", func(t *testing.T) {
		stub := &stubAccountsService{preferencesErr: errors.New("boom")}
		handler.accountsService = stub

		resp, err := handler.handleUpdatePreferences(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:   "cmd",
			Type: streaming.CmdUpdatePreferences,
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "UPDATE_FAILED", resp.Error.Code)
		assert.Equal(t, 1, stub.updatePreferencesCalls)
	})

	t.Run("conversion_error", func(t *testing.T) {
		stub := &stubAccountsService{
			preferencesResult: &accounts.PreferencesResult{
				Preferences: map[string]interface{}{"bad": make(chan int)},
			},
		}
		handler.accountsService = stub

		resp, err := handler.handleUpdatePreferences(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
			ID:      "cmd",
			Type:    streaming.CmdUpdatePreferences,
			Payload: map[string]interface{}{},
		})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
	})
}
