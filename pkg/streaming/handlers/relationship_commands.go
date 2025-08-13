// Package handlers provides WebSocket command handlers for different domains
package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// RelationshipCommandHandler handles WebSocket commands related to relationships
type RelationshipCommandHandler struct {
	*streaming.BaseCommandHandler
	relationshipsService *relationships.Service
	accountsService      *accounts.Service
}

// NewRelationshipCommandHandler creates a new relationship command handler
func NewRelationshipCommandHandler(relationshipsService *relationships.Service, accountsService *accounts.Service, logger *zap.Logger) *RelationshipCommandHandler {
	return &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(logger),
		relationshipsService: relationshipsService,
		accountsService:      accountsService,
	}
}

// GetSupportedCommands returns the list of commands this handler supports
func (rch *RelationshipCommandHandler) GetSupportedCommands() []string {
	return []string{
		streaming.CmdFollowUser,
		streaming.CmdUnfollowUser,
		streaming.CmdBlockUser,
		streaming.CmdUnblockUser,
		streaming.CmdMuteUser,
		streaming.CmdUnmuteUser,
		streaming.CmdAcceptFollowRequest,
		streaming.CmdRejectFollowRequest,
		streaming.CmdRemoveFollower,
	}
}

// HandleCommand processes relationship-related WebSocket commands
func (rch *RelationshipCommandHandler) HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	switch cmd.Type {
	case streaming.CmdFollowUser:
		return rch.handleFollowUser(ctx, conn, cmd)
	case streaming.CmdUnfollowUser:
		return rch.handleUnfollowUser(ctx, conn, cmd)
	case streaming.CmdBlockUser:
		return rch.handleBlockUser(ctx, conn, cmd)
	case streaming.CmdUnblockUser:
		return rch.handleUnblockUser(ctx, conn, cmd)
	case streaming.CmdMuteUser:
		return rch.handleMuteUser(ctx, conn, cmd)
	case streaming.CmdUnmuteUser:
		return rch.handleUnmuteUser(ctx, conn, cmd)
	case streaming.CmdAcceptFollowRequest:
		return rch.handleAcceptFollowRequest(ctx, conn, cmd)
	case streaming.CmdRejectFollowRequest:
		return rch.handleRejectFollowRequest(ctx, conn, cmd)
	case streaming.CmdRemoveFollower:
		return rch.handleRemoveFollower(ctx, conn, cmd)
	default:
		return rch.CreateErrorResponse(cmd.ID, "UNSUPPORTED_COMMAND", 
			"Unsupported relationship command", fmt.Sprintf("Command %s not supported by relationship handler", cmd.Type)), nil
	}
}

// handleAcceptFollowRequest handles accepting a follow request
func (rch *RelationshipCommandHandler) handleAcceptFollowRequest(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	requesterID := rch.GetString(cmd.Payload, "id", "")

	// Accept the follow request using the relationships service
	acceptCmd := &relationships.AcceptFollowRequestCommand{
		RequesterID: conn.UserID,  // The current user accepting the request
		FollowerID:  requesterID,  // The user who made the request
	}

	result, err := rch.relationshipsService.AcceptFollowRequest(ctx, acceptCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "ACCEPT_REQUEST_FAILED", 
			"Failed to accept follow request", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleRejectFollowRequest handles rejecting a follow request
func (rch *RelationshipCommandHandler) handleRejectFollowRequest(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	requesterID := rch.GetString(cmd.Payload, "id", "")

	// Reject the follow request using the relationships service
	rejectCmd := &relationships.RejectFollowRequestCommand{
		RequesterID: conn.UserID,  // The current user rejecting the request
		FollowerID:  requesterID,  // The user who made the request
	}

	result, err := rch.relationshipsService.RejectFollowRequest(ctx, rejectCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "REJECT_REQUEST_FAILED", 
			"Failed to reject follow request", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleRemoveFollower handles removing a follower
func (rch *RelationshipCommandHandler) handleRemoveFollower(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	followerID := rch.GetString(cmd.Payload, "id", "")

	// Remove the follower using the accounts service
	removeCmd := &accounts.RemoveFollowerCommand{
		Username:   conn.UserID, // conn.UserID is the username
		FollowerID: followerID,  // The user to be removed as follower
		RemoverID:  conn.UserID,
	}

	result, err := rch.accountsService.RemoveFollower(ctx, removeCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "REMOVE_FOLLOWER_FAILED", 
			"Failed to remove follower", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}
// handleFollowUser handles following a user
func (rch *RelationshipCommandHandler) handleFollowUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")
	reblogs := rch.GetBool(cmd.Payload, "reblogs", true)
	notify := rch.GetBool(cmd.Payload, "notify", false)

	// Follow the user using the relationships service
	followCmd := &relationships.FollowCommand{
		FollowerID:  conn.UserID,
		FollowingID: targetUserID,
		ShowReblogs: reblogs,
		Notify:      notify,
	}

	result, err := rch.relationshipsService.Follow(ctx, followCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "FOLLOW_FAILED", 
			"Failed to follow user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnfollowUser handles unfollowing a user
func (rch *RelationshipCommandHandler) handleUnfollowUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")

	// Unfollow the user using the relationships service
	unfollowCmd := &relationships.UnfollowCommand{
		FollowerID:  conn.UserID,
		FollowingID: targetUserID,
	}

	result, err := rch.relationshipsService.Unfollow(ctx, unfollowCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "UNFOLLOW_FAILED", 
			"Failed to unfollow user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleBlockUser handles blocking a user
func (rch *RelationshipCommandHandler) handleBlockUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")

	// Block the user using the relationships service
	blockCmd := &relationships.BlockCommand{
		BlockerID: conn.UserID,
		BlockedID: targetUserID,
	}

	result, err := rch.relationshipsService.Block(ctx, blockCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "BLOCK_FAILED", 
			"Failed to block user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnblockUser handles unblocking a user
func (rch *RelationshipCommandHandler) handleUnblockUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")

	// Unblock the user using the relationships service
	unblockCmd := &relationships.UnblockCommand{
		BlockerID: conn.UserID,
		BlockedID: targetUserID,
	}

	result, err := rch.relationshipsService.Unblock(ctx, unblockCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "UNBLOCK_FAILED", 
			"Failed to unblock user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleMuteUser handles muting a user
func (rch *RelationshipCommandHandler) handleMuteUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")
	durationSeconds := rch.GetInt(cmd.Payload, "duration", 0)
	
	// Convert duration to time.Duration pointer (nil means indefinite)
	var duration *time.Duration
	if durationSeconds > 0 {
		d := time.Duration(durationSeconds) * time.Second
		duration = &d
	}

	// Mute the user using the relationships service
	muteCmd := &relationships.MuteCommand{
		MuterID:  conn.UserID,
		MutedID:  targetUserID,
		Duration: duration,
	}

	result, err := rch.relationshipsService.Mute(ctx, muteCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "MUTE_FAILED", 
			"Failed to mute user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleUnmuteUser handles unmuting a user
func (rch *RelationshipCommandHandler) handleUnmuteUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, []string{"id"}, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	targetUserID := rch.GetString(cmd.Payload, "id", "")

	// Unmute the user using the relationships service
	unmuteCmd := &relationships.UnmuteCommand{
		MuterID: conn.UserID,
		MutedID: targetUserID,
	}

	result, err := rch.relationshipsService.Unmute(ctx, unmuteCmd)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "UNMUTE_FAILED", 
			"Failed to unmute user", err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result.Relationship)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}
