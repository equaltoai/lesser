package services

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
)

// ServiceConfig holds configuration for all services
type ServiceConfig struct {
	BaseURL   string
	JWTSecret string
	Config    *config.Config // Add reference to full config
}

// ServiceDependencies provides access to repositories and external services
type ServiceDependencies struct {
	Repos  interface{} // Either storage.Storage or core.RepositoryStorage
	Config *ServiceConfig
	Logger interface{} // zap.Logger interface compatible
}

// CreatePostInput standardizes post creation across REST and GraphQL
type CreatePostInput struct {
	Content     string
	Visibility  string
	Sensitive   bool
	SpoilerText string
	InReplyToID string
	MediaIDs    []string
	ScheduledAt *string
	Language    string
}

// CreatePostResult contains the result of post creation
type CreatePostResult struct {
	Activity     *activitypub.Activity
	Note         *activitypub.Note
	Actor        *activitypub.Actor
	Poll         interface{} // Can be *models.Poll or other poll types
	ParsedEmojis interface{} // Platform-specific emoji data
}

// FollowInput standardizes follow operations
type FollowInput struct {
	TargetActorID string
	ShowReblogs   bool
	Notify        bool
}

// FollowResult contains the result of follow operations
type FollowResult struct {
	Activity  *activitypub.Activity
	Following bool
	Requested bool // For locked accounts
}

// LikeInput standardizes like operations
type LikeInput struct {
	ObjectID string
}

// LikeResult contains the result of like operations
type LikeResult struct {
	Activity *activitypub.Activity
	Liked    bool
}

// DeletePostInput standardizes post deletion
type DeletePostInput struct {
	ObjectID string
}

// DeletePostResult contains the result of post deletion
type DeletePostResult struct {
	Activity *activitypub.Activity
	Deleted  bool
}

// UpdatePostInput standardizes post updates
type UpdatePostInput struct {
	ObjectID    string
	Content     string
	SpoilerText string
	Sensitive   bool
	Visibility  string
	Language    string
}

// UpdatePostResult contains the result of post updates
type UpdatePostResult struct {
	Activity *activitypub.Activity
	Note     *activitypub.Note
}

// UserContext provides authenticated user information
type UserContext struct {
	Username string
	ActorID  string
	Claims   *auth.Claims
}

// ServiceError provides consistent error handling across services
type ServiceError struct {
	Code    string
	Message string
	Status  int
	Cause   error
}

func (e *ServiceError) Error() string {
	return e.Message
}

// NewValidationError creates a validation error with status 400
func NewValidationError(message string) *ServiceError {
	return &ServiceError{
		Code:    "VALIDATION_ERROR",
		Message: message,
		Status:  400,
	}
}

// NewUnauthorizedError creates an authorization error with status 401
func NewUnauthorizedError(message string) *ServiceError {
	return &ServiceError{
		Code:    "UNAUTHORIZED",
		Message: message,
		Status:  401,
	}
}

// NewForbiddenError creates a forbidden error with status 403
func NewForbiddenError(message string) *ServiceError {
	return &ServiceError{
		Code:    "FORBIDDEN",
		Message: message,
		Status:  403,
	}
}

// NewNotFoundError creates a not found error with status 404
func NewNotFoundError(message string) *ServiceError {
	return &ServiceError{
		Code:    "NOT_FOUND",
		Message: message,
		Status:  404,
	}
}

// NewInternalError creates an internal error with status 500
func NewInternalError(message string, cause error) *ServiceError {
	return &ServiceError{
		Code:    "INTERNAL_ERROR",
		Message: message,
		Status:  500,
		Cause:   cause,
	}
}

// BusinessLogicService defines the interface for all business logic services
type BusinessLogicService interface {
	// Post operations
	CreatePost(ctx context.Context, user *UserContext, input *CreatePostInput) (*CreatePostResult, error)
	DeletePost(ctx context.Context, user *UserContext, input *DeletePostInput) (*DeletePostResult, error)
	UpdatePost(ctx context.Context, user *UserContext, input *UpdatePostInput) (*UpdatePostResult, error)

	// Social operations
	FollowActor(ctx context.Context, user *UserContext, input *FollowInput) (*FollowResult, error)
	UnfollowActor(ctx context.Context, user *UserContext, targetActorID string) (*FollowResult, error)

	// Interaction operations
	LikeObject(ctx context.Context, user *UserContext, input *LikeInput) (*LikeResult, error)
	UnlikeObject(ctx context.Context, user *UserContext, objectID string) (*LikeResult, error)

	// Timeline operations
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error

	// Federation operations
	DeliverActivity(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, visibility string) error
}

// ValidationService handles input validation
type ValidationService interface {
	ValidateCreatePost(input *CreatePostInput) error
	ValidateFollowInput(input *FollowInput) error
	ValidateLikeInput(input *LikeInput) error
	ValidateDeletePost(input *DeletePostInput) error
	ValidateUpdatePost(input *UpdatePostInput) error
}

// AuthenticationService handles user authentication and authorization
type AuthenticationService interface {
	AuthenticateUser(ctx context.Context, token string) (*UserContext, error)
	ValidateScope(user *UserContext, requiredScope string) error
}

// NotificationService handles notification creation and delivery
type NotificationService interface {
	CreateFollowNotification(ctx context.Context, followActivity *activitypub.Activity) error
	CreateLikeNotification(ctx context.Context, likeActivity *activitypub.Activity) error
	CreateReblogNotification(ctx context.Context, announceActivity *activitypub.Activity) error
	CreateReplyNotification(ctx context.Context, replyActivity *activitypub.Activity) error
	CreateMentionNotification(ctx context.Context, mentions []string, activity *activitypub.Activity) error
}

// FederationService handles ActivityPub federation
type FederationService interface {
	DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
	DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
	DetermineRecipients(ctx context.Context, activity *activitypub.Activity, visibility string) ([]string, error)

	// Instance relationship management
	GetInstanceRelationships(ctx context.Context, domain string) (*model.InstanceRelations, error)
}

// TimelineService handles timeline operations
type TimelineService interface {
	FanOutToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
	UpdateTimelines(ctx context.Context, activity *activitypub.Activity) error
	RemoveFromTimelines(ctx context.Context, objectID string) error
}

// AnalyticsService handles usage tracking and metrics
type AnalyticsService interface {
	RecordStatusCreation(ctx context.Context, actorID string, timestamp time.Time) error
	RecordHashtagUsage(ctx context.Context, hashtags []string, objectID, actorID string) error
	RecordLinkShare(ctx context.Context, links []string, objectID, actorID string) error
	RecordEngagement(ctx context.Context, objectID, engagementType, actorID string) error
	RecordInstanceActivity(ctx context.Context, activityType string, timestamp time.Time) error

	// Infrastructure and health monitoring
	GetInfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error)
	GetInstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error)
	GetInstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error)
}
