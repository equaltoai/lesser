// Package handlers provides WebSocket command handlers for different domains
package handlers

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// AccountCommandHandler handles WebSocket commands related to accounts/users
type AccountCommandHandler struct {
	*streaming.BaseCommandHandler
	accountsService *accounts.Service
}

// NewAccountCommandHandler creates a new account command handler
func NewAccountCommandHandler(accountsService *accounts.Service, logger *zap.Logger) *AccountCommandHandler {
	return &AccountCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(logger),
		accountsService:    accountsService,
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (ach *AccountCommandHandler) GetSupportedCommands() []string {
	return []string{
		streaming.CmdUpdateProfile,
		streaming.CmdUpdatePreferences,
	}
}

// HandleCommand processes account-related WebSocket commands
func (ach *AccountCommandHandler) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	switch cmd.Type {
	case streaming.CmdUpdateProfile:
		return ach.handleUpdateProfile(ctx, conn, cmd)
	case streaming.CmdUpdatePreferences:
		return ach.handleUpdatePreferences(ctx, conn, cmd)
	default:
		return ach.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND", 
			"Unsupported account command", fmt.Sprintf("Command %s not supported by account handler", cmd.Type)), nil
	}
}

// handleUpdateProfile handles updating user profile information
func (ach *AccountCommandHandler) handleUpdateProfile(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Get profile update fields
	displayName := ach.GetString(cmd.Payload, "display_name", "")
	bio := ach.GetString(cmd.Payload, "bio", "")
	avatar := ach.GetString(cmd.Payload, "avatar", "")
	header := ach.GetString(cmd.Payload, "header", "")
	locked := ach.GetBool(cmd.Payload, "locked", false)
	bot := ach.GetBool(cmd.Payload, "bot", false)
	discoverable := ach.GetBool(cmd.Payload, "discoverable", true)

	// Update profile using the accounts service
	updateCmd := &accounts.UpdateProfileCommand{
		Username:     conn.UserID, // conn.UserID is the username
		DisplayName:  displayName,
		Bio:          bio,
		Avatar:       avatar,
		Header:       header,
		Locked:       locked,
		Bot:          bot,
		Discoverable: discoverable,
		UpdaterID:    conn.UserID,
	}

	result, err := ach.accountsService.UpdateProfile(ctx, updateCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "UPDATE_FAILED", 
			"Failed to update profile", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := ach.ConvertToJSON(result.Account)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUpdatePreferences handles updating user preferences
func (ach *AccountCommandHandler) handleUpdatePreferences(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := ach.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Get preference fields
	defaultPrivacy := ach.GetString(cmd.Payload, "default_privacy", "public")
	defaultSensitive := ach.GetBool(cmd.Payload, "default_sensitive", false)
	defaultLanguage := ach.GetString(cmd.Payload, "default_language", "en")

	// Update preferences using the accounts service
	updateCmd := &accounts.UpdatePreferencesCommand{
		Username:                 conn.UserID, // conn.UserID is the username
		Language:                 defaultLanguage,
		DefaultPostingVisibility: defaultPrivacy,
		DefaultMediaSensitive:    defaultSensitive,
	}

	result, err := ach.accountsService.UpdatePreferences(ctx, updateCmd)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "UPDATE_FAILED", 
			"Failed to update preferences", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := ach.ConvertToJSON(result.Preferences)
	if err != nil {
		return ach.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return ach.CreateSuccessResponse(cmd.ID, data), nil
}