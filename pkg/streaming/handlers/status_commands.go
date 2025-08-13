// Package handlers provides WebSocket command handlers for different domains
package handlers

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// StatusCommandHandler handles WebSocket commands related to statuses/notes
type StatusCommandHandler struct {
	*streaming.BaseCommandHandler
	notesService *notes.Service
}

// NewStatusCommandHandler creates a new status command handler
func NewStatusCommandHandler(notesService *notes.Service, logger *zap.Logger) *StatusCommandHandler {
	return &StatusCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(logger),
		notesService:       notesService,
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (sch *StatusCommandHandler) GetSupportedCommands() []string {
	return []string{
		streaming.CmdCreateStatus,
		streaming.CmdDeleteStatus,
		streaming.CmdFavoriteStatus,
		streaming.CmdUnfavoriteStatus,
		streaming.CmdReblogStatus,
		streaming.CmdUnreblogStatus,
		streaming.CmdBookmarkStatus,
		streaming.CmdUnbookmarkStatus,
		streaming.CmdMuteStatus,
		streaming.CmdUnmuteStatus,
		streaming.CmdPinStatus,
		streaming.CmdUnpinStatus,
	}
}

// HandleCommand processes status-related WebSocket commands
func (sch *StatusCommandHandler) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	switch cmd.Type {
	case streaming.CmdCreateStatus:
		return sch.handleCreateStatus(ctx, conn, cmd)
	case streaming.CmdDeleteStatus:
		return sch.handleDeleteStatus(ctx, conn, cmd)
	case streaming.CmdFavoriteStatus:
		return sch.handleFavoriteStatus(ctx, conn, cmd)
	case streaming.CmdUnfavoriteStatus:
		return sch.handleUnfavoriteStatus(ctx, conn, cmd)
	case streaming.CmdReblogStatus:
		return sch.handleReblogStatus(ctx, conn, cmd)
	case streaming.CmdUnreblogStatus:
		return sch.handleUnreblogStatus(ctx, conn, cmd)
	case streaming.CmdBookmarkStatus:
		return sch.handleBookmarkStatus(ctx, conn, cmd)
	case streaming.CmdUnbookmarkStatus:
		return sch.handleUnbookmarkStatus(ctx, conn, cmd)
	case streaming.CmdMuteStatus:
		return sch.handleMuteStatus(ctx, conn, cmd)
	case streaming.CmdUnmuteStatus:
		return sch.handleUnmuteStatus(ctx, conn, cmd)
	case streaming.CmdPinStatus:
		return sch.handlePinStatus(ctx, conn, cmd)
	case streaming.CmdUnpinStatus:
		return sch.handleUnpinStatus(ctx, conn, cmd)
	default:
		return sch.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND", 
			"Unsupported status command", fmt.Sprintf("Command %s not supported by status handler", cmd.Type)), nil
	}
}

// handleCreateStatus handles creating a new status
func (sch *StatusCommandHandler) handleCreateStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"status"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Extract parameters
	statusText := sch.GetString(cmd.Payload, "status", "")
	inReplyToID := sch.GetString(cmd.Payload, "in_reply_to_id", "")
	mediaIDs := sch.GetStringSlice(cmd.Payload, "media_ids")
	sensitive := sch.GetBool(cmd.Payload, "sensitive", false)
	visibility := sch.GetString(cmd.Payload, "visibility", "public")
	language := sch.GetString(cmd.Payload, "language", "")

	// Create the status using the notes service
	createCmd := &notes.CreateNoteCommand{
		AuthorID:    conn.UserID,
		Content:     statusText,
		InReplyToID: inReplyToID,
		MediaIDs:    mediaIDs,
		Sensitive:   sensitive,
		Visibility:  visibility,
		Language:    language,
	}

	result, err := sch.notesService.CreateNote(ctx, createCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CREATE_FAILED", 
			"Failed to create status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Note)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleDeleteStatus handles deleting a status
func (sch *StatusCommandHandler) handleDeleteStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Delete the status using the notes service
	deleteCmd := &notes.DeleteNoteCommand{
		StatusID:  statusID,
		DeleterID: conn.UserID,
	}

	err := sch.notesService.DeleteNote(ctx, deleteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "DELETE_FAILED", 
			"Failed to delete status", err.Error()), nil
	}

	// Return success response
	data := map[string]interface{}{
		"deleted": true,
		"id":      statusID,
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleFavoriteStatus handles favoriting a status
func (sch *StatusCommandHandler) handleFavoriteStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Favorite the status using the notes service
	favoriteCmd := &notes.LikeNoteCommand{
		StatusID: statusID,
		LikerID:  conn.UserID,
	}

	result, err := sch.notesService.LikeNote(ctx, favoriteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "FAVORITE_FAILED", 
			"Failed to favorite status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnfavoriteStatus handles removing favorite from a status
func (sch *StatusCommandHandler) handleUnfavoriteStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Unfavorite the status using the notes service
	unfavoriteCmd := &notes.UnlikeNoteCommand{
		StatusID:  statusID,
		UnlikerID: conn.UserID,
	}

	result, err := sch.notesService.UnlikeNote(ctx, unfavoriteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UNFAVORITE_FAILED", 
			"Failed to unfavorite status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleReblogStatus handles reblogging/boosting a status
func (sch *StatusCommandHandler) handleReblogStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Reblog the status using the notes service
	reblogCmd := &notes.ReblogNoteCommand{
		StatusID:    statusID,
		RebloggerID: conn.UserID,
	}

	result, err := sch.notesService.ReblogNote(ctx, reblogCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "REBLOG_FAILED", 
			"Failed to reblog status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnreblogStatus handles removing reblog/boost from a status
func (sch *StatusCommandHandler) handleUnreblogStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Unreblog the status using the notes service
	unreblogCmd := &notes.UnreblogNoteCommand{
		StatusID:      statusID,
		UnrebloggerID: conn.UserID,
	}

	result, err := sch.notesService.UnreblogNote(ctx, unreblogCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UNREBLOG_FAILED", 
			"Failed to unreblog status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleBookmarkStatus handles bookmarking a status
func (sch *StatusCommandHandler) handleBookmarkStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Bookmark the status using the notes service
	bookmarkCmd := &notes.BookmarkNoteCommand{
		StatusID:     statusID,
		BookmarkerID: conn.UserID,
	}

	result, err := sch.notesService.BookmarkNote(ctx, bookmarkCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "BOOKMARK_FAILED", 
			"Failed to bookmark status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnbookmarkStatus handles removing bookmark from a status
func (sch *StatusCommandHandler) handleUnbookmarkStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Unbookmark the status using the notes service
	unbookmarkCmd := &notes.UnbookmarkNoteCommand{
		StatusID:       statusID,
		UnbookmarkerID: conn.UserID,
	}

	result, err := sch.notesService.UnbookmarkNote(ctx, unbookmarkCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UNBOOKMARK_FAILED", 
			"Failed to unbookmark status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleMuteStatus handles muting a status
func (sch *StatusCommandHandler) handleMuteStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Mute the status using the notes service
	muteCmd := &notes.MuteNoteCommand{
		StatusID: statusID,
		MuterID:  conn.UserID,
	}

	result, err := sch.notesService.MuteNote(ctx, muteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "MUTE_FAILED", 
			"Failed to mute status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnmuteStatus handles unmuting a status
func (sch *StatusCommandHandler) handleUnmuteStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Unmute the status using the notes service
	unmuteCmd := &notes.UnmuteNoteCommand{
		StatusID: statusID,
		MuterID:  conn.UserID,
	}

	result, err := sch.notesService.UnmuteNote(ctx, unmuteCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UNMUTE_FAILED", 
			"Failed to unmute status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handlePinStatus handles pinning a status
func (sch *StatusCommandHandler) handlePinStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Pin the status using the notes service
	pinCmd := &notes.PinNoteCommand{
		StatusID: statusID,
		PinnerID: conn.UserID,
	}

	result, err := sch.notesService.PinNote(ctx, pinCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "PIN_FAILED", 
			"Failed to pin status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnpinStatus handles unpinning a status
func (sch *StatusCommandHandler) handleUnpinStatus(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := sch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := sch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	statusID := sch.GetString(cmd.Payload, "id", "")

	// Unpin the status using the notes service
	unpinCmd := &notes.UnpinNoteCommand{
		StatusID: statusID,
		PinnerID: conn.UserID,
	}

	result, err := sch.notesService.UnpinNote(ctx, unpinCmd)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "UNPIN_FAILED", 
			"Failed to unpin status", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := sch.ConvertToJSON(result.Status)
	if err != nil {
		return sch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return sch.CreateSuccessResponse(cmd.ID, data), nil
}