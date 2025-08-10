// Package accounts provides the core Accounts Service for the Lesser project's API alignment.
// This service handles all account/user profile operations including profile updates,
// preferences management, and account discovery. It emits appropriate events for real-time
// streaming and queues federation delivery for remote followers.
package accounts

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
	"go.uber.org/zap"
)

// Service provides account operations
type Service struct {
	accountRepo interfaces.AccountRepository
	publisher   streaming.Publisher
	logger      *zap.Logger
	domainName  string
	federation  FederationService // Interface to be defined
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// NewService creates a new Accounts Service with the required dependencies
func NewService(
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
		accountRepo: accountRepo,
		publisher:   publisher,
		federation:  federation,
		logger:      logger,
		domainName:  domainName,
	}
}

// Command structs for operations

// UpdateProfileCommand contains all data needed to update a user's profile
type UpdateProfileCommand struct {
	Username    string                   `json:"username" validate:"required"`
	DisplayName string                   `json:"display_name" validate:"max=100"`
	Bio         string                   `json:"bio" validate:"max=5000"`
	Avatar      string                   `json:"avatar"` // URL to avatar image
	Header      string                   `json:"header"` // URL to header image
	Locked      bool                     `json:"locked"` // Account locked (requires approval for follows)
	Bot         bool                     `json:"bot"`    // Bot account flag
	Fields      []ProfileField           `json:"fields"` // Custom profile fields
	Discoverable bool                    `json:"discoverable"` // Show in directory
	NoIndex      bool                    `json:"no_index"`     // Don't index for search
	Sensitive    bool                    `json:"sensitive"`    // Mark media as sensitive by default
	Language     string                   `json:"language"`     // Default language
	UpdaterID    string                   `json:"updater_id" validate:"required"` // Must be the account owner
}

// UpdatePreferencesCommand contains all data needed to update user preferences
type UpdatePreferencesCommand struct {
	Username                  string          `json:"username" validate:"required"`
	Language                  string          `json:"language"`
	DefaultPostingVisibility  string          `json:"default_posting_visibility" validate:"oneof=public unlisted private"`
	DefaultMediaSensitive     bool            `json:"default_media_sensitive"`
	ExpandSpoilers            bool            `json:"expand_spoilers"`
	ExpandMedia               string          `json:"expand_media" validate:"oneof=default show_all hide_all"`
	AutoplayGifs              bool            `json:"autoplay_gifs"`
	ShowFollowCounts          bool            `json:"show_follow_counts"`
	PreferredTimelineOrder    string          `json:"preferred_timeline_order" validate:"oneof=newest oldest"`
	SearchSuggestionsEnabled  bool            `json:"search_suggestions_enabled"`
	PersonalizedSearchEnabled bool            `json:"personalized_search_enabled"`
	ReblogFilters             map[string]bool `json:"reblog_filters"`
	UpdaterID                 string          `json:"updater_id" validate:"required"` // Must be the account owner
}

// GetAccountQuery contains parameters for retrieving a single account
type GetAccountQuery struct {
	Username string `json:"username" validate:"required"`
	ViewerID string `json:"viewer_id"` // User requesting the account (for privacy checks)
}

// SearchAccountsQuery contains parameters for searching accounts
type SearchAccountsQuery struct {
	Query      string                        `json:"query" validate:"required"`
	ViewerID   string                        `json:"viewer_id"` // User performing the search
	Pagination interfaces.PaginationOptions `json:"pagination"`
	Resolve    bool                          `json:"resolve"`    // Resolve remote accounts
	Following  bool                          `json:"following"`  // Only search following
	Followers  bool                          `json:"followers"`  // Only search followers
}

// ProfileField represents a custom profile field
type ProfileField struct {
	Name       string     `json:"name" validate:"required,max=255"`
	Value      string     `json:"value" validate:"required,max=255"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

// Result structs for operations

// AccountResult contains an account and associated events that were emitted
type AccountResult struct {
	Account *storage.Account  `json:"account"`
	Events  []*streaming.Event `json:"events"`
}

// PreferencesResult contains user preferences and any events
type PreferencesResult struct {
	Preferences map[string]interface{} `json:"preferences"`
	Events      []*streaming.Event      `json:"events"`
}

// AccountSearchResult contains search results and pagination information
type AccountSearchResult struct {
	Accounts   []*storage.Account                               `json:"accounts"`
	Pagination *interfaces.PaginatedResult[*storage.Account]   `json:"pagination"`
	Events     []*streaming.Event                              `json:"events"`
}

// UpdateProfile updates a user's profile, validates input, stores changes, emits events, and queues federation
func (s *Service) UpdateProfile(ctx context.Context, cmd *UpdateProfileCommand) (*AccountResult, error) {
	s.logger.Info("updating profile",
		zap.String("username", cmd.Username),
		zap.String("updater_id", cmd.UpdaterID))

	// Validate the command
	if err := s.validateUpdateProfileCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get existing account
	account, err := s.accountRepo.GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Verify permission (only account owner can update)
	if account.User.Username != cmd.UpdaterID {
		return nil, fmt.Errorf("unauthorized: only the account owner can update their profile")
	}

	// Update profile fields
	if err := s.updateAccountProfile(account, cmd); err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Store the updated account
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to store account: %w", err)
	}

	s.logger.Info("updated profile successfully",
		zap.String("username", cmd.Username))

	// Emit events and queue federation
	events := s.emitAccountUpdatedEvents(ctx, account)
	s.queueFederationUpdate(ctx, account)

	return &AccountResult{
		Account: account,
		Events:  events,
	}, nil
}

// UpdatePreferences updates user preferences and emits events (but not federation)
func (s *Service) UpdatePreferences(ctx context.Context, cmd *UpdatePreferencesCommand) (*PreferencesResult, error) {
	s.logger.Info("updating preferences",
		zap.String("username", cmd.Username),
		zap.String("updater_id", cmd.UpdaterID))

	// Validate the command
	if err := s.validateUpdatePreferencesCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Verify permission (only account owner can update preferences)
	if cmd.Username != cmd.UpdaterID {
		return nil, fmt.Errorf("unauthorized: only the account owner can update their preferences")
	}

	// Build preferences map from command
	preferences := map[string]interface{}{
		"language":                    cmd.Language,
		"default_posting_visibility":  cmd.DefaultPostingVisibility,
		"default_media_sensitive":     cmd.DefaultMediaSensitive,
		"expand_spoilers":             cmd.ExpandSpoilers,
		"expand_media":                cmd.ExpandMedia,
		"autoplay_gifs":               cmd.AutoplayGifs,
		"show_follow_counts":          cmd.ShowFollowCounts,
		"preferred_timeline_order":    cmd.PreferredTimelineOrder,
		"search_suggestions_enabled":  cmd.SearchSuggestionsEnabled,
		"personalized_search_enabled": cmd.PersonalizedSearchEnabled,
		"reblog_filters":              cmd.ReblogFilters,
	}

	// Update preferences
	if err := s.accountRepo.UpdateAccountPreferences(ctx, cmd.Username, preferences); err != nil {
		return nil, fmt.Errorf("failed to update preferences: %w", err)
	}

	s.logger.Info("updated preferences successfully",
		zap.String("username", cmd.Username))

	// Note: Preference changes don't trigger federation events, only local events
	events := s.emitPreferencesUpdatedEvents(ctx, cmd.Username, preferences)

	return &PreferencesResult{
		Preferences: preferences,
		Events:      events,
	}, nil
}

// GetAccount retrieves a single account with privacy-aware data
func (s *Service) GetAccount(ctx context.Context, query *GetAccountQuery) (*storage.Account, error) {
	s.logger.Debug("getting account",
		zap.String("username", query.Username),
		zap.String("viewer_id", query.ViewerID))

	// Get the account
	account, err := s.accountRepo.GetAccount(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Check if account is suspended or deleted - hide from public
	if account.User.Suspended && query.ViewerID != query.Username {
		return nil, fmt.Errorf("account not found") // Don't reveal it's suspended
	}

	// Apply privacy filtering based on viewer relationship
	sanitized := s.sanitizeAccountForViewer(account, query.ViewerID)

	return sanitized, nil
}

// SearchAccounts searches for accounts with filters and privacy checks
func (s *Service) SearchAccounts(ctx context.Context, query *SearchAccountsQuery) (*AccountSearchResult, error) {
	s.logger.Debug("searching accounts",
		zap.String("query", query.Query),
		zap.String("viewer_id", query.ViewerID),
		zap.Bool("resolve", query.Resolve))

	// Validate search query
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	// Perform the search
	result, err := s.accountRepo.SearchAccounts(ctx, query.Query, query.Pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to search accounts: %w", err)
	}

	// Filter results based on privacy and other criteria
	filteredAccounts := make([]*storage.Account, 0, len(result.Items))
	for _, account := range result.Items {
		// Skip suspended accounts (unless viewer is the account owner)
		if account.User.Suspended && query.ViewerID != account.User.Username {
			continue
		}

		// Apply privacy filtering
		sanitized := s.sanitizeAccountForViewer(account, query.ViewerID)
		filteredAccounts = append(filteredAccounts, sanitized)
	}

	// Update the result with filtered items
	filteredResult := &interfaces.PaginatedResult[*storage.Account]{
		Items:      filteredAccounts,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		Total:      int64(len(filteredAccounts)),
	}

	return &AccountSearchResult{
		Accounts:   filteredAccounts,
		Pagination: filteredResult,
		Events:     []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) validateUpdateProfileCommand(_ context.Context, cmd *UpdateProfileCommand) error {
	if cmd.Username == "" {
		return fmt.Errorf("username is required")
	}

	if cmd.UpdaterID == "" {
		return fmt.Errorf("updater_id is required")
	}

	if len(cmd.DisplayName) > 100 {
		return fmt.Errorf("display name too long (max 100 characters)")
	}

	if len(cmd.Bio) > 5000 {
		return fmt.Errorf("bio too long (max 5000 characters)")
	}

	if len(cmd.Fields) > 4 {
		return fmt.Errorf("too many profile fields (max 4)")
	}

	for i, field := range cmd.Fields {
		if strings.TrimSpace(field.Name) == "" {
			return fmt.Errorf("profile field %d name cannot be empty", i+1)
		}
		if len(field.Name) > 255 {
			return fmt.Errorf("profile field %d name too long (max 255 characters)", i+1)
		}
		if len(field.Value) > 255 {
			return fmt.Errorf("profile field %d value too long (max 255 characters)", i+1)
		}
	}

	return nil
}

func (s *Service) validateUpdatePreferencesCommand(_ context.Context, cmd *UpdatePreferencesCommand) error {
	if cmd.Username == "" {
		return fmt.Errorf("username is required")
	}

	if cmd.UpdaterID == "" {
		return fmt.Errorf("updater_id is required")
	}

	validVisibilities := map[string]bool{
		models.VisibilityPublic:   true,
		models.VisibilityUnlisted: true,
		models.VisibilityPrivate:  true,
	}

	if cmd.DefaultPostingVisibility != "" && !validVisibilities[cmd.DefaultPostingVisibility] {
		return fmt.Errorf("invalid default posting visibility: %s", cmd.DefaultPostingVisibility)
	}

	validExpandMedia := map[string]bool{
		"default":  true,
		"show_all": true,
		"hide_all": true,
	}

	if cmd.ExpandMedia != "" && !validExpandMedia[cmd.ExpandMedia] {
		return fmt.Errorf("invalid expand media setting: %s", cmd.ExpandMedia)
	}

	validTimelineOrder := map[string]bool{
		"newest": true,
		"oldest": true,
	}

	if cmd.PreferredTimelineOrder != "" && !validTimelineOrder[cmd.PreferredTimelineOrder] {
		return fmt.Errorf("invalid timeline order: %s", cmd.PreferredTimelineOrder)
	}

	return nil
}

func (s *Service) updateAccountProfile(account *storage.Account, cmd *UpdateProfileCommand) error {
	// Update User fields
	if cmd.DisplayName != "" {
		account.User.DisplayName = cmd.DisplayName
	}

	// Update Actor fields (ActivityPub profile)
	if account.Actor == nil {
		return fmt.Errorf("account has no ActivityPub actor")
	}

	if cmd.DisplayName != "" {
		account.Actor.Name = cmd.DisplayName
	}

	if cmd.Bio != "" {
		account.Actor.Summary = cmd.Bio
	}

	// Update profile image URLs
	if cmd.Avatar != "" {
		if account.Actor.Icon == nil {
			account.Actor.Icon = &activitypub.Image{}
		}
		account.Actor.Icon.URL = cmd.Avatar
	}

	if cmd.Header != "" {
		if account.Actor.Image == nil {
			account.Actor.Image = &activitypub.Image{}
		}
		account.Actor.Image.URL = cmd.Header
	}

	// Update account flags
	account.Actor.ManuallyApprovesFollowers = cmd.Locked
	account.Actor.Discoverable = cmd.Discoverable

	// Bot status is determined by Actor Type, not a boolean field
	if cmd.Bot {
		account.Actor.Type = "Service"
	} else {
		account.Actor.Type = "Person"
	}

	// Update profile fields using Attachment format
	if len(cmd.Fields) > 0 {
		attachments := make([]activitypub.Attachment, len(cmd.Fields))
		for i, field := range cmd.Fields {
			attachments[i] = activitypub.Attachment{
				Type:  "PropertyValue",
				Name:  field.Name,
				Value: field.Value,
			}
		}
		account.Actor.Attachment = attachments
	}

	// Set updated timestamp
	account.User.UpdatedAt = time.Now()
	now := time.Now()
	account.Actor.Updated = &now

	return nil
}

func (s *Service) sanitizeAccountForViewer(account *storage.Account, viewerID string) *storage.Account {
	// Create a copy to avoid modifying the original
	sanitized := &storage.Account{
		User:  account.User,
		Actor: account.Actor,
	}

	// If viewer is the account owner, show everything
	if viewerID == account.User.Username {
		return sanitized
	}

	// For other viewers, apply privacy rules
	if account.User.Silenced {
		// Silenced accounts have limited visibility
		// Keep basic info but hide sensitive data
		if account.Actor != nil {
			account.Actor.Summary = "[Content hidden]"
		}
	}

	// Hide private/internal fields from other viewers
	userCopy := *account.User
	userCopy.Email = "" // Never expose email to others
	userCopy.PasswordHash = ""
	sanitized.User = &userCopy

	// ActivityPub actor is generally public, but we might want to hide some fields
	// based on privacy settings in the future

	return sanitized
}

func (s *Service) emitAccountUpdatedEvents(ctx context.Context, account *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "account.updated",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"account": account,
		},
	}

	// Emit to user's own stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", account.User.Username)
	if err := s.publisher.PublishToUser(ctx, account.User.Username, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	// Emit to followers' streams (they should see profile updates)
	// Note: This would typically require getting the follower list, but for now
	// we'll emit to a followers stream that other services can subscribe to
	followersEvent := *event
	followersEvent.Stream = fmt.Sprintf("followers:%s", account.User.Username)
	if err := s.publisher.PublishToStream(ctx, followersEvent.Stream, &followersEvent); err != nil {
		s.logger.Error("failed to publish to followers stream", zap.Error(err))
	} else {
		events = append(events, &followersEvent)
	}

	return events
}

func (s *Service) emitPreferencesUpdatedEvents(ctx context.Context, username string, preferences map[string]interface{}) []*streaming.Event {
	var events []*streaming.Event

	// Create preferences event (only for the user themselves)
	event := &streaming.Event{
		Type:      "preferences.updated",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"preferences": preferences,
		},
	}

	// Emit to user's stream only (preferences are private)
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", username)
	if err := s.publisher.PublishToUser(ctx, username, &userEvent); err != nil {
		s.logger.Error("failed to publish preferences update to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) queueFederationUpdate(ctx context.Context, account *storage.Account) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping profile update")
		return
	}

	// Create ActivityPub Update activity for the profile
	now := time.Now()
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   "https://www.w3.org/ns/activitystreams",
			Type:      "Update",
			ID:        fmt.Sprintf("%s#updates/%d", account.Actor.ID, now.Unix()),
			Published: &now,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:        []string{fmt.Sprintf("%s/followers", account.Actor.ID)},
		},
		Actor:  account.Actor.ID,
		Object: account.Actor,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation profile update",
			zap.String("username", account.User.Username),
			zap.Error(err))
	}
}