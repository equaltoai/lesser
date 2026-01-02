package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

type stubRelationshipsService struct {
	acceptCmd   *relationships.AcceptFollowRequestCommand
	rejectCmd   *relationships.RejectFollowRequestCommand
	followCmd   *relationships.FollowCommand
	unfollowCmd *relationships.UnfollowCommand
	blockCmd    *relationships.BlockCommand
	unblockCmd  *relationships.UnblockCommand
	muteCmd     *relationships.MuteCommand
	unmuteCmd   *relationships.UnmuteCommand

	errFor map[string]error
}

func (s *stubRelationshipsService) AcceptFollowRequest(_ context.Context, cmd *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
	s.acceptCmd = cmd
	if err := s.errFor[streaming.CmdAcceptFollowRequest]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) RejectFollowRequest(_ context.Context, cmd *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error) {
	s.rejectCmd = cmd
	if err := s.errFor[streaming.CmdRejectFollowRequest]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Follow(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
	s.followCmd = cmd
	if err := s.errFor[streaming.CmdFollowUser]; err != nil {
		return nil, err
	}
	return &relationships.FollowResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Unfollow(_ context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
	s.unfollowCmd = cmd
	if err := s.errFor[streaming.CmdUnfollowUser]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Block(_ context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
	s.blockCmd = cmd
	if err := s.errFor[streaming.CmdBlockUser]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Unblock(_ context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
	s.unblockCmd = cmd
	if err := s.errFor[streaming.CmdUnblockUser]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Mute(_ context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error) {
	s.muteCmd = cmd
	if err := s.errFor[streaming.CmdMuteUser]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

func (s *stubRelationshipsService) Unmute(_ context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error) {
	s.unmuteCmd = cmd
	if err := s.errFor[streaming.CmdUnmuteUser]; err != nil {
		return nil, err
	}
	return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{ID: "rel1"}}, nil
}

type stubAccountsRelationshipService struct {
	removeCmd *accounts.RemoveFollowerCommand
	err       error
}

func (s *stubAccountsRelationshipService) RemoveFollower(_ context.Context, cmd *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
	s.removeCmd = cmd
	if s.err != nil {
		return nil, s.err
	}
	return &accounts.RelationshipResult{Relationship: map[string]any{"ok": true}}, nil
}

func TestRelationshipCommandHandler_UnsupportedCommand(t *testing.T) {
	t.Parallel()

	handler := &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		relationshipsService: &stubRelationshipsService{errFor: map[string]error{}},
		accountsService:      &stubAccountsRelationshipService{},
	}

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{ID: "cmd", Type: "nope"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "UNSUPPORTED_COMMAND", resp.Error.Code)
}

func TestNewRelationshipCommandHandler_GetSupportedCommands(t *testing.T) {
	t.Parallel()

	handler := NewRelationshipCommandHandler(nil, nil, zaptest.NewLogger(t))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.BaseCommandHandler)

	commands := handler.GetSupportedCommands()
	assert.Contains(t, commands, streaming.CmdFollowUser)
	assert.Contains(t, commands, streaming.CmdRemoveFollower)
}

func TestRelationshipCommandHandler_AuthAndValidation(t *testing.T) {
	t.Parallel()

	rels := &stubRelationshipsService{errFor: map[string]error{}}
	accountsSvc := &stubAccountsRelationshipService{}
	handler := &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		relationshipsService: rels,
		accountsService:      accountsSvc,
	}

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdFollowUser,
		Payload: map[string]interface{}{"id": "bob"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "AUTHENTICATION_REQUIRED", resp.Error.Code)
	assert.Nil(t, rels.followCmd)

	resp, err = handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdFollowUser,
		Payload: map[string]interface{}{},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	assert.Nil(t, rels.followCmd)
}

func TestRelationshipCommandHandler_AllCommandsSuccess(t *testing.T) {
	t.Parallel()

	rels := &stubRelationshipsService{errFor: map[string]error{}}
	accountsSvc := &stubAccountsRelationshipService{}
	handler := &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		relationshipsService: rels,
		accountsService:      accountsSvc,
	}

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	ctx := context.Background()

	tests := []struct {
		name    string
		cmdType string
		payload map[string]interface{}
		verify  func()
	}{
		{
			name:    "follow",
			cmdType: streaming.CmdFollowUser,
			payload: map[string]interface{}{"id": "bob", "reblogs": false, "notify": true},
			verify: func() {
				if assert.NotNil(t, rels.followCmd) {
					assert.Equal(t, "alice", rels.followCmd.FollowerID)
					assert.Equal(t, "bob", rels.followCmd.FollowingID)
					assert.False(t, rels.followCmd.ShowReblogs)
					assert.True(t, rels.followCmd.Notify)
				}
			},
		},
		{
			name:    "unfollow",
			cmdType: streaming.CmdUnfollowUser,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.unfollowCmd) {
					assert.Equal(t, "alice", rels.unfollowCmd.FollowerID)
					assert.Equal(t, "bob", rels.unfollowCmd.FollowingID)
				}
			},
		},
		{
			name:    "block",
			cmdType: streaming.CmdBlockUser,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.blockCmd) {
					assert.Equal(t, "alice", rels.blockCmd.BlockerID)
					assert.Equal(t, "bob", rels.blockCmd.BlockedID)
				}
			},
		},
		{
			name:    "unblock",
			cmdType: streaming.CmdUnblockUser,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.unblockCmd) {
					assert.Equal(t, "alice", rels.unblockCmd.BlockerID)
					assert.Equal(t, "bob", rels.unblockCmd.BlockedID)
				}
			},
		},
		{
			name:    "mute_with_duration",
			cmdType: streaming.CmdMuteUser,
			payload: map[string]interface{}{"id": "bob", "duration": 60},
			verify: func() {
				if assert.NotNil(t, rels.muteCmd) {
					assert.Equal(t, "alice", rels.muteCmd.MuterID)
					assert.Equal(t, "bob", rels.muteCmd.MutedID)
					if assert.NotNil(t, rels.muteCmd.Duration) {
						assert.Equal(t, time.Minute, *rels.muteCmd.Duration)
					}
				}
			},
		},
		{
			name:    "mute_without_duration",
			cmdType: streaming.CmdMuteUser,
			payload: map[string]interface{}{"id": "bob", "duration": 0},
			verify: func() {
				if assert.NotNil(t, rels.muteCmd) {
					assert.Nil(t, rels.muteCmd.Duration)
				}
			},
		},
		{
			name:    "unmute",
			cmdType: streaming.CmdUnmuteUser,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.unmuteCmd) {
					assert.Equal(t, "alice", rels.unmuteCmd.MuterID)
					assert.Equal(t, "bob", rels.unmuteCmd.MutedID)
				}
			},
		},
		{
			name:    "accept_follow_request",
			cmdType: streaming.CmdAcceptFollowRequest,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.acceptCmd) {
					assert.Equal(t, "alice", rels.acceptCmd.RequesterID)
					assert.Equal(t, "bob", rels.acceptCmd.FollowerID)
				}
			},
		},
		{
			name:    "reject_follow_request",
			cmdType: streaming.CmdRejectFollowRequest,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, rels.rejectCmd) {
					assert.Equal(t, "alice", rels.rejectCmd.RequesterID)
					assert.Equal(t, "bob", rels.rejectCmd.FollowerID)
				}
			},
		},
		{
			name:    "remove_follower",
			cmdType: streaming.CmdRemoveFollower,
			payload: map[string]interface{}{"id": "bob"},
			verify: func() {
				if assert.NotNil(t, accountsSvc.removeCmd) {
					assert.Equal(t, "alice", accountsSvc.removeCmd.Username)
					assert.Equal(t, "bob", accountsSvc.removeCmd.FollowerID)
					assert.Equal(t, "alice", accountsSvc.removeCmd.RemoverID)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handler.HandleCommand(ctx, conn, &streaming.Command{ID: tc.name, Type: tc.cmdType, Payload: tc.payload})
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.True(t, resp.Success)
			if tc.verify != nil {
				tc.verify()
			}
		})
	}
}

func TestRelationshipCommandHandler_ServiceError(t *testing.T) {
	t.Parallel()

	rels := &stubRelationshipsService{errFor: map[string]error{streaming.CmdBlockUser: errors.New("nope")}}
	handler := &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		relationshipsService: rels,
		accountsService:      &stubAccountsRelationshipService{},
	}

	resp, err := handler.HandleCommand(context.Background(), &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}, &streaming.Command{
		ID:      "cmd",
		Type:    streaming.CmdBlockUser,
		Payload: map[string]interface{}{"id": "bob"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "BLOCK_FAILED", resp.Error.Code)
}

func TestRelationshipCommandHandler_ErrorBranches(t *testing.T) {
	t.Parallel()

	rels := &stubRelationshipsService{errFor: map[string]error{
		streaming.CmdAcceptFollowRequest: errors.New("accept failed"),
		streaming.CmdRejectFollowRequest: errors.New("reject failed"),
		streaming.CmdFollowUser:          errors.New("follow failed"),
		streaming.CmdUnfollowUser:        errors.New("unfollow failed"),
		streaming.CmdMuteUser:            errors.New("mute failed"),
		streaming.CmdUnblockUser:         errors.New("unblock failed"),
		streaming.CmdUnmuteUser:          errors.New("unmute failed"),
	}}
	accountsSvc := &stubAccountsRelationshipService{err: errors.New("remove failed")}

	handler := &RelationshipCommandHandler{
		BaseCommandHandler:   streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
		relationshipsService: rels,
		accountsService:      accountsSvc,
	}

	conn := &streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"}
	ctx := context.Background()

	tests := []struct {
		cmdType       string
		expectedError string
		payload       map[string]interface{}
	}{
		{cmdType: streaming.CmdAcceptFollowRequest, expectedError: "ACCEPT_REQUEST_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdRejectFollowRequest, expectedError: "REJECT_REQUEST_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdFollowUser, expectedError: "FOLLOW_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdUnfollowUser, expectedError: "UNFOLLOW_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdMuteUser, expectedError: "MUTE_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdUnblockUser, expectedError: "UNBLOCK_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdUnmuteUser, expectedError: "UNMUTE_FAILED", payload: map[string]interface{}{"id": "bob"}},
		{cmdType: streaming.CmdRemoveFollower, expectedError: "REMOVE_FOLLOWER_FAILED", payload: map[string]interface{}{"id": "bob"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.cmdType, func(t *testing.T) {
			resp, err := handler.HandleCommand(ctx, conn, &streaming.Command{ID: "cmd", Type: tc.cmdType, Payload: tc.payload})
			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.False(t, resp.Success)
			assert.Equal(t, tc.expectedError, resp.Error.Code)
		})
	}
}

func TestRelationshipCommandHandler_ConversionError(t *testing.T) {
	t.Parallel()

	handler := &RelationshipCommandHandler{
		BaseCommandHandler: streaming.NewBaseCommandHandler(zaptest.NewLogger(t)),
	}

	resp, err := handler.genericRelationshipHandler(
		context.Background(),
		&streaming.ConnectionInfo{IsAuthenticated: true, UserID: "alice"},
		&streaming.Command{ID: "cmd", Payload: map[string]interface{}{"id": "bob"}},
		[]string{"id"},
		func(context.Context, string, string, map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{"bad": make(chan int)}, nil
		},
		"REL_FAILED",
		"Failed relationship op",
	)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONVERSION_ERROR", resp.Error.Code)
}
