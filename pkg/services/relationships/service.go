// Package relationships provides the core Relationships Service for the Lesser project's API alignment.
// This service handles all relationship operations including follows, blocks, mutes, and relationship 
// status management. It emits appropriate events for real-time streaming and queues federation 
// activities for remote users.
package relationships

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides relationship operations
type Service struct {
	relationshipRepo interfaces.RelationshipRepository
	accountRepo      interfaces.AccountRepository
	publisher        streaming.Publisher
	logger           *zap.Logger
	domainName       string
	federation       FederationService
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// NewService creates a new Relationships Service with the required dependencies
func NewService(
	relationshipRepo interfaces.RelationshipRepository,
	accountRepo interfaces.AccountRepository,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		relationshipRepo: relationshipRepo,
		accountRepo:      accountRepo,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domainName:       domainName,
	}
}

// Command structs for operations

// FollowCommand contains all data needed to follow a user
type FollowCommand struct {
	FollowerID   string `json:"follower_id" validate:"required"`
	FollowingID  string `json:"following_id" validate:"required"`
	ShowReblogs  bool   `json:"show_reblogs"`  // Whether to show reblogs from this user
	Notify       bool   `json:"notify"`        // Whether to notify on new posts
	Languages    []string `json:"languages"`   // Filter to specific languages
}

// UnfollowCommand contains all data needed to unfollow a user
type UnfollowCommand struct {
	FollowerID  string `json:"follower_id" validate:"required"`
	FollowingID string `json:"following_id" validate:"required"`
}

// BlockCommand contains all data needed to block a user
type BlockCommand struct {
	BlockerID string `json:"blocker_id" validate:"required"`
	BlockedID string `json:"blocked_id" validate:"required"`
	Reason    string `json:"reason"` // Optional reason for blocking
}

// UnblockCommand contains all data needed to unblock a user
type UnblockCommand struct {
	BlockerID string `json:"blocker_id" validate:"required"`
	BlockedID string `json:"blocked_id" validate:"required"`
}

// MuteCommand contains all data needed to mute a user
type MuteCommand struct {
	MuterID         string         `json:"muter_id" validate:"required"`
	MutedID         string         `json:"muted_id" validate:"required"`
	MuteNotifications bool         `json:"mute_notifications"` // Also mute notifications
	Duration        *time.Duration `json:"duration"`           // Optional duration, nil for indefinite
	Reason          string         `json:"reason"`             // Optional reason for muting
}

// UnmuteCommand contains all data needed to unmute a user
type UnmuteCommand struct {
	MuterID string `json:"muter_id" validate:"required"`
	MutedID string `json:"muted_id" validate:"required"`
}

// GetRelationshipQuery contains parameters for retrieving relationship status
type GetRelationshipQuery struct {
	RequesterID string `json:"requester_id" validate:"required"`
	TargetID    string `json:"target_id" validate:"required"`
}

// GetRelationshipsQuery contains parameters for retrieving multiple relationship statuses
type GetRelationshipsQuery struct {
	RequesterID string   `json:"requester_id" validate:"required"`
	TargetIDs   []string `json:"target_ids" validate:"required,max=40"`
}

// Result structs for operations

// RelationshipData contains comprehensive relationship information
type RelationshipData struct {
	ID                  string    `json:"id"`
	Following           bool      `json:"following"`
	ShowingReblogs      bool      `json:"showing_reblogs"`
	Notifying           bool      `json:"notifying"`
	Languages           []string  `json:"languages"`
	FollowedBy          bool      `json:"followed_by"`
	Blocking            bool      `json:"blocking"`
	BlockedBy           bool      `json:"blocked_by"`
	Muting              bool      `json:"muting"`
	MutingNotifications bool      `json:"muting_notifications"`
	Requested           bool      `json:"requested"`
	RequestedBy         bool      `json:"requested_by"`
	DomainBlocking      bool      `json:"domain_blocking"`
	Endorsed            bool      `json:"endorsed"`
	Note                string    `json:"note"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// RelationshipResult contains a relationship and associated events that were emitted
type RelationshipResult struct {
	Relationship *RelationshipData   `json:"relationship"`
	Events       []*streaming.Event  `json:"events"`
}

// Result contains multiple relationships
type Result struct {
	Relationships []*RelationshipData `json:"relationships"`
	Events        []*streaming.Event  `json:"events"`
}

// FollowResult contains follow-specific data and events
type FollowResult struct {
	Relationship *RelationshipData   `json:"relationship"`
	RequestID    string              `json:"request_id,omitempty"` // If follow requires approval
	IsFollowing  bool                `json:"is_following"`         // Whether follow was immediately accepted
	Events       []*streaming.Event  `json:"events"`
}

// Follow initiates a follow relationship, handling locked accounts and emitting events
func (s *Service) Follow(ctx context.Context, cmd *FollowCommand) (*FollowResult, error) {
	s.logger.Info("processing follow request",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	// Validate command
	if err := s.validateFollowCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Prevent self-follows
	if cmd.FollowerID == cmd.FollowingID {
		return nil, fmt.Errorf("users cannot follow themselves")
	}

	// Check if users exist
	follower, err := s.accountRepo.GetAccount(ctx, cmd.FollowerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get follower account: %w", err)
	}

	following, err := s.accountRepo.GetAccount(ctx, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get following account: %w", err)
	}

	// Check if already following
	isFollowing, err := s.relationshipRepo.IsFollowing(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to check follow status: %w", err)
	}

	if isFollowing {
		// Already following - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.FollowerID,
			TargetID:    cmd.FollowingID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get existing relationship: %w", err)
		}

		return &FollowResult{
			Relationship: relationship,
			IsFollowing:  true,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Check if blocked
	isBlocked, err := s.relationshipRepo.IsBlocked(ctx, cmd.FollowingID, cmd.FollowerID)
	if err != nil {
		return nil, fmt.Errorf("failed to check block status: %w", err)
	}

	if isBlocked {
		return nil, fmt.Errorf("cannot follow user: you are blocked")
	}

	// Create follow request
	activityID := uuid.New().String()
	
	err = s.relationshipRepo.CreateFollowRequest(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to create follow request: %w", err)
	}

	// Determine if follow requires approval
	requiresApproval := following.Actor != nil && following.Actor.ManuallyApprovesFollowers

	var events []*streaming.Event
	var requestID string
	isFollowingNow := false

	if requiresApproval {
		// Follow request pending approval
		requestID = activityID
		events = s.emitFollowRequestedEvents(ctx, follower, following, activityID)
		s.queueFederationFollowRequest(ctx, follower, following, activityID)
	} else {
		// Automatically accept follow
		err = s.relationshipRepo.AcceptFollowRequest(ctx, cmd.FollowerID, cmd.FollowingID)
		if err != nil {
			return nil, fmt.Errorf("failed to accept follow request: %w", err)
		}
		isFollowingNow = true
		events = s.emitFollowAcceptedEvents(ctx, follower, following, activityID)
		s.queueFederationFollow(ctx, follower, following, activityID)
	}

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.FollowerID,
		TargetID:    cmd.FollowingID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("follow request processed",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID),
		zap.Bool("requires_approval", requiresApproval),
		zap.Bool("is_following", isFollowingNow))

	return &FollowResult{
		Relationship: relationship,
		RequestID:    requestID,
		IsFollowing:  isFollowingNow,
		Events:       events,
	}, nil
}

// Unfollow removes a follow relationship and emits events
func (s *Service) Unfollow(ctx context.Context, cmd *UnfollowCommand) (*RelationshipResult, error) {
	s.logger.Info("processing unfollow request",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	// Validate command
	if err := s.validateUnfollowCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if currently following
	isFollowing, err := s.relationshipRepo.IsFollowing(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to check follow status: %w", err)
	}

	if !isFollowing {
		// Not following - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.FollowerID,
			TargetID:    cmd.FollowingID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get current relationship: %w", err)
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	follower, err := s.accountRepo.GetAccount(ctx, cmd.FollowerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get follower account: %w", err)
	}

	following, err := s.accountRepo.GetAccount(ctx, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get following account: %w", err)
	}

	// Remove follow relationship
	err = s.relationshipRepo.Unfollow(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, fmt.Errorf("failed to unfollow: %w", err)
	}

	// Emit events and queue federation
	events := s.emitUnfollowEvents(ctx, follower, following)
	s.queueFederationUndo(ctx, follower, following, "Follow")

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.FollowerID,
		TargetID:    cmd.FollowingID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("unfollow processed",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Block blocks a user, automatically unfollows, and emits events
func (s *Service) Block(ctx context.Context, cmd *BlockCommand) (*RelationshipResult, error) {
	s.logger.Info("processing block request",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	// Validate command
	if err := s.validateBlockCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Prevent self-blocks
	if cmd.BlockerID == cmd.BlockedID {
		return nil, fmt.Errorf("users cannot block themselves")
	}

	// Check if already blocked
	isBlocked, err := s.relationshipRepo.IsBlocked(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to check block status: %w", err)
	}

	if isBlocked {
		// Already blocked - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.BlockerID,
			TargetID:    cmd.BlockedID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get current relationship: %w", err)
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	blocker, err := s.accountRepo.GetAccount(ctx, cmd.BlockerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocker account: %w", err)
	}

	blocked, err := s.accountRepo.GetAccount(ctx, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocked account: %w", err)
	}

	// Automatically unfollow if currently following
	isFollowing, err := s.relationshipRepo.IsFollowing(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		s.logger.Warn("failed to check follow status during block", zap.Error(err))
	} else if isFollowing {
		if err := s.relationshipRepo.Unfollow(ctx, cmd.BlockerID, cmd.BlockedID); err != nil {
			s.logger.Warn("failed to unfollow during block", zap.Error(err))
		}
	}

	// Also unfollow reverse relationship
	isFollowedBy, err := s.relationshipRepo.IsFollowing(ctx, cmd.BlockedID, cmd.BlockerID)
	if err != nil {
		s.logger.Warn("failed to check reverse follow status during block", zap.Error(err))
	} else if isFollowedBy {
		if err := s.relationshipRepo.Unfollow(ctx, cmd.BlockedID, cmd.BlockerID); err != nil {
			s.logger.Warn("failed to unfollow reverse during block", zap.Error(err))
		}
	}

	// Create block
	err = s.relationshipRepo.BlockUser(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to block user: %w", err)
	}

	// Emit events (only to blocker's stream for privacy)
	events := s.emitBlockEvents(ctx, blocker, blocked)
	s.queueFederationBlock(ctx, blocker, blocked)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.BlockerID,
		TargetID:    cmd.BlockedID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("block processed",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Unblock removes a user block and emits events
func (s *Service) Unblock(ctx context.Context, cmd *UnblockCommand) (*RelationshipResult, error) {
	s.logger.Info("processing unblock request",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	// Validate command
	if err := s.validateUnblockCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if currently blocked
	isBlocked, err := s.relationshipRepo.IsBlocked(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to check block status: %w", err)
	}

	if !isBlocked {
		// Not blocked - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.BlockerID,
			TargetID:    cmd.BlockedID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get current relationship: %w", err)
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	blocker, err := s.accountRepo.GetAccount(ctx, cmd.BlockerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocker account: %w", err)
	}

	blocked, err := s.accountRepo.GetAccount(ctx, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocked account: %w", err)
	}

	// Remove block
	err = s.relationshipRepo.UnblockUser(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, fmt.Errorf("failed to unblock user: %w", err)
	}

	// Emit events (only to blocker's stream for privacy)
	events := s.emitUnblockEvents(ctx, blocker, blocked)
	s.queueFederationUndo(ctx, blocker, blocked, "Block")

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.BlockerID,
		TargetID:    cmd.BlockedID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("unblock processed",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Mute mutes a user (hides from timelines) and emits events
func (s *Service) Mute(ctx context.Context, cmd *MuteCommand) (*RelationshipResult, error) {
	s.logger.Info("processing mute request",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	// Validate command
	if err := s.validateMuteCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Prevent self-mutes
	if cmd.MuterID == cmd.MutedID {
		return nil, fmt.Errorf("users cannot mute themselves")
	}

	// Check if already muted
	isMuted, err := s.relationshipRepo.IsMuted(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to check mute status: %w", err)
	}

	if isMuted {
		// Already muted - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.MuterID,
			TargetID:    cmd.MutedID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get current relationship: %w", err)
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	muter, err := s.accountRepo.GetAccount(ctx, cmd.MuterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get muter account: %w", err)
	}

	muted, err := s.accountRepo.GetAccount(ctx, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get muted account: %w", err)
	}

	// Create mute
	err = s.relationshipRepo.MuteUser(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to mute user: %w", err)
	}

	// Emit events (only to muter's stream for privacy)
	events := s.emitMuteEvents(ctx, muter, muted, cmd.Duration)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.MuterID,
		TargetID:    cmd.MutedID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("mute processed",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Unmute removes a user mute and emits events
func (s *Service) Unmute(ctx context.Context, cmd *UnmuteCommand) (*RelationshipResult, error) {
	s.logger.Info("processing unmute request",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	// Validate command
	if err := s.validateUnmuteCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if currently muted
	isMuted, err := s.relationshipRepo.IsMuted(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to check mute status: %w", err)
	}

	if !isMuted {
		// Not muted - return current relationship
		relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
			RequesterID: cmd.MuterID,
			TargetID:    cmd.MutedID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get current relationship: %w", err)
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	muter, err := s.accountRepo.GetAccount(ctx, cmd.MuterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get muter account: %w", err)
	}

	muted, err := s.accountRepo.GetAccount(ctx, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get muted account: %w", err)
	}

	// Remove mute
	err = s.relationshipRepo.UnmuteUser(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, fmt.Errorf("failed to unmute user: %w", err)
	}

	// Emit events (only to muter's stream for privacy)
	events := s.emitUnmuteEvents(ctx, muter, muted)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, &GetRelationshipQuery{
		RequesterID: cmd.MuterID,
		TargetID:    cmd.MutedID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get updated relationship: %w", err)
	}

	s.logger.Info("unmute processed",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// GetRelationship retrieves relationship status between two users
func (s *Service) GetRelationship(ctx context.Context, query *GetRelationshipQuery) (*RelationshipData, error) {
	s.logger.Debug("getting relationship",
		zap.String("requester_id", query.RequesterID),
		zap.String("target_id", query.TargetID))

	// Validate query
	if err := s.validateGetRelationshipQuery(query); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Build relationship data
	relationship, err := s.buildRelationshipData(ctx, query.RequesterID, query.TargetID)
	if err != nil {
		return nil, fmt.Errorf("failed to build relationship data: %w", err)
	}

	return relationship, nil
}

// GetRelationships retrieves relationship statuses for multiple users
func (s *Service) GetRelationships(ctx context.Context, query *GetRelationshipsQuery) (*Result, error) {
	s.logger.Debug("getting relationships",
		zap.String("requester_id", query.RequesterID),
		zap.Int("target_count", len(query.TargetIDs)))

	// Validate query
	if err := s.validateGetRelationshipsQuery(query); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	var relationships []*RelationshipData
	
	// Get relationship data for each target
	for _, targetID := range query.TargetIDs {
		relationship, err := s.buildRelationshipData(ctx, query.RequesterID, targetID)
		if err != nil {
			s.logger.Warn("failed to get relationship data",
				zap.String("requester_id", query.RequesterID),
				zap.String("target_id", targetID),
				zap.Error(err))
			// Create empty relationship data for failed lookups
			relationship = &RelationshipData{
				ID: targetID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}
		relationships = append(relationships, relationship)
	}

	return &Result{
		Relationships: relationships,
		Events:        []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) validateFollowCommand(_ context.Context, cmd *FollowCommand) error {
	if cmd.FollowerID == "" {
		return fmt.Errorf("follower_id is required")
	}
	if cmd.FollowingID == "" {
		return fmt.Errorf("following_id is required")
	}
	return nil
}

func (s *Service) validateUnfollowCommand(_ context.Context, cmd *UnfollowCommand) error {
	if cmd.FollowerID == "" {
		return fmt.Errorf("follower_id is required")
	}
	if cmd.FollowingID == "" {
		return fmt.Errorf("following_id is required")
	}
	return nil
}

func (s *Service) validateBlockCommand(_ context.Context, cmd *BlockCommand) error {
	if cmd.BlockerID == "" {
		return fmt.Errorf("blocker_id is required")
	}
	if cmd.BlockedID == "" {
		return fmt.Errorf("blocked_id is required")
	}
	return nil
}

func (s *Service) validateUnblockCommand(_ context.Context, cmd *UnblockCommand) error {
	if cmd.BlockerID == "" {
		return fmt.Errorf("blocker_id is required")
	}
	if cmd.BlockedID == "" {
		return fmt.Errorf("blocked_id is required")
	}
	return nil
}

func (s *Service) validateMuteCommand(_ context.Context, cmd *MuteCommand) error {
	if cmd.MuterID == "" {
		return fmt.Errorf("muter_id is required")
	}
	if cmd.MutedID == "" {
		return fmt.Errorf("muted_id is required")
	}
	return nil
}

func (s *Service) validateUnmuteCommand(_ context.Context, cmd *UnmuteCommand) error {
	if cmd.MuterID == "" {
		return fmt.Errorf("muter_id is required")
	}
	if cmd.MutedID == "" {
		return fmt.Errorf("muted_id is required")
	}
	return nil
}

func (s *Service) validateGetRelationshipQuery(query *GetRelationshipQuery) error {
	if query.RequesterID == "" {
		return fmt.Errorf("requester_id is required")
	}
	if query.TargetID == "" {
		return fmt.Errorf("target_id is required")
	}
	return nil
}

func (s *Service) validateGetRelationshipsQuery(query *GetRelationshipsQuery) error {
	if query.RequesterID == "" {
		return fmt.Errorf("requester_id is required")
	}
	if len(query.TargetIDs) == 0 {
		return fmt.Errorf("target_ids cannot be empty")
	}
	if len(query.TargetIDs) > 40 {
		return fmt.Errorf("too many target_ids (max 40)")
	}
	return nil
}

func (s *Service) buildRelationshipData(ctx context.Context, requesterID, targetID string) (*RelationshipData, error) {
	now := time.Now()
	
	// Initialize with defaults
	data := &RelationshipData{
		ID:        targetID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Get follow status (requester -> target)
	following, err := s.relationshipRepo.IsFollowing(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check following status", zap.Error(err))
	} else {
		data.Following = following
	}

	// Get follow status (target -> requester)
	followedBy, err := s.relationshipRepo.IsFollowing(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check followed_by status", zap.Error(err))
	} else {
		data.FollowedBy = followedBy
	}

	// Get block status (requester -> target)
	blocking, err := s.relationshipRepo.IsBlocked(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check blocking status", zap.Error(err))
	} else {
		data.Blocking = blocking
	}

	// Get block status (target -> requester)
	blockedBy, err := s.relationshipRepo.IsBlocked(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check blocked_by status", zap.Error(err))
	} else {
		data.BlockedBy = blockedBy
	}

	// Get mute status (requester -> target)
	muting, err := s.relationshipRepo.IsMuted(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check muting status", zap.Error(err))
	} else {
		data.Muting = muting
	}

	// Get follow request status
	followStatus, err := s.relationshipRepo.GetFollowStatus(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check follow request status", zap.Error(err))
	} else {
		data.Requested = (followStatus == models.RelationshipPending)
	}

	// Get reverse follow request status
	reverseFollowStatus, err := s.relationshipRepo.GetFollowStatus(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check reverse follow request status", zap.Error(err))
	} else {
		data.RequestedBy = (reverseFollowStatus == models.RelationshipPending)
	}

	return data, nil
}

// Event emission methods

func (s *Service) emitFollowRequestedEvents(ctx context.Context, follower, following *storage.Account, activityID string) []*streaming.Event {
	var events []*streaming.Event

	// Event to follower's stream
	followerEvent := streaming.NewEvent(streaming.RelationshipFollowRequested).
		ForStream(streaming.UserStreamName(follower.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, follower.User.Username, followerEvent); err != nil {
		s.logger.Error("failed to publish follow request to follower stream", zap.Error(err))
	} else {
		events = append(events, followerEvent)
	}

	// Event to following user's stream (notification)
	followingEvent := streaming.NewEvent(streaming.RelationshipFollowRequested).
		ForStream(streaming.UserStreamName(following.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, following.User.Username, followingEvent); err != nil {
		s.logger.Error("failed to publish follow request to following stream", zap.Error(err))
	} else {
		events = append(events, followingEvent)
	}

	return events
}

func (s *Service) emitFollowAcceptedEvents(ctx context.Context, follower, following *storage.Account, activityID string) []*streaming.Event {
	var events []*streaming.Event

	// Event to follower's stream
	followerEvent := streaming.NewEvent(streaming.RelationshipFollowAccepted).
		ForStream(streaming.UserStreamName(follower.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, follower.User.Username, followerEvent); err != nil {
		s.logger.Error("failed to publish follow accepted to follower stream", zap.Error(err))
	} else {
		events = append(events, followerEvent)
	}

	// Event to following user's stream
	followingEvent := streaming.NewEvent(streaming.RelationshipFollowAccepted).
		ForStream(streaming.UserStreamName(following.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, following.User.Username, followingEvent); err != nil {
		s.logger.Error("failed to publish follow accepted to following stream", zap.Error(err))
	} else {
		events = append(events, followingEvent)
	}

	return events
}

func (s *Service) emitUnfollowEvents(ctx context.Context, follower, following *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Event to follower's stream
	followerEvent := streaming.NewEvent(streaming.RelationshipUnfollowed).
		ForStream(streaming.UserStreamName(follower.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		Build()

	if err := s.publisher.PublishToUser(ctx, follower.User.Username, followerEvent); err != nil {
		s.logger.Error("failed to publish unfollow to follower stream", zap.Error(err))
	} else {
		events = append(events, followerEvent)
	}

	// Event to following user's stream
	followingEvent := streaming.NewEvent(streaming.RelationshipUnfollowed).
		ForStream(streaming.UserStreamName(following.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		Build()

	if err := s.publisher.PublishToUser(ctx, following.User.Username, followingEvent); err != nil {
		s.logger.Error("failed to publish unfollow to following stream", zap.Error(err))
	} else {
		events = append(events, followingEvent)
	}

	return events
}

func (s *Service) emitBlockEvents(ctx context.Context, blocker, blocked *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Event only to blocker's stream for privacy
	blockerEvent := streaming.NewEvent(streaming.RelationshipBlocked).
		ForStream(streaming.UserStreamName(blocker.User.Username)).
		WithData("actor_id", blocker.User.Username).
		WithData("target_id", blocked.User.Username).
		Build()

	if err := s.publisher.PublishToUser(ctx, blocker.User.Username, blockerEvent); err != nil {
		s.logger.Error("failed to publish block to blocker stream", zap.Error(err))
	} else {
		events = append(events, blockerEvent)
	}

	return events
}

func (s *Service) emitUnblockEvents(ctx context.Context, blocker, blocked *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Event only to blocker's stream for privacy
	blockerEvent := streaming.NewEvent(streaming.RelationshipUnblocked).
		ForStream(streaming.UserStreamName(blocker.User.Username)).
		WithData("actor_id", blocker.User.Username).
		WithData("target_id", blocked.User.Username).
		Build()

	if err := s.publisher.PublishToUser(ctx, blocker.User.Username, blockerEvent); err != nil {
		s.logger.Error("failed to publish unblock to blocker stream", zap.Error(err))
	} else {
		events = append(events, blockerEvent)
	}

	return events
}

func (s *Service) emitMuteEvents(ctx context.Context, muter, muted *storage.Account, duration *time.Duration) []*streaming.Event {
	var events []*streaming.Event

	// Event only to muter's stream for privacy
	muterEvent := streaming.NewEvent(streaming.RelationshipMuted).
		ForStream(streaming.UserStreamName(muter.User.Username)).
		WithData("actor_id", muter.User.Username).
		WithData("target_id", muted.User.Username)

	if duration != nil {
		muterEvent.WithData("duration", duration.String())
	}

	event := muterEvent.Build()

	if err := s.publisher.PublishToUser(ctx, muter.User.Username, event); err != nil {
		s.logger.Error("failed to publish mute to muter stream", zap.Error(err))
	} else {
		events = append(events, event)
	}

	return events
}

func (s *Service) emitUnmuteEvents(ctx context.Context, muter, muted *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Event only to muter's stream for privacy
	muterEvent := streaming.NewEvent(streaming.RelationshipUnmuted).
		ForStream(streaming.UserStreamName(muter.User.Username)).
		WithData("actor_id", muter.User.Username).
		WithData("target_id", muted.User.Username).
		Build()

	if err := s.publisher.PublishToUser(ctx, muter.User.Username, muterEvent); err != nil {
		s.logger.Error("failed to publish unmute to muter stream", zap.Error(err))
	} else {
		events = append(events, muterEvent)
	}

	return events
}

// Federation queueing methods

func (s *Service) queueFederationFollowRequest(ctx context.Context, follower, following *storage.Account, activityID string) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping follow request")
		return
	}

	// Only federate to remote users
	if following.Actor == nil || isLocalActor(following.Actor, s.domainName) {
		return
	}

	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Follow",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, activityID),
			Published: &now,
		},
		Actor:  follower.Actor.ID,
		Object: following.Actor.ID,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation follow request",
			zap.String("follower", follower.User.Username),
			zap.String("following", following.User.Username),
			zap.Error(err))
	}
}

func (s *Service) queueFederationFollow(ctx context.Context, follower, following *storage.Account, activityID string) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping follow")
		return
	}

	// Only federate to remote users
	if following.Actor == nil || isLocalActor(following.Actor, s.domainName) {
		return
	}

	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Follow",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, activityID),
			Published: &now,
		},
		Actor:  follower.Actor.ID,
		Object: following.Actor.ID,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation follow",
			zap.String("follower", follower.User.Username),
			zap.String("following", following.User.Username),
			zap.Error(err))
	}
}

func (s *Service) queueFederationBlock(ctx context.Context, blocker, blocked *storage.Account) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping block")
		return
	}

	// Only federate to remote users
	if blocked.Actor == nil || isLocalActor(blocked.Actor, s.domainName) {
		return
	}

	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Block",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, uuid.New().String()),
			Published: &now,
		},
		Actor:  blocker.Actor.ID,
		Object: blocked.Actor.ID,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation block",
			zap.String("blocker", blocker.User.Username),
			zap.String("blocked", blocked.User.Username),
			zap.Error(err))
	}
}

func (s *Service) queueFederationUndo(ctx context.Context, actor, target *storage.Account, activityType string) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping undo")
		return
	}

	// Only federate to remote users
	if target.Actor == nil || isLocalActor(target.Actor, s.domainName) {
		return
	}

	now := time.Now()
	undoID := uuid.New().String()
	
	// Create the original activity being undone
	originalActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: "https://www.w3.org/ns/activitystreams",
			Type:    activityType,
			ID:      fmt.Sprintf("https://%s/activities/%s", s.domainName, uuid.New().String()),
		},
		Actor:  actor.Actor.ID,
		Object: target.Actor.ID,
	}

	// Create the Undo activity
	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Undo",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, undoID),
			Published: &now,
		},
		Actor:  actor.Actor.ID,
		Object: originalActivity,
	}

	if err := s.federation.QueueActivity(ctx, undoActivity); err != nil {
		s.logger.Error("failed to queue federation undo",
			zap.String("actor", actor.User.Username),
			zap.String("target", target.User.Username),
			zap.String("activity_type", activityType),
			zap.Error(err))
	}
}

// isLocalActor checks if an actor is local to this instance
func isLocalActor(actor *activitypub.Actor, domainName string) bool {
	if actor == nil {
		return false
	}
	// Check if the actor ID contains our domain
	return strings.Contains(actor.ID, domainName)
}