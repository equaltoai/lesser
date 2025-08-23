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

// genericRelationshipHandler handles common relationship command logic
func (rch *RelationshipCommandHandler) genericRelationshipHandler(
	ctx context.Context,
	conn *streaming.ConnectionInfo,
	cmd *streaming.Command,
	requiredFields []string,
	executor func(ctx context.Context, userID string, targetID string, payload map[string]interface{}) (interface{}, error),
	errorCode string,
	errorMessage string,
) (*streaming.CommandResponse, error) {
	// Check authentication
	if authErr := rch.RequireAuth(conn, cmd.ID); authErr != nil {
		return authErr, nil
	}

	// Validate required fields
	if validationErr := rch.ValidatePayload(cmd.Payload, requiredFields, cmd.ID); validationErr != nil {
		return validationErr, nil
	}

	// Get target ID (usually from "id" field)
	targetID := rch.GetString(cmd.Payload, "id", "")

	// Execute the relationship operation
	result, err := executor(ctx, conn.UserID, targetID, cmd.Payload)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, errorCode, errorMessage, err.Error()), nil
	}

	// Convert result to JSON for response
	data, err := rch.ConvertToJSON(result)
	if err != nil {
		return rch.CreateErrorResponse(cmd.ID, "CONVERSION_ERROR", 
			"Failed to format response", err.Error()), nil
	}

	return rch.CreateSuccessResponse(cmd.ID, data), nil
}

// handleAcceptFollowRequest handles accepting a follow request
func (rch *RelationshipCommandHandler) handleAcceptFollowRequest(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			acceptCmd := &relationships.AcceptFollowRequestCommand{
				RequesterID: userID,
				FollowerID:  targetID,
			}
			result, err := rch.relationshipsService.AcceptFollowRequest(ctx, acceptCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"ACCEPT_REQUEST_FAILED",
		"Failed to accept follow request",
	)
}

// handleRejectFollowRequest handles rejecting a follow request
func (rch *RelationshipCommandHandler) handleRejectFollowRequest(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			rejectCmd := &relationships.RejectFollowRequestCommand{
				RequesterID: userID,
				FollowerID:  targetID,
			}
			result, err := rch.relationshipsService.RejectFollowRequest(ctx, rejectCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"REJECT_REQUEST_FAILED",
		"Failed to reject follow request",
	)
}

// handleRemoveFollower handles removing a follower
func (rch *RelationshipCommandHandler) handleRemoveFollower(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			removeCmd := &accounts.RemoveFollowerCommand{
				Username:   userID,
				FollowerID: targetID,
				RemoverID:  userID,
			}
			result, err := rch.accountsService.RemoveFollower(ctx, removeCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"REMOVE_FOLLOWER_FAILED",
		"Failed to remove follower",
	)
}
// handleFollowUser handles following a user
func (rch *RelationshipCommandHandler) handleFollowUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, payload map[string]interface{}) (interface{}, error) {
			reblogs := rch.GetBool(payload, "reblogs", true)
			notify := rch.GetBool(payload, "notify", false)
			
			followCmd := &relationships.FollowCommand{
				FollowerID:  userID,
				FollowingID: targetID,
				ShowReblogs: reblogs,
				Notify:      notify,
			}
			result, err := rch.relationshipsService.Follow(ctx, followCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"FOLLOW_FAILED",
		"Failed to follow user",
	)
}

// handleUnfollowUser handles unfollowing a user
func (rch *RelationshipCommandHandler) handleUnfollowUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			unfollowCmd := &relationships.UnfollowCommand{
				FollowerID:  userID,
				FollowingID: targetID,
			}
			result, err := rch.relationshipsService.Unfollow(ctx, unfollowCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"UNFOLLOW_FAILED",
		"Failed to unfollow user",
	)
}

// handleBlockUser handles blocking a user
func (rch *RelationshipCommandHandler) handleBlockUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			blockCmd := &relationships.BlockCommand{
				BlockerID: userID,
				BlockedID: targetID,
			}
			result, err := rch.relationshipsService.Block(ctx, blockCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"BLOCK_FAILED",
		"Failed to block user",
	)
}

// handleUnblockUser handles unblocking a user
func (rch *RelationshipCommandHandler) handleUnblockUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			unblockCmd := &relationships.UnblockCommand{
				BlockerID: userID,
				BlockedID: targetID,
			}
			result, err := rch.relationshipsService.Unblock(ctx, unblockCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"UNBLOCK_FAILED",
		"Failed to unblock user",
	)
}

// handleMuteUser handles muting a user
func (rch *RelationshipCommandHandler) handleMuteUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, payload map[string]interface{}) (interface{}, error) {
			durationSeconds := rch.GetInt(payload, "duration", 0)
			
			// Convert duration to time.Duration pointer (nil means indefinite)
			var duration *time.Duration
			if durationSeconds > 0 {
				d := time.Duration(durationSeconds) * time.Second
				duration = &d
			}
			
			muteCmd := &relationships.MuteCommand{
				MuterID:  userID,
				MutedID:  targetID,
				Duration: duration,
			}
			result, err := rch.relationshipsService.Mute(ctx, muteCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"MUTE_FAILED",
		"Failed to mute user",
	)
}

// handleUnmuteUser handles unmuting a user
func (rch *RelationshipCommandHandler) handleUnmuteUser(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error) {
	return rch.genericRelationshipHandler(
		ctx, conn, cmd,
		[]string{"id"},
		func(ctx context.Context, userID, targetID string, _ map[string]interface{}) (interface{}, error) {
			unmuteCmd := &relationships.UnmuteCommand{
				MuterID: userID,
				MutedID: targetID,
			}
			result, err := rch.relationshipsService.Unmute(ctx, unmuteCmd)
			if err != nil {
				return nil, err
			}
			return result.Relationship, nil
		},
		"UNMUTE_FAILED",
		"Failed to unmute user",
	)
}
