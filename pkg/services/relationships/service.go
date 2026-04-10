// Package relationships provides the core Relationships Service for the Lesser project's API alignment.
// This service handles all relationship operations including follows, blocks, mutes, and relationship
// status management. It emits appropriate events for real-time streaming and queues federation
// activities for remote users.
package relationships

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/common"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
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

	// Additional repositories for extended functionality
	storage core.RepositoryStorage // Full storage interface for access to all repos

	// Business logic frameworks for semantic consolidation
	businessLogic    *common.BusinessLogicService
	activityPubLogic *common.ActivityPubBusinessLogic
	mastodonLogic    *common.MastodonBusinessLogic
	streamingEmitter streamingEventEmitter
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// streamingEventEmitter adapts streaming.Publisher to common.EventEmitter interface
type streamingEventEmitter struct {
	publisher streaming.Publisher
}

// EmitEvents implements the common.EventEmitter interface
func (e *streamingEventEmitter) EmitEvents(ctx context.Context, events []*common.StreamingEvent) error {
	// Convert common.StreamingEvent to streaming.Event
	streamingEvents := make([]*streaming.Event, len(events))
	for i, event := range events {
		streamingEvents[i] = &streaming.Event{
			Type:      event.Type,
			Stream:    "user", // Default stream, will be overridden
			Timestamp: event.Timestamp,
			Payload:   event.Metadata,
		}
	}

	// Emit using the publisher
	for _, event := range streamingEvents {
		if err := e.publisher.PublishToStream(ctx, event.Stream, event); err != nil {
			return err
		}
	}

	return nil
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

	// Initialize business logic frameworks
	streamingEmitter := streamingEventEmitter{publisher: publisher}
	businessLogic := common.NewBusinessLogicService(logger, &streamingEmitter, domainName)
	federationConfig := &common.FederationConfig{
		Domain:         domainName,
		UserAgent:      "Lesser/1.0",
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	activityPubLogic := common.NewActivityPubBusinessLogic(federationConfig, logger)
	mastodonConfig := common.DefaultMastodonConfig()
	mastodonConfig.Domain = domainName
	mastodonLogic := common.NewMastodonBusinessLogic(mastodonConfig, logger)

	return &Service{
		relationshipRepo: relationshipRepo,
		accountRepo:      accountRepo,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domainName:       domainName,
		businessLogic:    businessLogic,
		activityPubLogic: activityPubLogic,
		mastodonLogic:    mastodonLogic,
		streamingEmitter: streamingEmitter,
	}
}

// NewServiceWithStorage creates a new Relationships Service with full storage access
func NewServiceWithStorage(
	storage core.RepositoryStorage,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize business logic frameworks
	streamingEmitter := streamingEventEmitter{publisher: publisher}
	businessLogic := common.NewBusinessLogicService(logger, &streamingEmitter, domainName)
	federationConfig := &common.FederationConfig{
		Domain:         domainName,
		UserAgent:      "Lesser/1.0",
		MaxRetries:     3,
		RetryDelay:     5 * time.Second,
		RequestTimeout: 30 * time.Second,
	}
	activityPubLogic := common.NewActivityPubBusinessLogic(federationConfig, logger)
	mastodonConfig := common.DefaultMastodonConfig()
	mastodonConfig.Domain = domainName
	mastodonLogic := common.NewMastodonBusinessLogic(mastodonConfig, logger)

	return &Service{
		relationshipRepo: nil, // We'll use storage directly for repository access
		accountRepo:      nil, // We'll use storage directly for repository access
		storage:          storage,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domainName:       domainName,
		businessLogic:    businessLogic,
		activityPubLogic: activityPubLogic,
		mastodonLogic:    mastodonLogic,
		streamingEmitter: streamingEmitter,
	}
}

// Command structs for operations

// FollowCommand contains all data needed to follow a user
type FollowCommand struct {
	FollowerID  string   `json:"follower_id" validate:"required"`
	FollowingID string   `json:"following_id" validate:"required"`
	ShowReblogs bool     `json:"show_reblogs"` // Whether to show reblogs from this user
	Notify      bool     `json:"notify"`       // Whether to notify on new posts
	Languages   []string `json:"languages"`    // Filter to specific languages
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
	MuterID           string         `json:"muter_id" validate:"required"`
	MutedID           string         `json:"muted_id" validate:"required"`
	MuteNotifications bool           `json:"mute_notifications"` // Also mute notifications
	Duration          *time.Duration `json:"duration"`           // Optional duration, nil for indefinite
	Reason            string         `json:"reason"`             // Optional reason for muting
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

// UpdateRelationshipCommand contains data for updating relationship preferences
type UpdateRelationshipCommand struct {
	FollowerID  string    `json:"follower_id" validate:"required"`
	FollowingID string    `json:"following_id" validate:"required"`
	Notify      *bool     `json:"notify,omitempty"`       // Update notification settings
	ShowReblogs *bool     `json:"show_reblogs,omitempty"` // Update reblog visibility
	Languages   *[]string `json:"languages,omitempty"`    // Update language filter
	Note        *string   `json:"note,omitempty"`         // Update private note
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
	Relationship *RelationshipData  `json:"relationship"`
	Events       []*streaming.Event `json:"events"`
}

// Result contains multiple relationships
type Result struct {
	Relationships []*RelationshipData `json:"relationships"`
	Events        []*streaming.Event  `json:"events"`
}

// FollowResult contains follow-specific data and events
type FollowResult struct {
	Relationship *RelationshipData     `json:"relationship"`
	RequestID    string                `json:"request_id,omitempty"` // If follow requires approval
	IsFollowing  bool                  `json:"is_following"`         // Whether follow was immediately accepted
	Events       []*streaming.Event    `json:"events"`
	Activity     *activitypub.Activity `json:"activity,omitempty"`
}

// Follow initiates a follow relationship, handling locked accounts and emitting events
func (s *Service) Follow(ctx context.Context, cmd *FollowCommand) (*FollowResult, error) {
	// Validate command
	if err := s.validateFollowCommand(ctx, cmd); err != nil {
		return nil, err
	}

	// Normalize identifiers so GraphQL IDs and usernames share the same storage keys
	cmd.FollowerID = s.normalizeActorIdentifier(cmd.FollowerID)
	cmd.FollowingID = s.normalizeActorIdentifier(cmd.FollowingID)

	s.logger.Info("processing follow request",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	// Prevent self-follows
	if cmd.FollowerID == cmd.FollowingID {
		return nil, CannotFollowSelf()
	}

	// Get accounts
	follower, following, err := s.getFollowAccounts(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, err
	}
	var followerUsername string
	if follower != nil && follower.User != nil {
		followerUsername = follower.User.Username
	}
	var followingUsername string
	if following != nil && following.User != nil {
		followingUsername = following.User.Username
	}
	s.logger.Info("retrieved follow accounts",
		zap.String("follower_username", followerUsername),
		zap.String("following_username", followingUsername),
		zap.Bool("follower_actor_present", follower != nil && follower.Actor != nil),
		zap.Bool("following_actor_present", following != nil && following.Actor != nil))

	// Check prerequisites and handle if already following
	existingResult, shouldReturn, err := s.checkFollowPrerequisites(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, err
	}
	s.logger.Info("follow prerequisites evaluated",
		zap.Bool("should_return", shouldReturn),
		zap.Bool("existing_relationship", existingResult != nil))
	if shouldReturn {
		s.logger.Info("follow request short-circuited by prerequisites",
			zap.Bool("existing_relationship", existingResult != nil))
		if existingResult != nil {
			existingResult.Activity = s.buildFollowActivity(ctx, follower, following, cmd.FollowerID, cmd.FollowingID, existingResult.RequestID, existingResult.Relationship)
		}
		return existingResult, nil
	}

	// Persist the outbound Follow before recording pending state so a transient
	// activity write failure cannot wedge retries behind a stale relationship row.
	activityID := s.newCanonicalLocalActivityID()
	followActivity := s.buildFollowActivity(ctx, follower, following, cmd.FollowerID, cmd.FollowingID, activityID, nil)
	if err := s.persistOutboundFollowActivity(ctx, followActivity); err != nil {
		return nil, err
	}

	s.logger.Info("invoking createRelationship",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID),
		zap.String("activity_id", activityID))
	if err := s.createRelationship(ctx, cmd.FollowerID, cmd.FollowingID, activityID); err != nil {
		return nil, err
	}
	followActivity, existingResult, err = s.reconcileStoredFollowActivity(ctx, follower, following, cmd.FollowerID, cmd.FollowingID, followActivity)
	if err != nil {
		return nil, err
	}
	if existingResult != nil {
		return existingResult, nil
	}

	// Handle approval workflow
	s.logger.Info("processing follow approval",
		zap.Bool("requires_manual_approval", following != nil && following.Actor != nil && following.Actor.ManuallyApprovesFollowers))
	result, err := s.processFollowApproval(ctx, follower, following, followActivity, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("follow request processed",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID),
		zap.Bool("requires_approval", result.RequestID != ""),
		zap.Bool("is_following", result.IsFollowing))

	return result, nil
}

// getFollowAccounts retrieves the follower and following accounts
func (s *Service) getFollowAccounts(ctx context.Context, followerID, followingID string) (*storage.Account, *storage.Account, error) {
	followerID = s.normalizeActorIdentifier(followerID)
	followingID = s.normalizeActorIdentifier(followingID)

	if s.accountRepo != nil {
		follower, err := s.accountRepo.GetAccount(ctx, followerID)
		if err != nil {
			return nil, nil, err
		}
		following, err := s.accountRepo.GetAccount(ctx, followingID)
		if err != nil {
			return nil, nil, err
		}
		return follower, following, nil
	}

	if s.storage != nil {
		follower, err := s.resolveFollowStorageAccount(ctx, followerID, true)
		if err != nil {
			return nil, nil, err
		}
		following, err := s.resolveFollowStorageAccount(ctx, followingID, false)
		if err != nil {
			return nil, nil, err
		}
		return follower, following, nil
	}

	return nil, nil, NoRepositoryOrStorage()
}

func (s *Service) resolveFollowStorageAccount(ctx context.Context, identifier string, requireLocal bool) (*storage.Account, error) {
	return s.resolveStorageAccount(ctx, identifier, requireLocal, false)
}

func (s *Service) resolveFollowDecisionAccount(ctx context.Context, identifier string, requireLocal bool) (*storage.Account, error) {
	return s.resolveStorageAccount(ctx, identifier, requireLocal, true)
}

func (s *Service) resolveStorageAccount(ctx context.Context, identifier string, requireLocal, requireDeliverable bool) (*storage.Account, error) {
	if s.storage == nil {
		return nil, NoRepositoryOrStorage()
	}

	resolver := federation.NewRemoteSearchService(s.storage)

	var (
		resolution *federation.ExactActorResolution
		err        error
	)
	if requireDeliverable {
		resolution, err = resolver.ResolveDeliverableActor(ctx, identifier, s.domainName)
	} else {
		resolution, err = resolver.ResolveExactActor(ctx, identifier, s.domainName)
	}
	if err != nil {
		return nil, err
	}
	if resolution == nil || resolution.Actor == nil {
		return nil, common.ActorNotFoundError{Username: identifier}
	}
	if requireLocal && resolution.IsRemote {
		return nil, common.ActorNotFoundError{Username: identifier}
	}

	fallbackUsername := resolution.Username
	if resolution.IsRemote && strings.TrimSpace(resolution.Acct) != "" {
		fallbackUsername = resolution.Acct
	}

	account := s.buildAccountFromActor(ctx, resolution.Actor, fallbackUsername)
	if account == nil {
		return nil, common.ActorNotFoundError{Username: identifier}
	}

	return account, nil
}

// checkFollowPrerequisites checks if already following or blocked
func (s *Service) checkFollowPrerequisites(ctx context.Context, followerID, followingID string) (*FollowResult, bool, error) {
	followerID = s.normalizeActorIdentifier(followerID)
	followingID = s.normalizeActorIdentifier(followingID)

	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, false, RepositoryNotAvailable("relationship")
	}

	// Check if already following
	isFollowing, err := repo.IsFollowing(ctx, followerID, followingID)
	if err != nil {
		s.logger.Error("failed to check existing follow relationship",
			zap.String("follower_id", followerID),
			zap.String("following_id", followingID),
			zap.String("error_code", string(pkgerrors.GetErrorCode(err))),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.Error(err))
		return nil, false, err
	}

	if isFollowing {
		relationship, err := s.GetRelationship(ctx, followerID, followingID)
		if err != nil {
			return nil, false, err
		}
		result := &FollowResult{
			Relationship: relationship,
			IsFollowing:  true,
			Events:       []*streaming.Event{},
		}
		result.Activity = s.buildFollowActivity(ctx, nil, nil, followerID, followingID, "", relationship)
		return result, true, nil
	}

	hasPending, requestID, err := s.pendingFollowState(ctx, followerID, followingID)
	if err != nil {
		return nil, false, err
	}
	if hasPending {
		relationship, err := s.GetRelationship(ctx, followerID, followingID)
		if err != nil {
			return nil, false, err
		}

		result := &FollowResult{
			Relationship: relationship,
			RequestID:    requestID,
			IsFollowing:  false,
			Events:       []*streaming.Event{},
		}
		result.Activity = s.buildFollowActivity(ctx, nil, nil, followerID, followingID, requestID, relationship)
		return result, true, nil
	}

	// Check if blocked
	isBlocked, err := repo.IsBlocked(ctx, followingID, followerID)
	if err != nil {
		return nil, false, err
	}

	if isBlocked {
		return nil, false, ErrFollowWhileBlocked
	}

	return nil, false, nil
}

func (s *Service) pendingFollowState(ctx context.Context, followerID, followingID string) (bool, string, error) {
	if s.storage != nil {
		relationshipRepo := s.storage.Relationship()
		if relationshipRepo == nil {
			return false, "", RepositoryNotAvailable("relationship")
		}

		hasPending, err := relationshipRepo.HasPendingFollowRequest(ctx, followerID, followingID)
		if err != nil {
			return false, "", err
		}
		if !hasPending {
			return false, "", nil
		}

		relationship, err := relationshipRepo.GetRelationship(ctx, followerID, followingID)
		if err != nil || relationship == nil {
			return true, "", err
		}

		return true, relationship.ActivityID, nil
	}

	return false, "", nil
}

// createRelationship creates the relationship record
func (s *Service) buildFollowActivity(ctx context.Context, follower, following *storage.Account, followerID, followingID, activityID string, relationship *RelationshipData) *activitypub.Activity {
	followerID = s.normalizeActorIdentifier(followerID)
	followingID = s.normalizeActorIdentifier(followingID)

	follower = s.ensureAccountForActivity(ctx, follower, followerID)
	following = s.ensureAccountForActivity(ctx, following, followingID)

	actorID := followerID
	if follower != nil {
		if follower.Actor != nil {
			switch {
			case follower.Actor.ID != "":
				actorID = follower.Actor.ID
			case follower.Actor.PreferredUsername != "":
				actorID = follower.Actor.PreferredUsername
			}
		} else if follower.User != nil && follower.User.Username != "" {
			actorID = follower.User.Username
		}
	}

	if actorID == "" {
		return nil
	}
	if !strings.Contains(actorID, "://") && s.baseURL() != "" {
		actorID = fmt.Sprintf("%s/users/%s", s.baseURL(), url.PathEscape(actorID))
	}

	objectSlug := followingID
	if following != nil {
		if following.User != nil && following.User.Username != "" {
			objectSlug = following.User.Username
		} else if following.Actor != nil && following.Actor.PreferredUsername != "" {
			objectSlug = following.Actor.PreferredUsername
		}
	}
	if objectSlug == "" {
		objectSlug = followingID
	}

	objectIdentifier := objectSlug
	if following != nil && following.Actor != nil {
		switch {
		case following.Actor.ID != "":
			objectIdentifier = following.Actor.ID
		case following.Actor.URL != "":
			objectIdentifier = following.Actor.URL
		}
	}
	if objectIdentifier == "" {
		objectIdentifier = objectSlug
	}
	if !strings.Contains(objectIdentifier, "://") && s.baseURL() != "" && !strings.Contains(objectIdentifier, "@") {
		objectIdentifier = fmt.Sprintf("%s/users/%s", s.baseURL(), url.PathEscape(objectIdentifier))
	}

	baseActor := strings.TrimSuffix(actorID, "/")
	if baseActor == "" {
		baseActor = actorID
	}

	canonicalActivityID := s.canonicalizeLocalActivityID(activityID)
	if canonicalActivityID == "" {
		canonicalActivityID = fmt.Sprintf("%s/follows/%s", baseActor, url.PathEscape(objectSlug))
	}

	published := time.Now().UTC()
	if relationship != nil && !relationship.CreatedAt.IsZero() {
		published = relationship.CreatedAt.UTC()
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        canonicalActivityID,
			Type:      activitypub.FollowType,
			Published: &published,
		},
		Actor:  actorID,
		Object: objectIdentifier,
	}
	if objectIdentifier != "" {
		activity.To = []string{objectIdentifier}
	}

	return activity
}

func (s *Service) newCanonicalLocalActivityID() string {
	baseURL := s.baseURL()
	if baseURL == "" {
		baseURL = "https://example.invalid"
	}

	return fmt.Sprintf("%s/activities/%s", baseURL, uuid.New().String())
}

func (s *Service) canonicalizeLocalActivityID(activityID string) string {
	trimmed := strings.TrimSpace(activityID)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}

	baseURL := s.baseURL()
	if baseURL == "" {
		baseURL = "https://example.invalid"
	}

	return fmt.Sprintf("%s/activities/%s", baseURL, trimmed)
}

func (s *Service) ensureAccountForActivity(ctx context.Context, account *storage.Account, identifier string) *storage.Account {
	if account != nil && account.Actor != nil {
		return account
	}

	normalized := s.normalizeActorIdentifier(identifier)
	if normalized == "" {
		return account
	}

	if s.accountRepo != nil {
		if fetched, err := s.accountRepo.GetAccount(ctx, normalized); err == nil && fetched != nil {
			return fetched
		}
	}

	if s.storage != nil {
		if actorRepo := s.storage.Actor(); actorRepo != nil {
			if actor, err := actorRepo.GetActor(ctx, normalized); err == nil && actor != nil {
				return &storage.Account{
					User: &storage.User{
						Username: normalized,
					},
					Actor: actor,
				}
			}
		}
	}

	if account == nil {
		return &storage.Account{
			User: &storage.User{
				Username: normalized,
			},
		}
	}

	if account.User == nil {
		account.User = &storage.User{Username: normalized}
	}

	return account
}

func (s *Service) createRelationship(ctx context.Context, followerID, followingID, activityID string) error {
	rawFollowerID := followerID
	rawFollowingID := followingID
	followerID = s.normalizeActorIdentifier(followerID)
	followingID = s.normalizeActorIdentifier(followingID)

	s.logger.Info("createRelationship invoked",
		zap.String("raw_follower_id", rawFollowerID),
		zap.String("raw_following_id", rawFollowingID),
		zap.String("normalized_follower_id", followerID),
		zap.String("normalized_following_id", followingID),
		zap.String("activity_id", activityID),
		zap.Bool("storage_available", s.storage != nil),
		zap.Bool("relationship_repo_available", s.relationshipRepo != nil))

	if s.storage != nil {
		relRepo := s.storage.Relationship()
		if relRepo == nil {
			return RepositoryNotAvailable("relationship")
		}

		s.logger.Info("creating follow relationship",
			zap.String("follower_id", followerID),
			zap.String("following_id", followingID),
			zap.String("activity_id", activityID))

		if err := relRepo.CreateRelationship(ctx, followerID, followingID, activityID); err != nil {
			s.logger.Error("failed to create follow relationship",
				zap.String("follower_id", followerID),
				zap.String("following_id", followingID),
				zap.String("activity_id", activityID),
				zap.Error(err))
			return err
		}

		return nil
	}
	if s.relationshipRepo != nil {
		// Legacy interface only supports CreateFollowRequest (without activityID)
		return s.relationshipRepo.CreateFollowRequest(ctx, followerID, followingID)
	}
	return NoRepositoryOrStorage()
}

func (s *Service) reconcileStoredFollowActivity(
	ctx context.Context,
	follower, following *storage.Account,
	followerID, followingID string,
	followActivity *activitypub.Activity,
) (*activitypub.Activity, *FollowResult, error) {
	if followActivity == nil || s.storage == nil {
		return followActivity, nil, nil
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return followActivity, nil, RepositoryNotAvailable("relationship")
	}

	relationshipRecord, err := relationshipRepo.GetRelationship(ctx, followerID, followingID)
	if err != nil || relationshipRecord == nil {
		return followActivity, nil, err
	}

	generatedActivityID := strings.TrimSpace(followActivity.ID)
	storedActivityID := strings.TrimSpace(relationshipRecord.ActivityID)
	if generatedActivityID == "" || storedActivityID == "" || generatedActivityID == storedActivityID {
		return followActivity, nil, nil
	}
	if err := s.deleteSupersededFollowActivity(ctx, generatedActivityID, storedActivityID); err != nil {
		return nil, nil, err
	}

	relationship, err := s.GetRelationship(ctx, followerID, followingID)
	if err != nil {
		return nil, nil, err
	}

	canonicalActivity := s.loadStoredFollowActivity(ctx, storedActivityID)
	if canonicalActivity == nil {
		canonicalActivity = s.buildFollowActivity(ctx, follower, following, followerID, followingID, storedActivityID, relationship)
	}

	result := &FollowResult{
		Relationship: relationship,
		Events:       []*streaming.Event{},
		Activity:     canonicalActivity,
	}
	switch relationshipRecord.State {
	case models.RelationshipAccepted:
		result.IsFollowing = true
	case models.RelationshipPending:
		result.RequestID = storedActivityID
	}

	s.logger.Info("follow request adopted existing canonical activity id",
		zap.String("follower_id", followerID),
		zap.String("following_id", followingID),
		zap.String("generated_activity_id", generatedActivityID),
		zap.String("stored_activity_id", storedActivityID),
		zap.String("relationship_state", relationshipRecord.State))

	return canonicalActivity, result, nil
}

func (s *Service) deleteSupersededFollowActivity(ctx context.Context, generatedActivityID, storedActivityID string) error {
	if s.storage == nil {
		return nil
	}

	generatedActivityID = strings.TrimSpace(generatedActivityID)
	storedActivityID = strings.TrimSpace(storedActivityID)
	if generatedActivityID == "" || generatedActivityID == storedActivityID {
		return nil
	}

	activityRepo := s.storage.Activity()
	if activityRepo == nil {
		return nil
	}

	deleteRepo, ok := activityRepo.(interface {
		DeleteActivity(ctx context.Context, activityID string) error
	})
	if !ok {
		s.logger.Debug("activity repository does not support deleting superseded follow activities",
			zap.String("activity_id", generatedActivityID),
			zap.String("stored_activity_id", storedActivityID))
		return nil
	}

	if err := deleteRepo.DeleteActivity(ctx, generatedActivityID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		s.logger.Error("failed to delete superseded follow activity",
			zap.String("activity_id", generatedActivityID),
			zap.String("stored_activity_id", storedActivityID),
			zap.Error(err))
		return err
	}

	return nil
}

func (s *Service) loadStoredFollowActivity(ctx context.Context, activityID string) *activitypub.Activity {
	if s.storage == nil {
		return nil
	}

	activityRepo := s.storage.Activity()
	if activityRepo == nil {
		return nil
	}

	activity, err := activityRepo.GetActivity(ctx, activityID)
	if err != nil {
		s.logger.Debug("failed to load stored follow activity, rebuilding from relationship state",
			zap.String("activity_id", activityID),
			zap.Error(err))
		return nil
	}

	return activity
}

// processFollowApproval handles the approval workflow and emits events
func (s *Service) processFollowApproval(ctx context.Context, follower, following *storage.Account, followActivity *activitypub.Activity, followerID, followingID string) (*FollowResult, error) {
	requiresApproval := following.Actor != nil && following.Actor.ManuallyApprovesFollowers
	requiresRemoteAcceptance := s.followRequiresRemoteAcceptance(following)

	var events []*streaming.Event
	var requestID string
	isFollowingNow := false
	activityID := ""
	if followActivity != nil {
		activityID = strings.TrimSpace(followActivity.ID)
	}

	if requiresApproval || requiresRemoteAcceptance {
		requestID = activityID
		events = s.emitFollowRequestedEvents(ctx, follower, following, activityID)
		s.queueFederationFollowRequest(ctx, followActivity, follower, following)
	} else {
		if err := s.acceptFollowRequest(ctx, followerID, followingID); err != nil {
			return nil, err
		}
		isFollowingNow = true
		events = s.emitFollowAcceptedEvents(ctx, follower, following, activityID)
		s.queueFederationFollowDirectly(ctx, followActivity, follower, following)
	}

	relationship, err := s.GetRelationship(ctx, followerID, followingID)
	if err != nil {
		return nil, err
	}

	return &FollowResult{
		Relationship: relationship,
		RequestID:    requestID,
		IsFollowing:  isFollowingNow,
		Events:       events,
		Activity:     followActivity,
	}, nil
}

func (s *Service) persistOutboundFollowActivity(ctx context.Context, followActivity *activitypub.Activity) error {
	if followActivity == nil || s.storage == nil {
		return nil
	}

	activityRepo := s.storage.Activity()
	if activityRepo == nil {
		return RepositoryNotAvailable("activity")
	}

	return activityRepo.CreateActivity(ctx, followActivity)
}

func (s *Service) followRequiresRemoteAcceptance(following *storage.Account) bool {
	if following == nil || following.Actor == nil {
		return false
	}

	return !isLocalActor(following.Actor, s.domainName)
}

// acceptFollowRequest accepts a follow request
func (s *Service) acceptFollowRequest(ctx context.Context, followerID, followingID string) error {
	followerID = s.normalizeActorIdentifier(followerID)
	followingID = s.normalizeActorIdentifier(followingID)

	if s.relationshipRepo != nil {
		return s.relationshipRepo.AcceptFollowRequest(ctx, followerID, followingID)
	}
	if s.storage != nil {
		return s.storage.Relationship().AcceptFollowRequest(ctx, followerID, followingID)
	}
	return NoRepositoryOrStorage()
}

// removeRelationshipParams contains parameters for relationship removal
type removeRelationshipParams struct {
	actorID        string
	targetID       string
	relationType   string
	actorName      string
	targetName     string
	checkExistsFn  func(context.Context, string, string) (bool, error)
	removeFn       func(context.Context, string, string) error
	emitEventsFn   func(context.Context, *storage.Account, *storage.Account) []*streaming.Event
	federationType string
}

// removeRelationshipGeneric handles the common pattern for removing relationships
func (s *Service) removeRelationshipGeneric(ctx context.Context, params removeRelationshipParams) (*RelationshipResult, error) {
	params.actorID = s.normalizeActorIdentifier(params.actorID)
	params.targetID = s.normalizeActorIdentifier(params.targetID)

	s.logger.Info("processing relationship request",
		zap.String(params.actorName, params.actorID),
		zap.String(params.targetName, params.targetID))

	// Check if relationship currently exists
	exists, err := params.checkExistsFn(ctx, params.actorID, params.targetID)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Relationship doesn't exist - return current relationship (idempotent success)
		s.logger.Debug("relationship does not exist, treating removal as idempotent success",
			zap.String(params.actorName, params.actorID),
			zap.String(params.targetName, params.targetID))

		relationship, err := s.GetRelationship(ctx, params.actorID, params.targetID)
		if err != nil {
			// Don't mask the error - we need to know why GetRelationship failed
			s.logger.Error("failed to get relationship status after idempotent check",
				zap.String(params.actorName, params.actorID),
				zap.String(params.targetName, params.targetID),
				zap.Error(err))
			return nil, err
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	var actor, target *storage.Account

	if s.accountRepo != nil {
		var err error
		actor, err = s.accountRepo.GetAccount(ctx, params.actorID)
		if err != nil {
			s.logger.Error("failed to get actor account for relationship removal",
				zap.String(params.actorName, params.actorID),
				zap.Error(err))
			return nil, err
		}

		target, err = s.accountRepo.GetAccount(ctx, params.targetID)
		if err != nil {
			s.logger.Error("failed to get target account for relationship removal",
				zap.String(params.targetName, params.targetID),
				zap.Error(err))
			return nil, err
		}
	} else if s.storage != nil {
		// Fallback to Actor repository if accountRepo is not available
		var err error
		actorActor, err := s.storage.Actor().GetActor(ctx, params.actorID)
		if err != nil {
			s.logger.Error("failed to get actor for relationship removal",
				zap.String(params.actorName, params.actorID),
				zap.Error(err))
			return nil, err
		}
		targetActor, err := s.storage.Actor().GetActor(ctx, params.targetID)
		if err != nil {
			s.logger.Error("failed to get target actor for relationship removal",
				zap.String(params.targetName, params.targetID),
				zap.Error(err))
			return nil, err
		}

		actor = &storage.Account{
			User:  &storage.User{Username: actorActor.PreferredUsername},
			Actor: actorActor,
		}
		target = &storage.Account{
			User:  &storage.User{Username: targetActor.PreferredUsername},
			Actor: targetActor,
		}
	} else {
		// No account repository available - create minimal accounts for events
		actor = &storage.Account{
			User: &storage.User{Username: params.actorID},
		}
		target = &storage.Account{
			User: &storage.User{Username: params.targetID},
		}
	}

	// Remove relationship
	err = params.removeFn(ctx, params.actorID, params.targetID)
	if err != nil {
		return nil, err
	}

	// Emit events and queue federation
	events := params.emitEventsFn(ctx, actor, target)
	s.queueFederationUndo(ctx, actor, target, params.federationType)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, params.actorID, params.targetID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("relationship processed",
		zap.String(params.actorName, params.actorID),
		zap.String(params.targetName, params.targetID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Unfollow removes a follow relationship and emits events
func (s *Service) Unfollow(ctx context.Context, cmd *UnfollowCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateUnfollowCommand(ctx, cmd); err != nil {
		return nil, err
	}

	// Normalize identifiers so storage lookups use canonical keys
	cmd.FollowerID = s.normalizeActorIdentifier(cmd.FollowerID)
	cmd.FollowingID = s.normalizeActorIdentifier(cmd.FollowingID)

	// Get relationship repository
	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	return s.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        cmd.FollowerID,
		targetID:       cmd.FollowingID,
		relationType:   "unfollow",
		actorName:      "follower_id",
		targetName:     "following_id",
		checkExistsFn:  repo.IsFollowing,
		removeFn:       repo.Unfollow,
		emitEventsFn:   s.emitUnfollowEvents,
		federationType: "Follow",
	})
}

// Block blocks a user, automatically unfollows, and emits events
func (s *Service) Block(ctx context.Context, cmd *BlockCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateBlockCommand(ctx, cmd); err != nil {
		return nil, err
	}

	// Normalize identifiers before any storage access
	cmd.BlockerID = s.normalizeActorIdentifier(cmd.BlockerID)
	cmd.BlockedID = s.normalizeActorIdentifier(cmd.BlockedID)

	s.logger.Info("processing block request",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	// Prevent self-blocks
	if cmd.BlockerID == cmd.BlockedID {
		return nil, CannotBlockSelf()
	}

	// Get relationship repository
	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	// Check if already blocked
	isBlocked, err := repo.IsBlocked(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, err
	}

	if isBlocked {
		// Already blocked - return current relationship
		relationship, err := s.GetRelationship(ctx, cmd.BlockerID, cmd.BlockedID)
		if err != nil {
			return nil, err
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	var blocker, blocked *storage.Account
	if s.accountRepo != nil {
		blocker, err = s.accountRepo.GetAccount(ctx, cmd.BlockerID)
		if err != nil {
			return nil, err
		}
		blocked, err = s.accountRepo.GetAccount(ctx, cmd.BlockedID)
		if err != nil {
			return nil, err
		}
	} else if s.storage != nil {
		blockerActor, err := s.storage.Actor().GetActor(ctx, cmd.BlockerID)
		if err != nil {
			return nil, err
		}
		blockedActor, err := s.storage.Actor().GetActor(ctx, cmd.BlockedID)
		if err != nil {
			return nil, err
		}
		blocker = &storage.Account{
			User:  &storage.User{Username: blockerActor.PreferredUsername},
			Actor: blockerActor,
		}
		blocked = &storage.Account{
			User:  &storage.User{Username: blockedActor.PreferredUsername},
			Actor: blockedActor,
		}
	}

	// Automatically unfollow if currently following
	isFollowing, err := repo.IsFollowing(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		s.logger.Warn("failed to check follow status during block", zap.Error(err))
	} else if isFollowing {
		if err := repo.Unfollow(ctx, cmd.BlockerID, cmd.BlockedID); err != nil {
			s.logger.Warn("failed to unfollow during block", zap.Error(err))
		}
	}

	// Also unfollow reverse relationship
	isFollowedBy, err := repo.IsFollowing(ctx, cmd.BlockedID, cmd.BlockerID)
	if err != nil {
		s.logger.Warn("failed to check reverse follow status during block", zap.Error(err))
	} else if isFollowedBy {
		if err := repo.Unfollow(ctx, cmd.BlockedID, cmd.BlockerID); err != nil {
			s.logger.Warn("failed to unfollow reverse during block", zap.Error(err))
		}
	}

	// Create block
	err = repo.BlockUser(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		s.auditRelationshipEvent(ctx, "relationship.block", cmd.BlockerID, cmd.BlockedID, false, "block_failed", nil)
		return nil, err
	}

	// Emit events (only to blocker's stream for privacy)
	events := s.emitBlockEvents(ctx, blocker, blocked)
	s.queueFederationBlock(ctx, blocker, blocked)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.BlockerID, cmd.BlockedID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("block processed",
		zap.String("blocker_id", cmd.BlockerID),
		zap.String("blocked_id", cmd.BlockedID))

	s.auditRelationshipEvent(ctx, "relationship.block", cmd.BlockerID, cmd.BlockedID, true, "", nil)

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Unblock removes a user block and emits events
func (s *Service) Unblock(ctx context.Context, cmd *UnblockCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateUnblockCommand(ctx, cmd); err != nil {
		return nil, err
	}

	cmd.BlockerID = s.normalizeActorIdentifier(cmd.BlockerID)
	cmd.BlockedID = s.normalizeActorIdentifier(cmd.BlockedID)

	// Get relationship repository
	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	result, err := s.removeRelationshipGeneric(ctx, removeRelationshipParams{
		actorID:        cmd.BlockerID,
		targetID:       cmd.BlockedID,
		relationType:   "unblock",
		actorName:      "blocker_id",
		targetName:     "blocked_id",
		checkExistsFn:  repo.IsBlocked,
		removeFn:       repo.UnblockUser,
		emitEventsFn:   s.emitUnblockEvents,
		federationType: "Block",
	})
	if err != nil {
		s.auditRelationshipEvent(ctx, "relationship.unblock", cmd.BlockerID, cmd.BlockedID, false, "unblock_failed", nil)
		return nil, err
	}

	s.auditRelationshipEvent(ctx, "relationship.unblock", cmd.BlockerID, cmd.BlockedID, true, "", nil)
	return result, nil
}

// Mute mutes a user (hides from timelines) and emits events
func (s *Service) Mute(ctx context.Context, cmd *MuteCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateMuteCommand(ctx, cmd); err != nil {
		return nil, err
	}

	cmd.MuterID = s.normalizeActorIdentifier(cmd.MuterID)
	cmd.MutedID = s.normalizeActorIdentifier(cmd.MutedID)

	s.logger.Info("processing mute request",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	// Prevent self-mutes
	if cmd.MuterID == cmd.MutedID {
		return nil, CannotMuteSelf()
	}

	// Get relationship repository
	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	// Check if already muted
	isMuted, err := repo.IsMuted(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, err
	}

	if isMuted {
		// Already muted - return current relationship
		relationship, err := s.GetRelationship(ctx, cmd.MuterID, cmd.MutedID)
		if err != nil {
			return nil, err
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	var muter, muted *storage.Account
	if s.accountRepo != nil {
		muter, err = s.accountRepo.GetAccount(ctx, cmd.MuterID)
		if err != nil {
			return nil, err
		}
		muted, err = s.accountRepo.GetAccount(ctx, cmd.MutedID)
		if err != nil {
			return nil, err
		}
	} else if s.storage != nil {
		muterActor, err := s.storage.Actor().GetActor(ctx, cmd.MuterID)
		if err != nil {
			return nil, err
		}
		mutedActor, err := s.storage.Actor().GetActor(ctx, cmd.MutedID)
		if err != nil {
			return nil, err
		}
		muter = &storage.Account{
			User:  &storage.User{Username: muterActor.PreferredUsername},
			Actor: muterActor,
		}
		muted = &storage.Account{
			User:  &storage.User{Username: mutedActor.PreferredUsername},
			Actor: mutedActor,
		}
	}

	// Create mute - need to use CreateMute which takes more parameters
	activityIDMute := uuid.New().String()

	// Always use storage for CreateMute since the interface doesn't include it
	if s.storage != nil {
		err = s.storage.Relationship().CreateMute(ctx, cmd.MuterID, cmd.MutedID, activityIDMute, cmd.MuteNotifications, cmd.Duration)
	} else {
		return nil, NoRepositoryOrStorage()
	}

	if err != nil {
		return nil, err
	}

	// Emit events (only to muter's stream for privacy)
	events := s.emitMuteEvents(ctx, muter, muted, cmd.Duration)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, err
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
	// Validate command
	if err := s.validateUnmuteCommand(ctx, cmd); err != nil {
		return nil, err
	}

	cmd.MuterID = s.normalizeActorIdentifier(cmd.MuterID)
	cmd.MutedID = s.normalizeActorIdentifier(cmd.MutedID)

	s.logger.Info("processing unmute request",
		zap.String("muter_id", cmd.MuterID),
		zap.String("muted_id", cmd.MutedID))

	// Get relationship repository
	repo := s.getRelationshipRepo()
	if repo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	// Check if currently muted
	isMuted, err := repo.IsMuted(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, err
	}

	if !isMuted {
		// Not muted - return current relationship
		relationship, err := s.GetRelationship(ctx, cmd.MuterID, cmd.MutedID)
		if err != nil {
			return nil, err
		}

		return &RelationshipResult{
			Relationship: relationship,
			Events:       []*streaming.Event{},
		}, nil
	}

	// Get accounts for events
	var muter, muted *storage.Account
	if s.accountRepo != nil {
		muter, err = s.accountRepo.GetAccount(ctx, cmd.MuterID)
		if err != nil {
			return nil, err
		}
		muted, err = s.accountRepo.GetAccount(ctx, cmd.MutedID)
		if err != nil {
			return nil, err
		}
	} else if s.storage != nil {
		// Get accounts via Actor repository
		muterActor, err := s.storage.Actor().GetActor(ctx, cmd.MuterID)
		if err != nil {
			return nil, err
		}
		mutedActor, err := s.storage.Actor().GetActor(ctx, cmd.MutedID)
		if err != nil {
			return nil, err
		}
		muter = &storage.Account{
			User:  &storage.User{Username: muterActor.PreferredUsername},
			Actor: muterActor,
		}
		muted = &storage.Account{
			User:  &storage.User{Username: mutedActor.PreferredUsername},
			Actor: mutedActor,
		}
	}

	// Remove mute
	err = repo.UnmuteUser(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, err
	}

	// Emit events (only to muter's stream for privacy)
	events := s.emitUnmuteEvents(ctx, muter, muted)

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.MuterID, cmd.MutedID)
	if err != nil {
		return nil, err
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
func (s *Service) GetRelationship(ctx context.Context, requesterID, targetID string) (*RelationshipData, error) {
	// Basic validation
	if err := common.ValidateRequiredParam("requester_id", requesterID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("target_id", targetID); err != nil {
		return nil, err
	}

	requesterID = s.normalizeActorIdentifier(requesterID)
	targetID = s.normalizeActorIdentifier(targetID)

	s.logger.Debug("getting relationship",
		zap.String("requester_id", requesterID),
		zap.String("target_id", targetID))

	// Build relationship data
	relationship, err := s.buildRelationshipData(ctx, requesterID, targetID)
	if err != nil {
		return nil, err
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
		return nil, err
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
				ID:        targetID,
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

// UpdateRelationship updates preferences for an existing relationship
func (s *Service) UpdateRelationship(ctx context.Context, cmd *UpdateRelationshipCommand) (*RelationshipData, error) {
	// Validate command
	if err := common.ValidateRequiredParam("follower_id", cmd.FollowerID); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("following_id", cmd.FollowingID); err != nil {
		return nil, err
	}

	cmd.FollowerID = s.normalizeActorIdentifier(cmd.FollowerID)
	cmd.FollowingID = s.normalizeActorIdentifier(cmd.FollowingID)

	s.logger.Info("updating relationship preferences",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	// Get relationship repository from storage
	if s.storage == nil {
		return nil, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, RepositoryNotAvailable("relationship")
	}

	// Check that relationship exists
	isFollowing, err := relationshipRepo.IsFollowing(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		s.logger.Error("failed to check relationship status", zap.Error(err))
		return nil, FailedToCheckFollowStatus(err)
	}
	if !isFollowing {
		return nil, CheckFollowingRelationship(fmt.Errorf("not following"))
	}

	// Build updates map
	updates := make(map[string]interface{})
	if cmd.Notify != nil {
		updates["Notifying"] = *cmd.Notify
	}
	if cmd.ShowReblogs != nil {
		updates["ShowingReblogs"] = *cmd.ShowReblogs
	}
	if cmd.Languages != nil {
		updates["Languages"] = *cmd.Languages
	}
	if cmd.Note != nil {
		updates["Note"] = *cmd.Note
	}

	// Apply updates
	if len(updates) > 0 {
		err = relationshipRepo.UpdateRelationship(ctx, cmd.FollowerID, cmd.FollowingID, updates)
		if err != nil {
			s.logger.Error("failed to update relationship", zap.Error(err))
			return nil, FailedToGetUpdatedRelationship(err)
		}
	}

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.FollowerID, cmd.FollowingID)
	if err != nil {
		s.logger.Error("failed to get updated relationship", zap.Error(err))
		return nil, FailedToGetUpdatedRelationship(err)
	}

	s.logger.Info("relationship preferences updated successfully",
		zap.String("follower_id", cmd.FollowerID),
		zap.String("following_id", cmd.FollowingID))

	return relationship, nil
}

// Domain Block Commands and Queries

// AddDomainBlockCommand contains data needed to add a domain block
type AddDomainBlockCommand struct {
	UserID string `json:"user_id" validate:"required"`
	Domain string `json:"domain" validate:"required"`
}

// RemoveDomainBlockCommand contains data needed to remove a domain block
type RemoveDomainBlockCommand struct {
	UserID string `json:"user_id" validate:"required"`
	Domain string `json:"domain" validate:"required"`
}

// GetDomainBlocksQuery contains parameters for retrieving domain blocks
type GetDomainBlocksQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

// GetMutedUsersQuery contains parameters for retrieving muted users
type GetMutedUsersQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

// GetBlockedUsersQuery contains parameters for retrieving blocked users
type GetBlockedUsersQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

// GetFollowersQuery contains parameters for retrieving followers
type GetFollowersQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

// Domain Block Results

// DomainBlocksResult contains domain blocks data
type DomainBlocksResult struct {
	Domains    []string           `json:"domains"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Events     []*streaming.Event `json:"events"`
}

// MutedUsersResult contains muted users data
type MutedUsersResult struct {
	MutedUsers []*storage.Account `json:"muted_users"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Events     []*streaming.Event `json:"events"`
}

// BlockedUsersResult contains blocked users data
type BlockedUsersResult struct {
	BlockedUsers []*storage.Account `json:"blocked_users"`
	NextCursor   string             `json:"next_cursor,omitempty"`
	Events       []*streaming.Event `json:"events"`
}

// FollowersResult contains followers data
type FollowersResult struct {
	Followers  []*storage.Account `json:"followers"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Events     []*streaming.Event `json:"events"`
}

// AddDomainBlock adds a domain block for a user
func (s *Service) AddDomainBlock(ctx context.Context, cmd *AddDomainBlockCommand) error {
	s.logger.Info("adding domain block",
		zap.String("user_id", cmd.UserID),
		zap.String("domain", cmd.Domain))

	// Validate command
	if err := s.validateAddDomainBlockCommand(cmd); err != nil {
		return err
	}

	// Get domain block repository from storage
	if s.storage == nil {
		return StorageNotAvailable()
	}

	domainBlockRepo := s.storage.DomainBlock()
	if domainBlockRepo == nil {
		return DomainBlockRepositoryNotAvailable()
	}

	// Add the domain block
	err := domainBlockRepo.AddDomainBlock(ctx, cmd.UserID, cmd.Domain)
	if err != nil {
		return err
	}

	s.logger.Info("domain block added successfully",
		zap.String("user_id", cmd.UserID),
		zap.String("domain", cmd.Domain))

	return nil
}

// RemoveDomainBlock removes a domain block for a user
func (s *Service) RemoveDomainBlock(ctx context.Context, cmd *RemoveDomainBlockCommand) error {
	s.logger.Info("removing domain block",
		zap.String("user_id", cmd.UserID),
		zap.String("domain", cmd.Domain))

	// Validate command
	if err := s.validateRemoveDomainBlockCommand(cmd); err != nil {
		return err
	}

	// Get domain block repository from storage
	if s.storage == nil {
		return StorageNotAvailable()
	}

	domainBlockRepo := s.storage.DomainBlock()
	if domainBlockRepo == nil {
		return DomainBlockRepositoryNotAvailable()
	}

	// Remove the domain block
	err := domainBlockRepo.RemoveDomainBlock(ctx, cmd.UserID, cmd.Domain)
	if err != nil {
		return err
	}

	s.logger.Info("domain block removed successfully",
		zap.String("user_id", cmd.UserID),
		zap.String("domain", cmd.Domain))

	return nil
}

// repositoryQueryParams contains parameters for repository queries
type repositoryQueryParams struct {
	userID     string
	limit      int
	cursor     string
	queryType  string
	validateFn func() error
}

// executeRepositoryQueryGeneric handles the common pattern for repository queries
func (s *Service) executeRepositoryQueryGeneric(_ context.Context, params repositoryQueryParams) error {
	s.logger.Debug(fmt.Sprintf("getting %s", params.queryType),
		zap.String("user_id", params.userID),
		zap.Int("limit", params.limit))

	// Validate query
	if err := params.validateFn(); err != nil {
		return err
	}

	// Get repository from storage
	if s.storage == nil {
		return StorageNotAvailable()
	}

	return nil
}

// GetDomainBlocks retrieves domain blocks for a user
func (s *Service) GetDomainBlocks(ctx context.Context, query *GetDomainBlocksQuery) (*DomainBlocksResult, error) {
	err := s.executeRepositoryQueryGeneric(ctx, repositoryQueryParams{
		userID:     query.UserID,
		limit:      query.Limit,
		cursor:     query.Cursor,
		queryType:  "domain blocks",
		validateFn: func() error { return s.validateGetDomainBlocksQuery(query) },
	})
	if err != nil {
		return nil, err
	}

	domainBlockRepo := s.storage.DomainBlock()

	// Get domain blocks
	domains, nextCursor, err := domainBlockRepo.GetUserDomainBlocks(ctx, query.UserID, query.Limit, query.Cursor)
	if err != nil {
		return nil, err
	}

	return &DomainBlocksResult{
		Domains:    domains,
		NextCursor: nextCursor,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// GetMutedUsers retrieves muted users for a user
func (s *Service) GetMutedUsers(ctx context.Context, query *GetMutedUsersQuery) (*MutedUsersResult, error) {
	s.logger.Debug("getting muted users",
		zap.String("user_id", query.UserID),
		zap.Int("limit", query.Limit))

	// Validate query
	if err := s.validateGetMutedUsersQuery(query); err != nil {
		return nil, err
	}

	// Get social repository from storage
	if s.storage == nil {
		return nil, StorageNotAvailable()
	}

	socialRepo := s.storage.Social()
	if socialRepo == nil {
		return nil, SocialRepositoryNotAvailable()
	}

	// Get muted users
	mutes, nextCursor, err := socialRepo.GetMutedUsers(ctx, query.UserID, query.Limit, query.Cursor)
	if err != nil {
		return nil, err
	}

	// Convert mutes to accounts
	var mutedUsers []*storage.Account
	for _, mute := range mutes {
		// Get the account for each muted user
		actor, err := s.storage.Actor().GetActor(ctx, mute.Object)
		if err != nil {
			s.logger.Warn("failed to get muted actor", zap.String("actor", mute.Object), zap.Error(err))
			continue
		}

		if actor != nil {
			// Convert actor to account (simplified conversion)
			account := &storage.Account{
				User: &storage.User{
					Username: actor.PreferredUsername,
				},
				Actor: actor,
			}
			mutedUsers = append(mutedUsers, account)
		}
	}

	return &MutedUsersResult{
		MutedUsers: mutedUsers,
		NextCursor: nextCursor,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// GetBlockedUsers retrieves blocked users for a user
func (s *Service) GetBlockedUsers(ctx context.Context, query *GetBlockedUsersQuery) (*BlockedUsersResult, error) {
	s.logger.Debug("getting blocked users",
		zap.String("user_id", query.UserID),
		zap.Int("limit", query.Limit))

	// Validate query
	if err := s.validateGetBlockedUsersQuery(query); err != nil {
		return nil, err
	}

	// Get relationship repository from storage
	if s.storage == nil {
		return nil, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, RepositoryNotAvailable("general")
	}

	// Get blocked users
	blockedUserIDs, nextCursor, err := relationshipRepo.GetBlockedUsers(ctx, query.UserID, query.Limit, query.Cursor)
	if err != nil {
		return nil, err
	}

	// Convert blocked user IDs to accounts
	var blockedUsers []*storage.Account
	for _, blockedUserID := range blockedUserIDs {
		// Extract username from actor ID
		parts := strings.Split(blockedUserID, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]

			// Get the actor for each blocked user
			actor, err := s.storage.Actor().GetActor(ctx, username)
			if err != nil {
				s.logger.Warn("failed to get blocked actor", zap.String("actor", blockedUserID), zap.Error(err))
				continue
			}

			account := s.buildAccountFromActor(ctx, actor, username)
			if account != nil {
				blockedUsers = append(blockedUsers, account)
			}
		}
	}

	return &BlockedUsersResult{
		BlockedUsers: blockedUsers,
		NextCursor:   nextCursor,
		Events:       []*streaming.Event{}, // No events for read operations
	}, nil
}

// getRelatedAccounts is a generic helper for getting followers/following
func (s *Service) getRelatedAccounts(ctx context.Context, username string, limit int, cursor string, relationType string) ([]*storage.Account, string, error) {
	s.logger.Debug(fmt.Sprintf("getting %s", relationType),
		zap.String("username", username),
		zap.Int("limit", limit))

	// Get relationship repository from storage
	if s.storage == nil {
		return nil, "", StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, "", RepositoryNotAvailable("general")
	}

	// Get related user IDs based on relation type
	var relatedIDs []string
	var nextCursor string
	var err error

	switch relationType {
	case "followers":
		relatedIDs, nextCursor, err = relationshipRepo.GetFollowers(ctx, username, limit, cursor)
	case "following":
		relatedIDs, nextCursor, err = relationshipRepo.GetFollowing(ctx, username, limit, cursor)
	default:
		return nil, "", ErrUnsupportedRelationType
	}

	if err != nil {
		return nil, "", err
	}

	// Convert user IDs to accounts
	var accounts []*storage.Account
	for _, userID := range relatedIDs {
		account, err := s.resolveRelatedAccount(ctx, userID)
		if err != nil {
			s.logger.Warn(fmt.Sprintf("failed to get %s actor", relationType), zap.String("actor", userID), zap.Error(err))
			continue
		}
		if account != nil {
			accounts = append(accounts, account)
		}
	}

	return accounts, nextCursor, nil
}

func (s *Service) resolveRelatedAccount(ctx context.Context, identifier string) (*storage.Account, error) {
	normalized := s.normalizeActorIdentifier(identifier)
	if normalized == "" {
		return nil, common.ActorNotFoundError{Username: identifier}
	}

	// Local usernames stay on the direct actor lookup path.
	if !strings.Contains(normalized, "@") {
		actor, err := s.storage.Actor().GetActor(ctx, normalized)
		if err != nil {
			return nil, err
		}
		return s.buildAccountFromActor(ctx, actor, normalized), nil
	}

	return s.resolveFollowStorageAccount(ctx, normalized, false)
}

// GetFollowers retrieves followers for a user
func (s *Service) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
	return s.getRelatedAccounts(ctx, username, limit, cursor, "followers")
}

// GetFollowing retrieves users being followed by a user
func (s *Service) GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
	return s.getRelatedAccounts(ctx, username, limit, cursor, "following")
}

// CountFollowers counts the number of followers for a user
func (s *Service) CountFollowers(ctx context.Context, username string) (int64, error) {
	relationshipRepo := s.storage.Relationship()
	count, err := relationshipRepo.CountFollowers(ctx, username)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// IsMuted checks if one user has muted another
func (s *Service) IsMuted(ctx context.Context, muterID, mutedID string) (bool, error) {
	socialRepo := s.storage.Social()
	isMuted, err := socialRepo.IsMuted(ctx, muterID, mutedID)
	if err != nil {
		return false, err
	}
	return isMuted, nil
}

// Follow Request Commands and Queries

// GetFollowRequestsQuery contains parameters for retrieving pending follow requests
type GetFollowRequestsQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

// AcceptFollowRequestCommand contains data needed to accept a follow request
type AcceptFollowRequestCommand struct {
	RequesterID string `json:"requester_id" validate:"required"` // User accepting the request
	FollowerID  string `json:"follower_id" validate:"required"`  // User who sent the request
}

// RejectFollowRequestCommand contains data needed to reject a follow request
type RejectFollowRequestCommand struct {
	RequesterID string `json:"requester_id" validate:"required"` // User rejecting the request
	FollowerID  string `json:"follower_id" validate:"required"`  // User who sent the request
}

// Follow Request Results

// FollowRequestsResult contains follow requests data
type FollowRequestsResult struct {
	FollowerIDs []string           `json:"follower_ids"`
	NextCursor  string             `json:"next_cursor,omitempty"`
	Events      []*streaming.Event `json:"events"`
}

// GetPendingFollowRequests retrieves pending follow requests for a user
func (s *Service) GetPendingFollowRequests(ctx context.Context, query *GetFollowRequestsQuery) (*FollowRequestsResult, error) {
	err := s.executeRepositoryQueryGeneric(ctx, repositoryQueryParams{
		userID:     query.UserID,
		limit:      query.Limit,
		cursor:     query.Cursor,
		queryType:  "pending follow requests",
		validateFn: func() error { return s.validateGetFollowRequestsQuery(query) },
	})
	if err != nil {
		return nil, err
	}

	query.UserID = s.normalizeActorIdentifier(query.UserID)

	relationshipRepo := s.storage.Relationship()

	// Get pending follow requests using the concrete method
	followerIDs, nextCursor, err := relationshipRepo.GetPendingFollowRequests(ctx, query.UserID, query.Limit, query.Cursor)
	if err != nil {
		return nil, err
	}

	return &FollowRequestsResult{
		FollowerIDs: followerIDs,
		NextCursor:  nextCursor,
		Events:      []*streaming.Event{}, // No events for read operations
	}, nil
}

// AcceptFollowRequest accepts a pending follow request
func (s *Service) AcceptFollowRequest(ctx context.Context, cmd *AcceptFollowRequestCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateAcceptFollowRequestCommand(cmd); err != nil {
		return nil, err
	}

	cmd.RequesterID = s.normalizeActorIdentifier(cmd.RequesterID)
	cmd.FollowerID = s.normalizeActorIdentifier(cmd.FollowerID)

	s.logger.Info("accepting follow request",
		zap.String("requester_id", cmd.RequesterID),
		zap.String("follower_id", cmd.FollowerID))

	// Get relationship repository from storage
	if s.storage == nil {
		return nil, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, RepositoryNotAvailable("general")
	}

	// Check if the follow request exists.
	followRequest, err := relationshipRepo.GetFollowRequest(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	// Accept the follow request
	err = relationshipRepo.AcceptFollowRequest(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	// Resolve actors for events and later federation work without assuming the follower is local.
	var follower, following *storage.Account
	if s.storage != nil {
		follower, err = s.resolveFollowDecisionAccount(ctx, cmd.FollowerID, false)
		if err != nil {
			s.logger.Warn("failed to resolve follower for events", zap.String("follower_id", cmd.FollowerID), zap.Error(err))
		}

		following, err = s.resolveFollowDecisionAccount(ctx, cmd.RequesterID, true)
		if err != nil {
			s.logger.Warn("failed to resolve requester for events", zap.String("requester_id", cmd.RequesterID), zap.Error(err))
		}
	}

	// Emit events
	var events []*streaming.Event
	if follower != nil && following != nil {
		activityID := uuid.New().String()
		events = s.emitFollowAcceptedEvents(ctx, follower, following, activityID)
		s.queueFederationAccept(ctx, follower, following, followRequest.ActivityID)
	}

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("follow request accepted",
		zap.String("requester_id", cmd.RequesterID),
		zap.String("follower_id", cmd.FollowerID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// RejectFollowRequest rejects a pending follow request
func (s *Service) RejectFollowRequest(ctx context.Context, cmd *RejectFollowRequestCommand) (*RelationshipResult, error) {
	// Validate command
	if err := s.validateRejectFollowRequestCommand(cmd); err != nil {
		return nil, err
	}

	cmd.RequesterID = s.normalizeActorIdentifier(cmd.RequesterID)
	cmd.FollowerID = s.normalizeActorIdentifier(cmd.FollowerID)

	s.logger.Info("rejecting follow request",
		zap.String("requester_id", cmd.RequesterID),
		zap.String("follower_id", cmd.FollowerID))

	// Get relationship repository from storage
	if s.storage == nil {
		return nil, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, RepositoryNotAvailable("general")
	}

	// Check if the follow request exists.
	followRequest, err := relationshipRepo.GetFollowRequest(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	// Reject the follow request
	err = relationshipRepo.RejectFollowRequest(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	// Resolve actors for later federation work without forcing remote followers through local-only lookup.
	var follower, following *storage.Account
	if s.storage != nil {
		follower, err = s.resolveFollowDecisionAccount(ctx, cmd.FollowerID, false)
		if err != nil {
			s.logger.Warn("failed to resolve follower for rejection", zap.String("follower_id", cmd.FollowerID), zap.Error(err))
		}

		following, err = s.resolveFollowDecisionAccount(ctx, cmd.RequesterID, true)
		if err != nil {
			s.logger.Warn("failed to resolve requester for rejection", zap.String("requester_id", cmd.RequesterID), zap.Error(err))
		}
	}

	// Emit minimal events and queue federation rejection
	var events []*streaming.Event
	if follower != nil && following != nil {
		// Queue rejection activity for federation
		s.queueFederationReject(ctx, follower, following, followRequest.ActivityID)
	}

	// Get updated relationship data
	relationship, err := s.GetRelationship(ctx, cmd.FollowerID, cmd.RequesterID)
	if err != nil {
		return nil, err
	}

	s.logger.Info("follow request rejected",
		zap.String("requester_id", cmd.RequesterID),
		zap.String("follower_id", cmd.FollowerID))

	return &RelationshipResult{
		Relationship: relationship,
		Events:       events,
	}, nil
}

// Private helper methods

func (s *Service) validateAddDomainBlockCommand(cmd *AddDomainBlockCommand) error {
	if err := common.ValidateRequiredParam("user_id", cmd.UserID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("domain", cmd.Domain); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateRemoveDomainBlockCommand(cmd *RemoveDomainBlockCommand) error {
	if err := common.ValidateRequiredParam("user_id", cmd.UserID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("domain", cmd.Domain); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateGetDomainBlocksQuery(query *GetDomainBlocksQuery) error {
	if err := common.ValidateRequiredParam("user_id", query.UserID); err != nil {
		return err
	}
	if query.Limit <= 0 {
		query.Limit = 100 // Default limit
	}
	return nil
}

func (s *Service) validateGetMutedUsersQuery(query *GetMutedUsersQuery) error {
	if err := common.ValidateRequiredParam("user_id", query.UserID); err != nil {
		return err
	}
	if query.Limit <= 0 {
		query.Limit = 40 // Default limit
	}
	return nil
}

func (s *Service) validateGetBlockedUsersQuery(query *GetBlockedUsersQuery) error {
	if err := common.ValidateRequiredParam("user_id", query.UserID); err != nil {
		return err
	}
	if query.Limit <= 0 {
		query.Limit = 40 // Default limit
	}
	return nil
}

func (s *Service) validateGetFollowRequestsQuery(query *GetFollowRequestsQuery) error {
	if err := common.ValidateRequiredParam("user_id", query.UserID); err != nil {
		return err
	}
	if query.Limit <= 0 {
		query.Limit = 100 // Default limit
	}
	return nil
}

func (s *Service) validateAcceptFollowRequestCommand(cmd *AcceptFollowRequestCommand) error {
	if err := common.ValidateRequiredParam("requester_id", cmd.RequesterID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("follower_id", cmd.FollowerID); err != nil {
		return err
	}
	if cmd.RequesterID == cmd.FollowerID {
		return CannotAcceptOwnFollowRequest()
	}
	return nil
}

func (s *Service) validateRejectFollowRequestCommand(cmd *RejectFollowRequestCommand) error {
	if err := common.ValidateRequiredParam("requester_id", cmd.RequesterID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("follower_id", cmd.FollowerID); err != nil {
		return err
	}
	if cmd.RequesterID == cmd.FollowerID {
		return CannotRejectOwnFollowRequest()
	}
	return nil
}

func (s *Service) validateFollowCommand(_ context.Context, cmd *FollowCommand) error {
	if err := common.ValidateRequiredParam("follower_id", cmd.FollowerID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("following_id", cmd.FollowingID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateUnfollowCommand(_ context.Context, cmd *UnfollowCommand) error {
	if err := common.ValidateRequiredParam("follower_id", cmd.FollowerID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("following_id", cmd.FollowingID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateBlockCommand(_ context.Context, cmd *BlockCommand) error {
	if err := common.ValidateRequiredParam("blocker_id", cmd.BlockerID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("blocked_id", cmd.BlockedID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateUnblockCommand(_ context.Context, cmd *UnblockCommand) error {
	if err := common.ValidateRequiredParam("blocker_id", cmd.BlockerID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("blocked_id", cmd.BlockedID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateMuteCommand(_ context.Context, cmd *MuteCommand) error {
	if err := common.ValidateRequiredParam("muter_id", cmd.MuterID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("muted_id", cmd.MutedID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateUnmuteCommand(_ context.Context, cmd *UnmuteCommand) error {
	if err := common.ValidateRequiredParam("muter_id", cmd.MuterID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("muted_id", cmd.MutedID); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateGetRelationshipsQuery(query *GetRelationshipsQuery) error {
	if err := common.ValidateRequiredParam("requester_id", query.RequesterID); err != nil {
		return err
	}
	if err := common.ValidateSliceNotEmpty("query.TargetIDs", query.TargetIDs); err != nil {
		return TargetIDsEmpty()
	}
	if len(query.TargetIDs) > 40 {
		return TooManyTargetIDs(len(query.TargetIDs))
	}
	return nil
}

func (s *Service) buildRelationshipData(ctx context.Context, requesterID, targetID string) (*RelationshipData, error) {
	requesterID = s.normalizeActorIdentifier(requesterID)
	targetID = s.normalizeActorIdentifier(targetID)

	now := time.Now()

	// Initialize with defaults
	data := &RelationshipData{
		ID:        targetID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Prefer storage-backed repositories when available (new architecture).
	if s.storage != nil {
		return s.enrichRelationshipDataFromStorage(ctx, data, requesterID, targetID)
	}

	// Legacy (interface-based) repositories: best-effort population without requiring full storage.
	return s.enrichRelationshipDataFromLegacy(ctx, data, requesterID, targetID)
}

func (s *Service) enrichRelationshipDataFromStorage(ctx context.Context, data *RelationshipData, requesterID, targetID string) (*RelationshipData, error) {
	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return data, nil // Return default data if repo not available
	}

	// Get follow status (requester -> target)
	following, err := relationshipRepo.IsFollowing(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check following status", zap.Error(err))
	} else {
		data.Following = following
	}

	// Get follow status (target -> requester)
	followedBy, err := relationshipRepo.IsFollowing(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check followed_by status", zap.Error(err))
	} else {
		data.FollowedBy = followedBy
	}

	// Get block status (requester -> target)
	blocking, err := relationshipRepo.IsBlocked(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check blocking status", zap.Error(err))
	} else {
		data.Blocking = blocking
	}

	// Get block status (target -> requester)
	blockedBy, err := relationshipRepo.IsBlocked(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check blocked_by status", zap.Error(err))
	} else {
		data.BlockedBy = blockedBy
	}

	// Get mute status using SocialRepository
	if socialRepo := s.storage.Social(); socialRepo != nil {
		muting, err := socialRepo.IsMuted(ctx, requesterID, targetID)
		if err != nil {
			s.logger.Warn("failed to check muting status", zap.Error(err))
		} else {
			data.Muting = muting
		}
	}

	// Get follow request status by checking if there's a pending request
	hasPendingRequest, err := relationshipRepo.HasPendingFollowRequest(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check follow request status", zap.Error(err))
	} else {
		data.Requested = hasPendingRequest
	}

	// Get reverse follow request status
	hasReversePendingRequest, err := relationshipRepo.HasPendingFollowRequest(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check reverse follow request status", zap.Error(err))
	} else {
		data.RequestedBy = hasReversePendingRequest
	}

	// Get relationship preferences if following
	if data.Following {
		relationship, err := relationshipRepo.GetRelationship(ctx, requesterID, targetID)
		if err != nil {
			s.logger.Warn("failed to get relationship details", zap.Error(err))
		} else if relationship != nil {
			data.Notifying = relationship.Notifying
			data.ShowingReblogs = relationship.ShowingReblogs
			data.Languages = relationship.Languages
			data.Note = relationship.Note
			data.CreatedAt = relationship.CreatedAt
			data.UpdatedAt = relationship.UpdatedAt
		}
	}

	return data, nil
}

func (s *Service) enrichRelationshipDataFromLegacy(ctx context.Context, data *RelationshipData, requesterID, targetID string) (*RelationshipData, error) {
	if s.relationshipRepo == nil {
		return data, nil
	}

	following, err := s.relationshipRepo.IsFollowing(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check following status", zap.Error(err))
	} else {
		data.Following = following
	}

	followedBy, err := s.relationshipRepo.IsFollowing(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check followed_by status", zap.Error(err))
	} else {
		data.FollowedBy = followedBy
	}

	blocking, err := s.relationshipRepo.IsBlocked(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check blocking status", zap.Error(err))
	} else {
		data.Blocking = blocking
	}

	blockedBy, err := s.relationshipRepo.IsBlocked(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check blocked_by status", zap.Error(err))
	} else {
		data.BlockedBy = blockedBy
	}

	muting, err := s.relationshipRepo.IsMuted(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check muting status", zap.Error(err))
	} else {
		data.Muting = muting
	}

	status, err := s.relationshipRepo.GetFollowStatus(ctx, requesterID, targetID)
	if err != nil {
		s.logger.Warn("failed to check follow request status", zap.Error(err))
	} else {
		data.Requested = strings.EqualFold(status, "pending")
	}

	reverseStatus, err := s.relationshipRepo.GetFollowStatus(ctx, targetID, requesterID)
	if err != nil {
		s.logger.Warn("failed to check reverse follow request status", zap.Error(err))
	} else {
		data.RequestedBy = strings.EqualFold(reverseStatus, "pending")
	}

	if data.Following {
		relationship, err := s.relationshipRepo.GetFollowRelationship(ctx, requesterID, targetID)
		if err != nil {
			s.logger.Warn("failed to get relationship details", zap.Error(err))
		} else if relationship != nil {
			data.Notifying = relationship.Notifying
			data.ShowingReblogs = relationship.ShowingReblogs
			data.Languages = relationship.Languages
			data.Note = relationship.Note
			data.CreatedAt = relationship.CreatedAt
			data.UpdatedAt = relationship.UpdatedAt
		}
	}

	return data, nil
}

// normalizeActorIdentifier maps ActivityPub IDs or URLs to canonical usernames/handles used in Dynamo keys.
// Local actors are reduced to their username, while remote actors retain their domain via username@domain.
func (s *Service) normalizeActorIdentifier(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return ""
	}

	if normalized := models.NormalizeRelationshipIdentity(trimmed, s.domainName); normalized != "" {
		return normalized
	}

	// Already a username or handle
	if !strings.Contains(trimmed, "://") {
		return trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return trimmed
	}

	segments := strings.Split(path, "/")
	username := strings.TrimSpace(segments[len(segments)-1])
	username = strings.TrimSuffix(username, ".json")
	if username == "" {
		return trimmed
	}

	localDomain := strings.ToLower(strings.TrimSpace(s.domainName))
	if localDomain != "" {
		if idx := strings.Index(localDomain, ":"); idx >= 0 {
			localDomain = localDomain[:idx]
		}
	}

	if host == "" || (localDomain != "" && host == localDomain) {
		return username
	}

	return fmt.Sprintf("%s@%s", username, host)
}

func (s *Service) buildAccountFromActor(ctx context.Context, actor *activitypub.Actor, fallbackUsername string) *storage.Account {
	if actor == nil {
		return nil
	}

	identity := federation.DescribeActorIdentity(actor, s.domainName)
	username, userLookupUsername, userUsername := relationshipAccountUsernames(actor, fallbackUsername, identity)
	storedUser := s.loadRelationshipStoredUser(ctx, userLookupUsername)

	if identity.IsRemote {
		return buildRemoteRelationshipAccount(actor, username, userUsername, storedUser)
	}

	return buildLocalRelationshipAccount(actor, s.baseURL(), username, userUsername, storedUser)
}

type relationshipAccountProfile struct {
	displayName string
	note        string
	avatar      string
	header      string
	createdAt   time.Time
	updatedAt   time.Time
}

func relationshipAccountUsernames(actor *activitypub.Actor, fallbackUsername string, identity federation.ActorIdentity) (string, string, string) {
	username := strings.TrimSpace(identity.Username)
	if username == "" {
		username = activitypubutil.DerivePreferredUsername(actor, fallbackUsername)
	}

	username = strings.TrimSpace(username)
	if username == "" {
		username = strings.TrimSpace(fallbackUsername)
	}

	userLookupUsername := username
	userUsername := username
	if identity.IsRemote && strings.TrimSpace(identity.Acct) != "" {
		userLookupUsername = strings.TrimSpace(identity.Acct)
		userUsername = userLookupUsername
	}

	return username, userLookupUsername, userUsername
}

func (s *Service) loadRelationshipStoredUser(ctx context.Context, username string) *storage.User {
	if s.storage == nil || username == "" {
		return nil
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return nil
	}

	fetched, err := userRepo.GetUser(ctx, username)
	if err == nil && fetched != nil {
		copied := *fetched
		return &copied
	}
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		s.logger.Debug("failed to hydrate relationship account from user repository",
			zap.String("username", username),
			zap.Error(err))
	}

	return nil
}

func buildRemoteRelationshipAccount(actor *activitypub.Actor, username, userUsername string, storedUser *storage.User) *storage.Account {
	if actor == nil {
		return nil
	}

	actorCopy := *actor
	if strings.TrimSpace(actorCopy.URL) == "" {
		actorCopy.URL = strings.TrimSpace(actorCopy.ID)
	}

	profile := relationshipActorProfile(&actorCopy, storedUser, username)
	return &storage.Account{
		User: &storage.User{
			ID:           actorCopy.ID,
			Username:     userUsername,
			DisplayName:  profile.displayName,
			Note:         profile.note,
			Avatar:       profile.avatar,
			Header:       profile.header,
			URL:          actorCopy.URL,
			Locked:       actorCopy.ManuallyApprovesFollowers,
			Discoverable: actorCopy.Discoverable,
			CreatedAt:    profile.createdAt,
			UpdatedAt:    profile.updatedAt,
		},
		Actor: &actorCopy,
	}
}

func buildLocalRelationshipAccount(actor *activitypub.Actor, baseURL, username, userUsername string, storedUser *storage.User) *storage.Account {
	actorCopy := activitypubutil.BuildLocalActor(username, baseURL, storedUser, actor)
	if actorCopy == nil {
		return nil
	}

	profile := relationshipActorProfile(actorCopy, storedUser, username)
	user := &storage.User{
		ID:           actorCopy.ID,
		Username:     userUsername,
		DisplayName:  profile.displayName,
		Note:         profile.note,
		Avatar:       profile.avatar,
		Header:       profile.header,
		URL:          actorCopy.URL,
		Locked:       actorCopy.ManuallyApprovesFollowers,
		Discoverable: actorCopy.Discoverable,
		CreatedAt:    profile.createdAt,
		UpdatedAt:    profile.updatedAt,
	}
	applyStoredUserToRelationshipUser(user, storedUser)

	return &storage.Account{
		User:  user,
		Actor: actorCopy,
	}
}

func relationshipActorProfile(actor *activitypub.Actor, storedUser *storage.User, fallbackDisplay string) relationshipAccountProfile {
	profile := relationshipAccountProfile{}
	if actor == nil {
		return profile
	}

	profile.displayName = strings.TrimSpace(actor.Name)
	if profile.displayName == "" && storedUser != nil {
		profile.displayName = strings.TrimSpace(storedUser.DisplayName)
	}
	if profile.displayName == "" {
		profile.displayName = strings.TrimSpace(fallbackDisplay)
	}

	profile.note = strings.TrimSpace(actor.Summary)
	if profile.note == "" && storedUser != nil {
		profile.note = strings.TrimSpace(storedUser.Note)
	}

	if actor.Icon != nil {
		profile.avatar = strings.TrimSpace(actor.Icon.URL)
	}
	if profile.avatar == "" && storedUser != nil {
		profile.avatar = strings.TrimSpace(storedUser.Avatar)
	}

	if actor.Image != nil {
		profile.header = strings.TrimSpace(actor.Image.URL)
	}
	if profile.header == "" && storedUser != nil {
		profile.header = strings.TrimSpace(storedUser.Header)
	}

	if storedUser != nil {
		profile.createdAt = storedUser.CreatedAt
		profile.updatedAt = storedUser.UpdatedAt
		return profile
	}

	if actor.Published != nil {
		profile.createdAt = *actor.Published
	}
	if actor.Updated != nil {
		profile.updatedAt = *actor.Updated
	}

	return profile
}

func applyStoredUserToRelationshipUser(user, storedUser *storage.User) {
	if user == nil || storedUser == nil {
		return
	}

	user.ID = storedUser.ID
	user.Email = storedUser.Email
	user.URL = storedUser.URL
	user.Metadata = storedUser.Metadata
	user.Fields = storedUser.Fields
	user.Approved = storedUser.Approved
	user.Suspended = storedUser.Suspended
	user.Silenced = storedUser.Silenced
	user.Role = storedUser.Role
	user.Locale = storedUser.Locale
	user.RecoveryMethods = storedUser.RecoveryMethods
	user.AllowNSFW = storedUser.AllowNSFW
	user.RequireNSFWWarning = storedUser.RequireNSFWWarning
	user.CreatedAt = storedUser.CreatedAt
	user.UpdatedAt = storedUser.UpdatedAt
	if strings.TrimSpace(storedUser.DisplayName) != "" {
		user.DisplayName = storedUser.DisplayName
	}
	if strings.TrimSpace(storedUser.Note) != "" {
		user.Note = storedUser.Note
	}
	if strings.TrimSpace(storedUser.Avatar) != "" {
		user.Avatar = storedUser.Avatar
	}
	if strings.TrimSpace(storedUser.Header) != "" {
		user.Header = storedUser.Header
	}
	user.Locked = storedUser.Locked
	user.Discoverable = storedUser.Discoverable
}

func (s *Service) baseURL() string {
	domain := strings.TrimSpace(s.domainName)
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return strings.TrimSuffix(domain, "/")
	}
	return "https://" + strings.TrimSuffix(domain, "/")
}

// Event emission methods

// emitRelationshipEvents emits events for relationship changes
func (s *Service) emitRelationshipEvents(ctx context.Context, follower, following *storage.Account, activityID string, eventType string, actionName string) []*streaming.Event {
	var events []*streaming.Event

	// Event to follower's stream
	followerEvent := streaming.NewEvent(eventType).
		ForStream(streaming.UserStreamName(follower.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, follower.User.Username, followerEvent); err != nil {
		s.logger.Error(fmt.Sprintf("failed to publish %s to follower stream", actionName), zap.Error(err))
	} else {
		events = append(events, followerEvent)
	}

	// Event to following user's stream (notification)
	followingEvent := streaming.NewEvent(eventType).
		ForStream(streaming.UserStreamName(following.User.Username)).
		WithData("actor_id", follower.User.Username).
		WithData("target_id", following.User.Username).
		WithData("activity_id", activityID).
		Build()

	if err := s.publisher.PublishToUser(ctx, following.User.Username, followingEvent); err != nil {
		s.logger.Error(fmt.Sprintf("failed to publish %s to following stream", actionName), zap.Error(err))
	} else {
		events = append(events, followingEvent)
	}

	return events
}

func (s *Service) emitFollowRequestedEvents(ctx context.Context, follower, following *storage.Account, activityID string) []*streaming.Event {
	return s.emitRelationshipEvents(ctx, follower, following, activityID, streaming.RelationshipFollowRequested, "follow request")
}

func (s *Service) emitFollowAcceptedEvents(ctx context.Context, follower, following *storage.Account, activityID string) []*streaming.Event {
	return s.emitRelationshipEvents(ctx, follower, following, activityID, streaming.RelationshipFollowAccepted, "follow accepted")
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

// queueFederationFollow queues a follow activity for federation
func (s *Service) queueFederationFollow(ctx context.Context, activity *activitypub.Activity, follower, following *storage.Account, actionType string) {
	if isNilFederationService(s.federation) {
		s.logger.Debug(fmt.Sprintf("federation service not available, skipping %s", actionType))
		return
	}

	if strings.TrimSpace(s.domainName) == "" {
		s.logger.Debug(fmt.Sprintf("domain name not configured, skipping %s", actionType))
		return
	}

	// Only federate to remote users
	if following.Actor == nil || isLocalActor(following.Actor, s.domainName) {
		return
	}
	if activity == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn(fmt.Sprintf("federation queue panic suppressed during %s", actionType),
				zap.String("follower", follower.User.Username),
				zap.String("following", following.User.Username),
				zap.Any("reason", r))
		}
	}()

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error(fmt.Sprintf("failed to queue federation %s", actionType),
			zap.String("follower", follower.User.Username),
			zap.String("following", following.User.Username),
			zap.Error(err))
	}
}

func (s *Service) queueFederationFollowRequest(ctx context.Context, activity *activitypub.Activity, follower, following *storage.Account) {
	s.queueFederationFollow(ctx, activity, follower, following, "follow request")
}

func (s *Service) queueFederationFollowDirectly(ctx context.Context, activity *activitypub.Activity, follower, following *storage.Account) {
	s.queueFederationFollow(ctx, activity, follower, following, "follow")
}

func (s *Service) queueFederationAccept(ctx context.Context, follower, following *storage.Account, originalActivityID string) {
	s.queueFederationFollowResponse(ctx, activitypub.AcceptType, follower, following, originalActivityID)
}

func (s *Service) queueFederationBlock(ctx context.Context, blocker, blocked *storage.Account) {
	if isNilFederationService(s.federation) {
		s.logger.Debug("federation service not available, skipping block")
		return
	}

	if strings.TrimSpace(s.domainName) == "" {
		s.logger.Debug("domain name not configured, skipping block")
		return
	}

	// Only federate to remote users
	if blocked.Actor == nil || isLocalActor(blocked.Actor, s.domainName) {
		return
	}

	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      "Block",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, uuid.New().String()),
			Published: &now,
		},
		Actor:  blocker.Actor.ID,
		Object: blocked.Actor.ID,
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("federation queue panic suppressed during block",
				zap.String("blocker", blocker.User.Username),
				zap.String("blocked", blocked.User.Username),
				zap.Any("reason", r))
		}
	}()

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation block",
			zap.String("blocker", blocker.User.Username),
			zap.String("blocked", blocked.User.Username),
			zap.Error(err))
	}
}

func (s *Service) queueFederationUndo(ctx context.Context, actor, target *storage.Account, activityType string) {
	if isNilFederationService(s.federation) {
		s.logger.Debug("federation service not available, skipping undo")
		return
	}

	if strings.TrimSpace(s.domainName) == "" {
		s.logger.Debug("domain name not configured, skipping undo")
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
			Context: activitypub.Context,
			Type:    activityType,
			ID:      fmt.Sprintf("https://%s/activities/%s", s.domainName, uuid.New().String()),
		},
		Actor:  actor.Actor.ID,
		Object: target.Actor.ID,
	}

	// Create the Undo activity
	undoActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      "Undo",
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, undoID),
			Published: &now,
		},
		Actor:  actor.Actor.ID,
		Object: originalActivity,
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("federation queue panic suppressed during undo",
				zap.String("actor", actor.User.Username),
				zap.String("target", target.User.Username),
				zap.Any("reason", r))
		}
	}()

	if err := s.federation.QueueActivity(ctx, undoActivity); err != nil {
		s.logger.Error("failed to queue federation undo",
			zap.String("actor", actor.User.Username),
			zap.String("target", target.User.Username),
			zap.String("activity_type", activityType),
			zap.Error(err))
	}
}

func (s *Service) queueFederationReject(ctx context.Context, follower, following *storage.Account, originalActivityID string) {
	s.queueFederationFollowResponse(ctx, activitypub.RejectType, follower, following, originalActivityID)
}

func (s *Service) queueFederationFollowResponse(ctx context.Context, activityType string, follower, following *storage.Account, originalActivityID string) {
	if isNilFederationService(s.federation) {
		s.logger.Debug("federation service not available, skipping follow response",
			zap.String("activity_type", activityType))
		return
	}

	if strings.TrimSpace(s.domainName) == "" {
		s.logger.Debug("domain name not configured, skipping follow response",
			zap.String("activity_type", activityType))
		return
	}

	// Manual follow responses go to the remote follower.
	if follower.Actor == nil || isLocalActor(follower.Actor, s.domainName) {
		return
	}
	if following.Actor == nil || strings.TrimSpace(following.Actor.ID) == "" {
		return
	}

	now := time.Now()
	activityID := uuid.New().String()

	responseActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activityType,
			ID:        fmt.Sprintf("https://%s/activities/%s", s.domainName, activityID),
			Published: &now,
			To:        []string{follower.Actor.ID},
		},
		Actor:  following.Actor.ID,
		Object: followResponseObjectID(originalActivityID, follower, following),
	}

	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("federation queue panic suppressed during follow response",
				zap.String("follower", follower.User.Username),
				zap.String("following", following.User.Username),
				zap.String("activity_type", activityType),
				zap.Any("reason", r))
		}
	}()

	if err := s.federation.QueueActivity(ctx, responseActivity); err != nil {
		s.logger.Error("failed to queue federation follow response",
			zap.String("follower", follower.User.Username),
			zap.String("following", following.User.Username),
			zap.String("activity_type", activityType),
			zap.Error(err))
	}
}

func followResponseObjectID(originalActivityID string, follower, following *storage.Account) string {
	if trimmed := strings.TrimSpace(originalActivityID); trimmed != "" {
		return trimmed
	}

	baseActorID := ""
	if follower != nil && follower.Actor != nil {
		baseActorID = strings.TrimRight(strings.TrimSpace(follower.Actor.ID), "/")
	}
	if baseActorID == "" && following != nil && following.Actor != nil {
		baseActorID = strings.TrimRight(strings.TrimSpace(following.Actor.ID), "/")
	}

	followingSlug := ""
	switch {
	case following != nil && following.User != nil && strings.TrimSpace(following.User.Username) != "":
		followingSlug = strings.TrimSpace(following.User.Username)
	case following != nil && following.Actor != nil && strings.TrimSpace(following.Actor.PreferredUsername) != "":
		followingSlug = strings.TrimSpace(following.Actor.PreferredUsername)
	default:
		followingSlug = uuid.New().String()
	}

	if baseActorID == "" {
		return fmt.Sprintf("https://example.invalid/activities/%s", uuid.New().String())
	}

	return fmt.Sprintf("%s/follows/%s", baseActorID, followingSlug)
}

// isLocalActor checks if an actor is local to this instance
func isLocalActor(actor *activitypub.Actor, domainName string) bool {
	if actor == nil {
		return false
	}
	// Check if the actor ID contains our domain
	return strings.Contains(actor.ID, domainName)
}

func isNilFederationService(f FederationService) bool {
	if f == nil {
		return true
	}

	value := reflect.ValueOf(f)
	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		return value.IsNil()
	default:
		return false
	}
}

func (s *Service) auditRelationshipEvent(ctx context.Context, eventType, actorID, targetID string, success bool, failureReason string, metadata map[string]any) {
	if s == nil || s.storage == nil || s.storage.Audit() == nil {
		return
	}

	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["actor_id"] = actorID
	metadata["target_id"] = targetID

	severity := "MEDIUM"
	if !success {
		severity = "HIGH"
	}

	if err := s.storage.Audit().StoreAuditEvent(ctx, eventType, severity, actorID, actorID, "", "", "", "", "", success, failureReason, metadata); err != nil {
		s.logger.Debug("failed to store audit event",
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

// getRelationshipRepo returns an object that has the basic relationship query and delete methods
func (s *Service) getRelationshipRepo() interface {
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
	Unfollow(ctx context.Context, followerID, followingID string) error
	IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error)
	BlockUser(ctx context.Context, blockerID, blockedID string) error
	UnblockUser(ctx context.Context, blockerID, blockedID string) error
	IsMuted(ctx context.Context, muterID, mutedID string) (bool, error)
	UnmuteUser(ctx context.Context, muterID, mutedID string) error
} {
	if s.relationshipRepo != nil {
		return s.relationshipRepo
	}
	if s.storage != nil {
		return s.storage.Relationship()
	}
	return nil
}

// IsBlocked checks if one user has blocked another
func (s *Service) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	// Check block status through relationship repository
	if s.storage == nil {
		return false, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return false, RepositoryNotAvailable("general")
	}

	isBlocked, err := relationshipRepo.IsBlocked(ctx, blockerID, blockedID)
	if err != nil {
		return false, err
	}

	return isBlocked, nil
}

// CountFollowing counts the number of users that an actor is following
func (s *Service) CountFollowing(ctx context.Context, username string) (int64, error) {
	// Count following through relationship repository
	if s.storage == nil {
		return 0, StorageNotAvailable()
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return 0, RepositoryNotAvailable("general")
	}

	count, err := relationshipRepo.CountFollowing(ctx, username)
	if err != nil {
		return 0, err
	}

	return int64(count), nil
}

// Severance-related types and methods

// AcknowledgeSeveranceCommand contains data needed to acknowledge a severance
type AcknowledgeSeveranceCommand struct {
	UserID      string `json:"user_id" validate:"required"`
	SeveranceID string `json:"severance_id" validate:"required"`
}

// AcknowledgeSeveranceResult contains the result of acknowledging a severance
type AcknowledgeSeveranceResult struct {
	Success bool               `json:"success"`
	Events  []*streaming.Event `json:"events"`
}

// GetAffectedRelationshipsQuery contains data needed to get affected relationships
type GetAffectedRelationshipsQuery struct {
	UserID                string `json:"user_id" validate:"required"`
	SeveredRelationshipID string `json:"severed_relationship_id" validate:"required"`
}

// AffectedRelationship represents a relationship affected by severance
type AffectedRelationship struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	AffectedUser storage.User `json:"affected_user"`
}

// GetAffectedRelationshipsResult contains the result of getting affected relationships
type GetAffectedRelationshipsResult struct {
	Relationships   []*AffectedRelationship `json:"relationships"`
	HasNextPage     bool                    `json:"has_next_page"`
	HasPreviousPage bool                    `json:"has_previous_page"`
	Events          []*streaming.Event      `json:"events"`
}

// AcknowledgeSeverance acknowledges a relationship severance
func (s *Service) AcknowledgeSeverance(ctx context.Context, cmd *AcknowledgeSeveranceCommand) (*AcknowledgeSeveranceResult, error) {
	s.logger.Info("acknowledging severance",
		zap.String("user_id", cmd.UserID),
		zap.String("severance_id", cmd.SeveranceID))

	// Validate the command
	if err := s.validateAcknowledgeSeveranceCommand(cmd); err != nil {
		return nil, err
	}

	// For now, we'll implement basic acknowledgment
	// In a full implementation, this would update severance records in storage

	// Emit acknowledgment events
	events := s.emitSeveranceAcknowledgedEvents(ctx, cmd.UserID, cmd.SeveranceID)

	s.logger.Info("acknowledged severance successfully",
		zap.String("user_id", cmd.UserID),
		zap.String("severance_id", cmd.SeveranceID))

	return &AcknowledgeSeveranceResult{
		Success: true,
		Events:  events,
	}, nil
}

// GetAffectedRelationships retrieves relationships affected by a severance
func (s *Service) GetAffectedRelationships(_ context.Context, query *GetAffectedRelationshipsQuery) (*GetAffectedRelationshipsResult, error) {
	s.logger.Info("getting affected relationships",
		zap.String("user_id", query.UserID),
		zap.String("severed_relationship_id", query.SeveredRelationshipID))

	// Validate the query
	if err := s.validateGetAffectedRelationshipsQuery(query); err != nil {
		return nil, err
	}

	// For now, we'll return empty results
	// In a full implementation, this would query storage for affected relationships

	s.logger.Info("retrieved affected relationships successfully",
		zap.String("user_id", query.UserID),
		zap.String("severed_relationship_id", query.SeveredRelationshipID))

	return &GetAffectedRelationshipsResult{
		Relationships:   []*AffectedRelationship{},
		HasNextPage:     false,
		HasPreviousPage: false,
		Events:          []*streaming.Event{},
	}, nil
}

// Validation methods

func (s *Service) validateAcknowledgeSeveranceCommand(cmd *AcknowledgeSeveranceCommand) error {
	if err := common.ValidateRequiredParam("user_id", strings.TrimSpace(cmd.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("severance_id", strings.TrimSpace(cmd.SeveranceID)); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateGetAffectedRelationshipsQuery(query *GetAffectedRelationshipsQuery) error {
	if err := common.ValidateRequiredParam("user_id", strings.TrimSpace(query.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("severed_relationship_id", strings.TrimSpace(query.SeveredRelationshipID)); err != nil {
		return err
	}
	return nil
}

// Event emission methods

func (s *Service) emitSeveranceAcknowledgedEvents(ctx context.Context, userID, severanceID string) []*streaming.Event {
	var events []*streaming.Event

	event := &streaming.Event{
		Type:      "severance.acknowledged",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id":      userID,
			"severance_id": severanceID,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", userID)
	if err := s.publisher.PublishToUser(ctx, userID, &userEvent); err != nil {
		s.logger.Error("failed to publish severance acknowledged event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}
