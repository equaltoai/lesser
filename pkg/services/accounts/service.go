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
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides account operations
type Service struct {
	storage    core.RepositoryStorage
	publisher  streaming.Publisher
	logger     *zap.Logger
	domainName string
	federation FederationService // Interface to be defined
	crypto     CryptoService     // Interface for crypto operations
	auth       AuthService       // Interface for auth operations
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// CryptoService defines the interface for cryptographic operations
type CryptoService interface {
	GenerateRSAKeyPair(bits int) (interface{}, error)
	EncodePublicKeyPEM(publicKey interface{}) ([]byte, error)
	EncodePrivateKeyPEM(privateKey interface{}) ([]byte, error)
}

// AuthService defines the interface for authentication operations
type AuthService interface {
	HashPassword(password string) (string, error)
	ValidatePassword(password, username string) error
	PasswordStrength(password string) int
}

// NewService creates a new Accounts Service with the required dependencies
func NewService(
	storage core.RepositoryStorage,
	publisher streaming.Publisher,
	federation FederationService,
	crypto CryptoService,
	auth AuthService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		storage:    storage,
		publisher:  publisher,
		federation: federation,
		crypto:     crypto,
		auth:       auth,
		logger:     logger,
		domainName: domainName,
	}
}

// Command structs for operations

// RegisterAccountCommand contains all data needed to register a new account
type RegisterAccountCommand struct {
	Username     string `json:"username" validate:"required,min=3,max=30"`
	Email        string `json:"email" validate:"required,email"`
	Password     string `json:"password"` // Optional for WebAuthn registration
	Locale       string `json:"locale"`
	Agreement    bool   `json:"agreement" validate:"required"`
	Reason       string `json:"reason"` // Registration reason (for approval)
	InviteCode   string `json:"invite_code"`
}

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

// GetPreferencesQuery contains parameters for retrieving user preferences
type GetPreferencesQuery struct {
	Username string `json:"username" validate:"required"`
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

// RegisterAccountResult contains the result of account registration
type RegisterAccountResult struct {
	Account   *storage.Account   `json:"account"`
	Actor     *activitypub.Actor `json:"actor"`
	Events    []*streaming.Event `json:"events"`
}

// AccountSearchResult contains search results and pagination information
type AccountSearchResult struct {
	Accounts   []*storage.Account                               `json:"accounts"`
	Pagination *interfaces.PaginatedResult[*storage.Account]   `json:"pagination"`
	Events     []*streaming.Event                              `json:"events"`
}

// Additional command structs for account operations

// LookupAccountQuery contains parameters for looking up an account by username@domain
type LookupAccountQuery struct {
	Acct     string `json:"acct" validate:"required"`     // username@domain format
	ViewerID string `json:"viewer_id"`                    // User performing the lookup
}

// GetFollowersQuery contains parameters for retrieving account followers
type GetFollowersQuery struct {
	Username   string                        `json:"username" validate:"required"`
	ViewerID   string                        `json:"viewer_id"` // User viewing the followers
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetFollowingQuery contains parameters for retrieving accounts being followed
type GetFollowingQuery struct {
	Username   string                        `json:"username" validate:"required"`
	ViewerID   string                        `json:"viewer_id"` // User viewing the following
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetFamiliarFollowersQuery contains parameters for finding mutual connections
type GetFamiliarFollowersQuery struct {
	AccountIDs []string `json:"account_ids" validate:"required,min=1"`
	ViewerID   string   `json:"viewer_id" validate:"required"`
}

// PinAccountCommand contains data needed to pin an account to user's profile
type PinAccountCommand struct {
	Username      string `json:"username" validate:"required"`
	TargetAccount string `json:"target_account" validate:"required"`
	PinnerID      string `json:"pinner_id" validate:"required"`
}

// UnpinAccountCommand contains data needed to unpin an account from user's profile
type UnpinAccountCommand struct {
	Username      string `json:"username" validate:"required"`
	TargetAccount string `json:"target_account" validate:"required"`
	PinnerID      string `json:"pinner_id" validate:"required"`
}

// GetAccountPinsQuery contains data needed to get account pins (endorsed accounts)
type GetAccountPinsQuery struct {
	Username string `json:"username" validate:"required"`
}

// SetAccountNoteCommand contains data needed to set a private note on an account
type SetAccountNoteCommand struct {
	Username      string `json:"username" validate:"required"`
	TargetAccount string `json:"target_account" validate:"required"`
	Note          string `json:"note" validate:"max=500"`
	SetterID      string `json:"setter_id" validate:"required"`
}

// RemoveFollowerCommand contains data needed to remove a follower
type RemoveFollowerCommand struct {
	Username    string `json:"username" validate:"required"`
	FollowerID  string `json:"follower_id" validate:"required"`
	RemoverID   string `json:"remover_id" validate:"required"`
}

// GetActivityPubCollectionQuery contains parameters for ActivityPub collection requests
type GetActivityPubCollectionQuery struct {
	Username       string `json:"username" validate:"required"`
	CollectionType string `json:"collection_type" validate:"required,oneof=followers following"`
	ViewerID       string `json:"viewer_id"`
	Page           bool   `json:"page"`
	Cursor         string `json:"cursor"`
	Limit          int    `json:"limit"`
}

// Additional result structs

// FollowersResult contains followers list and pagination
type FollowersResult struct {
	Followers  []*storage.Account                             `json:"followers"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
	Events     []*streaming.Event                            `json:"events"`
}

// FollowingResult contains following list and pagination
type FollowingResult struct {
	Following  []*storage.Account                             `json:"following"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
	Events     []*streaming.Event                            `json:"events"`
}

// FamiliarFollowersResult contains mutual connections grouped by account
type FamiliarFollowersResult struct {
	Results []FamiliarFollowersForAccount `json:"results"`
	Events  []*streaming.Event            `json:"events"`
}

// FamiliarFollowersForAccount represents mutual connections for a specific account
type FamiliarFollowersForAccount struct {
	ID       string             `json:"id"`
	Accounts []*storage.Account `json:"accounts"`
}

// RelationshipResult contains relationship status between accounts
type RelationshipResult struct {
	Relationship map[string]any     `json:"relationship"`
	Events       []*streaming.Event `json:"events"`
}

// AccountPinsResult contains pinned accounts (endorsements) and events
type AccountPinsResult struct {
	PinnedAccounts []*storage.Account `json:"pinned_accounts"`
	Events         []*streaming.Event `json:"events"`
}

// ActivityPubCollectionResult contains ActivityPub collection data
type ActivityPubCollectionResult struct {
	Collection map[string]any     `json:"collection"`
	Events     []*streaming.Event `json:"events"`
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
	account, err := s.storage.Account().GetAccount(ctx, cmd.Username)
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
	if err := s.storage.Account().UpdateAccount(ctx, account); err != nil {
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

// GetPreferences retrieves user preferences
func (s *Service) GetPreferences(ctx context.Context, query *GetPreferencesQuery) (*PreferencesResult, error) {
	s.logger.Info("getting preferences",
		zap.String("username", query.Username))

	// Get preferences from storage
	preferences, err := s.storage.Account().GetAccountPreferences(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences: %w", err)
	}

	// If no preferences found, return defaults
	if preferences == nil {
		preferences = map[string]interface{}{
			"language":                    "en",
			"default_posting_visibility":  "public",
			"default_media_sensitive":     false,
			"expand_spoilers":             false,
			"expand_media":                "default",
			"auto_play_gif":               false,
			"reduce_motion":               false,
			"use_blurhash":                true,
			"use_pending_items":           false,
			"show_trends":                 true,
		}
	}

	return &PreferencesResult{
		Preferences: preferences,
		Events:      []*streaming.Event{},
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
	if err := s.storage.Account().UpdateAccountPreferences(ctx, cmd.Username, preferences); err != nil {
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
func (s *Service) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	s.logger.Debug("getting account",
		zap.String("username", username))

	// Get the account
	account, err := s.storage.Account().GetAccount(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Check if account is suspended or deleted - hide from public
	if account.User.Suspended {
		return nil, fmt.Errorf("account not found") // Don't reveal it's suspended
	}

	// Return the account directly since we simplified the method
	return account, nil
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
	result, err := s.storage.Account().SearchAccounts(ctx, query.Query, query.Pagination)
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

// LookupAccount looks up an account by username@domain format
func (s *Service) LookupAccount(ctx context.Context, query *LookupAccountQuery) (*storage.Account, error) {
	s.logger.Debug("looking up account",
		zap.String("acct", query.Acct),
		zap.String("viewer_id", query.ViewerID))

	// Remove @ prefix if present
	acct := strings.TrimPrefix(query.Acct, "@")

	// For local accounts, just use the username part
	username := acct
	if parts := strings.Split(acct, "@"); len(parts) > 0 {
		username = parts[0]
	}

	// Get the account
	account, err := s.storage.Account().GetAccount(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Apply privacy filtering based on viewer
	sanitized := s.sanitizeAccountForViewer(account, query.ViewerID)
	return sanitized, nil
}

// GetFollowers retrieves the list of accounts following the given account
func (s *Service) GetFollowers(ctx context.Context, query *GetFollowersQuery) (*FollowersResult, error) {
	s.logger.Debug("getting followers",
		zap.String("username", query.Username),
		zap.String("viewer_id", query.ViewerID),
		zap.Int("limit", query.Pagination.Limit))

	// Verify target account exists
	_, err := s.storage.Account().GetAccount(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Get followers using relationship repository
	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not available")
	}

	// Get actor for the target account
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, fmt.Errorf("actor repository not available")
	}

	targetActor, err := actorRepo.GetActor(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Get followers
	followerIDs, cursor, err := relationshipRepo.GetFollowers(ctx, targetActor.ID, query.Pagination.Limit, query.Pagination.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to get followers: %w", err)
	}

	// Convert follower IDs to accounts
	accountRepo := s.storage.Account()
	followers := make([]*storage.Account, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		// Extract username from actor ID (format: https://domain/users/username)
		parts := strings.Split(followerID, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			account, err := accountRepo.GetAccount(ctx, username)
			if err != nil {
				s.logger.Warn("failed to get follower account",
					zap.String("follower_id", followerID),
					zap.Error(err))
				continue
			}
			followers = append(followers, account)
		}
	}

	return &FollowersResult{
		Followers: followers,
		Pagination: &interfaces.PaginatedResult[*storage.Account]{
			Items:      followers,
			NextCursor: cursor,
			HasMore:    cursor != "",
			Total:      int64(len(followers)),
		},
		Events: []*streaming.Event{},
	}, nil
}

// GetFollowing retrieves the list of accounts the given account is following
func (s *Service) GetFollowing(ctx context.Context, query *GetFollowingQuery) (*FollowingResult, error) {
	s.logger.Debug("getting following",
		zap.String("username", query.Username),
		zap.String("viewer_id", query.ViewerID),
		zap.Int("limit", query.Pagination.Limit))

	// Verify target account exists
	_, err := s.storage.Account().GetAccount(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Similar to GetFollowers - placeholder implementation
	return &FollowingResult{
		Following:  []*storage.Account{},
		Pagination: &interfaces.PaginatedResult[*storage.Account]{
			Items:      []*storage.Account{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		},
		Events: []*streaming.Event{},
	}, nil
}

// GetFamiliarFollowers returns accounts that the requesting user follows and who also follow the given accounts
func (s *Service) GetFamiliarFollowers(ctx context.Context, query *GetFamiliarFollowersQuery) (*FamiliarFollowersResult, error) {
	s.logger.Debug("getting familiar followers",
		zap.Strings("account_ids", query.AccountIDs),
		zap.String("viewer_id", query.ViewerID))

	// Get relationship repository
	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not available")
	}

	// Get actor repository
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, fmt.Errorf("actor repository not available")
	}

	// Get viewer's actor to get who they follow
	viewerActor, err := actorRepo.GetActor(ctx, query.ViewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get viewer actor: %w", err)
	}

	// Get who the viewer follows (their following list)
	viewerFollowing, _, err := relationshipRepo.GetFollowing(ctx, viewerActor.ID, 1000, "") // Get up to 1000 following
	if err != nil {
		return nil, fmt.Errorf("failed to get viewer following: %w", err)
	}

	// Create a set of who the viewer follows for quick lookup
	viewerFollowingSet := make(map[string]bool)
	for _, followingID := range viewerFollowing {
		viewerFollowingSet[followingID] = true
	}

	accountRepo := s.storage.Account()
	results := make([]FamiliarFollowersForAccount, 0, len(query.AccountIDs))

	for _, accountID := range query.AccountIDs {
		// Get target account's actor
		targetActor, err := actorRepo.GetActor(ctx, accountID)
		if err != nil {
			s.logger.Warn("failed to get target actor",
				zap.String("account_id", accountID),
				zap.Error(err))
			results = append(results, FamiliarFollowersForAccount{
				ID:       accountID,
				Accounts: []*storage.Account{},
			})
			continue
		}

		// Get target account's followers
		targetFollowers, _, err := relationshipRepo.GetFollowers(ctx, targetActor.ID, 100, "") // Limit to 100 for performance
		if err != nil {
			s.logger.Warn("failed to get target followers",
				zap.String("account_id", accountID),
				zap.Error(err))
			results = append(results, FamiliarFollowersForAccount{
				ID:       accountID,
				Accounts: []*storage.Account{},
			})
			continue
		}

		// Find intersection - followers of target who are also followed by the viewer
		familiarAccounts := make([]*storage.Account, 0)
		for _, followerID := range targetFollowers {
			if viewerFollowingSet[followerID] {
				// This follower follows the target AND is followed by the viewer
				// Extract username from actor ID
				parts := strings.Split(followerID, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					account, err := accountRepo.GetAccount(ctx, username)
					if err != nil {
						continue
					}
					familiarAccounts = append(familiarAccounts, account)
					if len(familiarAccounts) >= 12 { // Limit to 12 familiar followers per account
						break
					}
				}
			}
		}

		results = append(results, FamiliarFollowersForAccount{
			ID:       accountID,
			Accounts: familiarAccounts,
		})
	}

	return &FamiliarFollowersResult{
		Results: results,
		Events:  []*streaming.Event{},
	}, nil
}

// PinAccount pins an account to the user's profile
func (s *Service) PinAccount(ctx context.Context, cmd *PinAccountCommand) (*RelationshipResult, error) {
	s.logger.Info("pinning account",
		zap.String("username", cmd.Username),
		zap.String("target_account", cmd.TargetAccount),
		zap.String("pinner_id", cmd.PinnerID))

	// Verify permission
	if cmd.Username != cmd.PinnerID {
		return nil, fmt.Errorf("unauthorized: only the account owner can pin accounts")
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, fmt.Errorf("target account not found: %w", err)
	}

	// Create pin using repository
	pin := &storage.AccountPin{
		Username:       cmd.Username,
		PinnedActorID:  targetAccount.Actor.ID,
		PinnedUsername: cmd.TargetAccount,
		CreatedAt:      time.Now(),
	}

	if err := s.storage.Account().CreateAccountPin(ctx, pin); err != nil {
		if strings.Contains(err.Error(), "already pinned") {
			return nil, fmt.Errorf("account already pinned")
		}
		return nil, fmt.Errorf("failed to pin account: %w", err)
	}

	// Return relationship status (placeholder)
	return &RelationshipResult{
		Relationship: map[string]any{
			"id":        cmd.TargetAccount,
			"endorsed":  true,
			"following": false, // Would need to check actual relationship
			"followed_by": false,
		},
		Events: []*streaming.Event{},
	}, nil
}

// UnpinAccount unpins an account from the user's profile
func (s *Service) UnpinAccount(ctx context.Context, cmd *UnpinAccountCommand) (*RelationshipResult, error) {
	s.logger.Info("unpinning account",
		zap.String("username", cmd.Username),
		zap.String("target_account", cmd.TargetAccount),
		zap.String("pinner_id", cmd.PinnerID))

	// Verify permission
	if cmd.Username != cmd.PinnerID {
		return nil, fmt.Errorf("unauthorized: only the account owner can unpin accounts")
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, fmt.Errorf("target account not found: %w", err)
	}

	// Delete pin using repository
	if err := s.storage.Account().DeleteAccountPin(ctx, cmd.Username, targetAccount.Actor.ID); err != nil {
		return nil, fmt.Errorf("failed to unpin account: %w", err)
	}

	// Return relationship status (placeholder)
	return &RelationshipResult{
		Relationship: map[string]any{
			"id":        cmd.TargetAccount,
			"endorsed":  false,
			"following": false, // Would need to check actual relationship
			"followed_by": false,
		},
		Events: []*streaming.Event{},
	}, nil
}

// GetAccountPins retrieves all accounts pinned by a user (endorsements)
func (s *Service) GetAccountPins(ctx context.Context, query *GetAccountPinsQuery) (*AccountPinsResult, error) {
	s.logger.Info("getting account pins",
		zap.String("username", query.Username))

	// Get account pins from storage
	pins, err := s.storage.Social().GetAccountPins(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get account pins: %w", err)
	}

	// Convert pins to accounts
	accounts := make([]*storage.Account, 0, len(pins))
	for _, pin := range pins {
		// Extract username from actor ID - use a simple approach
		// Actor ID format: https://domain.com/users/username
		parts := strings.Split(pin.PinnedActorID, "/")
		var username string
		if len(parts) >= 2 && parts[len(parts)-2] == "users" {
			username = parts[len(parts)-1]
		}
		if username == "" {
			s.logger.Warn("could not extract username from actor ID",
				zap.String("actor_id", pin.PinnedActorID))
			continue
		}

		// Get account with actor data
		account, err := s.storage.Account().GetAccount(ctx, username)
		if err != nil {
			s.logger.Warn("failed to get pinned account",
				zap.String("username", username),
				zap.Error(err))
			continue
		}

		accounts = append(accounts, account)
	}

	return &AccountPinsResult{
		PinnedAccounts: accounts,
		Events:         []*streaming.Event{},
	}, nil
}

// SetAccountNote sets a private note on an account
func (s *Service) SetAccountNote(ctx context.Context, cmd *SetAccountNoteCommand) (*RelationshipResult, error) {
	s.logger.Info("setting account note",
		zap.String("username", cmd.Username),
		zap.String("target_account", cmd.TargetAccount),
		zap.String("setter_id", cmd.SetterID))

	// Verify permission
	if cmd.Username != cmd.SetterID {
		return nil, fmt.Errorf("unauthorized: only the account owner can set notes")
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, fmt.Errorf("target account not found: %w", err)
	}

	// Create or update account note
	note := &storage.AccountNote{
		Username:      cmd.Username,
		TargetActorID: targetAccount.Actor.ID,
		Note:          cmd.Note,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.storage.Account().CreateAccountNote(ctx, note); err != nil {
		return nil, fmt.Errorf("failed to set account note: %w", err)
	}

	// Return relationship status with note
	return &RelationshipResult{
		Relationship: map[string]any{
			"id":        cmd.TargetAccount,
			"note":      cmd.Note,
			"following": false, // Would need to check actual relationship
			"followed_by": false,
		},
		Events: []*streaming.Event{},
	}, nil
}

// RemoveFollower removes a follower from the current user's followers list
func (s *Service) RemoveFollower(ctx context.Context, cmd *RemoveFollowerCommand) (*RelationshipResult, error) {
	s.logger.Info("removing follower",
		zap.String("username", cmd.Username),
		zap.String("follower_id", cmd.FollowerID),
		zap.String("remover_id", cmd.RemoverID))

	// Verify permission
	if cmd.Username != cmd.RemoverID {
		return nil, fmt.Errorf("unauthorized: only the account owner can remove followers")
	}

	// Get relationship repository
	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return nil, fmt.Errorf("relationship repository not available")
	}

	// Get actor repository
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, fmt.Errorf("actor repository not available")
	}

	// Get actors for both accounts
	removerActor, err := actorRepo.GetActor(ctx, cmd.RemoverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get remover actor: %w", err)
	}

	followerActor, err := actorRepo.GetActor(ctx, cmd.FollowerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get follower actor: %w", err)
	}

	// Remove the follower relationship
	// Extract usernames from actor IDs for the repository method
	followerUsername := cmd.FollowerID
	removerUsername := cmd.RemoverID
	err = relationshipRepo.DeleteRelationship(ctx, followerUsername, removerUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to remove follower: %w", err)
	}

	// Get updated relationship status
	following, err := relationshipRepo.IsFollowing(ctx, removerActor.ID, followerActor.ID)
	if err != nil {
		s.logger.Warn("failed to check following status", zap.Error(err))
		following = false
	}

	followedBy, err := relationshipRepo.IsFollowing(ctx, followerActor.ID, removerActor.ID)
	if err != nil {
		s.logger.Warn("failed to check followed_by status", zap.Error(err))
		followedBy = false
	}

	// Emit event for follower removal
	events := []*streaming.Event{}
	if s.publisher != nil {
		event := &streaming.Event{
			Type:      "follower.removed",
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"remover_id":  cmd.RemoverID,
				"follower_id": cmd.FollowerID,
			},
		}
		if err := s.publisher.PublishToUser(ctx, cmd.RemoverID, event); err != nil {
			s.logger.Error("failed to publish follower removed event", zap.Error(err))
		}
		events = append(events, event)
	}

	return &RelationshipResult{
		Relationship: map[string]any{
			"id":                  cmd.FollowerID,
			"following":           following,
			"followed_by":         followedBy,
			"blocking":            false, // TODO: Check block status if needed
			"blocked_by":          false,
			"muting":              false,
			"muting_notifications": false,
			"requested":           false,
			"domain_blocking":     false,
			"showing_reblogs":     true,
			"endorsed":            false,
		},
		Events: events,
	}, nil
}

// GetActivityPubCollection handles ActivityPub collection requests with proper format
func (s *Service) GetActivityPubCollection(ctx context.Context, query *GetActivityPubCollectionQuery) (*ActivityPubCollectionResult, error) {
	s.logger.Debug("getting ActivityPub collection",
		zap.String("username", query.Username),
		zap.String("collection_type", query.CollectionType),
		zap.String("viewer_id", query.ViewerID),
		zap.Bool("page", query.Page))

	// Verify account exists
	account, err := s.storage.Account().GetAccount(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Check privacy settings for followers collection
	if query.CollectionType == "followers" && account.Actor.ManuallyApprovesFollowers {
		// Check if viewer is authorized (only account owner can see private followers)
		if query.ViewerID != query.Username {
			// Return empty collection for privacy protection
			return &ActivityPubCollectionResult{
				Collection: map[string]any{
					"@context":   "https://www.w3.org/ns/activitystreams",
					"id":         fmt.Sprintf("%s/%s", account.Actor.ID, query.CollectionType),
					"type":       "OrderedCollection",
					"totalItems": 0,
				},
				Events: []*streaming.Event{},
			}, nil
		}
	}

	// Build collection ID
	collectionID := fmt.Sprintf("%s/%s", account.Actor.ID, query.CollectionType)

	// If not requesting a page, return collection metadata
	if !query.Page {
		// Get total count (placeholder - would need proper repository integration)
		totalItems := 0

		collection := map[string]any{
			"@context":   "https://www.w3.org/ns/activitystreams",
			"id":         collectionID,
			"type":       "OrderedCollection",
			"totalItems": totalItems,
		}

		// Add first page link if there are items
		if totalItems > 0 {
			collection["first"] = fmt.Sprintf("%s?page=1", collectionID)
		}

		return &ActivityPubCollectionResult{
			Collection: collection,
			Events:     []*streaming.Event{},
		}, nil
	}

	// Return page data (placeholder implementation)
	pageID := fmt.Sprintf("%s?page=1", collectionID)
	if query.Cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, query.Cursor)
	}

	page := map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           pageID,
		"type":         "OrderedCollectionPage",
		"partOf":       collectionID,
		"orderedItems": []any{}, // Would contain actual follower/following URLs
	}

	return &ActivityPubCollectionResult{
		Collection: page,
		Events:     []*streaming.Event{},
	}, nil
}

// RegisterAccount creates a new user account with actor
func (s *Service) RegisterAccount(ctx context.Context, cmd *RegisterAccountCommand) (*RegisterAccountResult, error) {
	// Validate command
	if err := s.validateRegisterAccountCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if username is already taken
	existingAccount, _ := s.storage.Account().GetAccount(ctx, cmd.Username)
	if existingAccount != nil {
		return nil, fmt.Errorf("username already taken")
	}

	// Generate RSA keypair for the actor
	privateKey, err := s.generateRSAKeyPair()
	if err != nil {
		s.logger.Error("failed to generate RSA keypair", zap.Error(err))
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}

	// Encode public key to PEM format
	publicKeyPEM, err := s.encodePublicKeyPEM(privateKey)
	if err != nil {
		s.logger.Error("failed to encode public key", zap.Error(err))
		return nil, fmt.Errorf("failed to encode public key: %w", err)
	}

	// Create user object
	user := &storage.User{
		Username:     cmd.Username,
		Email:        cmd.Email,
		PasswordHash: "", // Will be set if password provided
		Approved:     true, // Auto-approve for now
		Suspended:    false,
		Role:         "user",
		Locale:       cmd.Locale,
		CreatedAt:    time.Now(),
	}

	// Hash password if provided
	if cmd.Password != "" {
		passwordHash, err := s.hashPassword(cmd.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = passwordHash
	}

	// Create corresponding actor
	actorID := fmt.Sprintf("https://%s/users/%s", s.domainName, cmd.Username)
	actor := activitypub.NewActor(activitypub.PersonType, actorID, cmd.Username)
	actor.Name = cmd.Username
	actor.URL = fmt.Sprintf("https://%s/@%s", s.domainName, cmd.Username)
	actor.CreatedAt = &user.CreatedAt
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}

	// Set default endpoints
	actor.Endpoints = &activitypub.Endpoints{
		SharedInbox: fmt.Sprintf("https://%s/inbox", s.domainName),
	}
	actor.Inbox = fmt.Sprintf("%s/inbox", actorID)
	actor.Outbox = fmt.Sprintf("%s/outbox", actorID)
	actor.Followers = fmt.Sprintf("%s/followers", actorID)
	actor.Following = fmt.Sprintf("%s/following", actorID)

	// Create account with actor
	account := &storage.Account{
		User:  user,
		Actor: actor,
	}

	// Save to storage
	if err := s.storage.Account().CreateAccount(ctx, account); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("username is already taken")
		}
		s.logger.Error("failed to create account", zap.Error(err))
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Record registration activity for metrics
	if err := s.storage.Activity().RecordActivity(ctx, "registration", actor.ID, time.Now()); err != nil {
		// Log the error but don't fail the request
		s.logger.Warn("failed to record registration activity", zap.Error(err))
	}

	// Create events for streaming
	events := s.emitAccountCreatedEvents(ctx, account)

	return &RegisterAccountResult{
		Account: account,
		Actor:   actor,
		Events:  events,
	}, nil
}

// validateRegisterAccountCommand validates the registration command
func (s *Service) validateRegisterAccountCommand(_ context.Context, cmd *RegisterAccountCommand) error {
	if cmd.Username == "" {
		return fmt.Errorf("username is required")
	}
	if len(cmd.Username) < 3 || len(cmd.Username) > 30 {
		return fmt.Errorf("username must be between 3 and 30 characters")
	}
	if cmd.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !cmd.Agreement {
		return fmt.Errorf("must agree to terms of service")
	}
	// Additional validation can be added here
	return nil
}

// Helper methods for account registration
func (s *Service) generateRSAKeyPair() (interface{}, error) {
	if s.crypto == nil {
		return nil, fmt.Errorf("crypto service not configured")
	}
	return s.crypto.GenerateRSAKeyPair(2048)
}

func (s *Service) encodePublicKeyPEM(publicKey interface{}) ([]byte, error) {
	if s.crypto == nil {
		return nil, fmt.Errorf("crypto service not configured")
	}
	return s.crypto.EncodePublicKeyPEM(publicKey)
}

func (s *Service) hashPassword(password string) (string, error) {
	if s.auth == nil {
		return "", fmt.Errorf("auth service not configured")
	}
	return s.auth.HashPassword(password)
}

// emitAccountCreatedEvents creates events for account creation
func (s *Service) emitAccountCreatedEvents(ctx context.Context, account *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Create account created event
	event := &streaming.Event{
		Type:      "account.created",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"account": account,
		},
	}

	// Emit to admin stream for monitoring
	adminEvent := *event
	adminEvent.Stream = "admin:accounts"
	if err := s.publisher.PublishToStream(ctx, "admin:accounts", &adminEvent); err != nil {
		s.logger.Warn("failed to publish account created event",
			zap.String("username", account.User.Username),
			zap.Error(err))
	}
	events = append(events, &adminEvent)

	return events
}

// GetMarkersQuery contains parameters for getting markers
type GetMarkersQuery struct {
	Username  string
	Timelines []string // Optional filter for specific timelines
}

// GetMarkersResult contains the result of getting markers
type GetMarkersResult struct {
	Markers map[string]*storage.Marker
	Events  []*streaming.Event
}

// GetMarkers retrieves timeline markers for a user
func (s *Service) GetMarkers(ctx context.Context, query *GetMarkersQuery) (*GetMarkersResult, error) {
	if query.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "required"}
	}

	markers, err := s.storage.Marker().GetMarkers(ctx, query.Username, query.Timelines)
	if err != nil {
		s.logger.Error("failed to get markers",
			zap.String("username", query.Username),
			zap.Error(err))
		return nil, err
	}

	return &GetMarkersResult{
		Markers: markers,
		Events:  []*streaming.Event{},
	}, nil
}

// SaveMarkerCommand contains parameters for saving a marker
type SaveMarkerCommand struct {
	Username   string
	Timeline   string
	LastReadID string
	Version    int
}

// SaveMarkerResult contains the result of saving a marker
type SaveMarkerResult struct {
	Events []*streaming.Event
}

// SaveMarker saves a timeline marker for a user
func (s *Service) SaveMarker(ctx context.Context, cmd *SaveMarkerCommand) (*SaveMarkerResult, error) {
	if cmd.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "required"}
	}
	if cmd.Timeline == "" {
		return nil, &ValidationError{Field: "timeline", Message: "required"}
	}
	if cmd.LastReadID == "" {
		return nil, &ValidationError{Field: "last_read_id", Message: "required"}
	}

	err := s.storage.Marker().SaveMarker(ctx, cmd.Username, cmd.Timeline, cmd.LastReadID, cmd.Version)
	if err != nil {
		s.logger.Error("failed to save marker",
			zap.String("username", cmd.Username),
			zap.String("timeline", cmd.Timeline),
			zap.Error(err))
		return nil, err
	}

	// Emit marker updated event
	var events []*streaming.Event
	if s.publisher != nil {
		event := &streaming.Event{
			Type:      "marker.updated",
			Stream:    fmt.Sprintf("user:%s", cmd.Username),
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"timeline":     cmd.Timeline,
				"last_read_id": cmd.LastReadID,
				"version":      cmd.Version,
			},
		}
		if err := s.publisher.PublishToUser(ctx, cmd.Username, event); err != nil {
			s.logger.Warn("failed to publish marker updated event",
				zap.String("username", cmd.Username),
				zap.String("timeline", cmd.Timeline),
				zap.Error(err))
		} else {
			events = append(events, event)
		}
	}

	return &SaveMarkerResult{
		Events: events,
	}, nil
}

// StoreOAuthStateCommand contains parameters for storing OAuth state
type StoreOAuthStateCommand struct {
	State      string
	OAuthState *storage.OAuthState
}

// StoreOAuthStateResult contains the result of storing OAuth state
type StoreOAuthStateResult struct {
	Events []*streaming.Event
}

// StoreOAuthState stores OAuth authorization state
func (s *Service) StoreOAuthState(ctx context.Context, cmd *StoreOAuthStateCommand) (*StoreOAuthStateResult, error) {
	if cmd.State == "" {
		return nil, &ValidationError{Field: "state", Message: "required"}
	}
	if cmd.OAuthState == nil {
		return nil, &ValidationError{Field: "oauth_state", Message: "required"}
	}

	err := s.storage.Account().StoreOAuthState(ctx, cmd.State, cmd.OAuthState)
	if err != nil {
		s.logger.Error("failed to store OAuth state",
			zap.String("state", cmd.State),
			zap.Error(err))
		return nil, err
	}

	return &StoreOAuthStateResult{
		Events: []*streaming.Event{},
	}, nil
}

// CreateAuthorizationCodeCommand contains parameters for creating an authorization code
type CreateAuthorizationCodeCommand struct {
	AuthCode *storage.AuthorizationCode
}

// CreateAuthorizationCodeResult contains the result of creating an authorization code
type CreateAuthorizationCodeResult struct {
	Events []*streaming.Event
}

// CreateAuthorizationCode creates a new authorization code
func (s *Service) CreateAuthorizationCode(ctx context.Context, cmd *CreateAuthorizationCodeCommand) (*CreateAuthorizationCodeResult, error) {
	if cmd.AuthCode == nil {
		return nil, &ValidationError{Field: "auth_code", Message: "required"}
	}

	err := s.storage.Account().CreateAuthorizationCode(ctx, cmd.AuthCode)
	if err != nil {
		s.logger.Error("failed to create authorization code",
			zap.String("code", cmd.AuthCode.Code),
			zap.Error(err))
		return nil, err
	}

	return &CreateAuthorizationCodeResult{
		Events: []*streaming.Event{},
	}, nil
}

// GetOAuthAppQuery contains parameters for getting an OAuth app
type GetOAuthAppQuery struct {
	ClientID string
}

// GetOAuthAppResult contains the result of getting an OAuth app
type GetOAuthAppResult struct {
	App    *storage.OAuthApp
	Events []*streaming.Event
}

// GetOAuthApp retrieves an OAuth application by client ID
func (s *Service) GetOAuthApp(ctx context.Context, query *GetOAuthAppQuery) (*GetOAuthAppResult, error) {
	if query.ClientID == "" {
		return nil, &ValidationError{Field: "client_id", Message: "required"}
	}

	app, err := s.storage.Account().GetOAuthApp(ctx, query.ClientID)
	if err != nil {
		s.logger.Error("failed to get OAuth app",
			zap.String("client_id", query.ClientID),
			zap.Error(err))
		return nil, err
	}

	return &GetOAuthAppResult{
		App:    app,
		Events: []*streaming.Event{},
	}, nil
}

// GetUserAppConsentQuery contains parameters for checking user consent
type GetUserAppConsentQuery struct {
	Username string
	ClientID string
}

// GetUserAppConsentResult contains the result of checking user consent
type GetUserAppConsentResult struct {
	Consent *storage.UserAppConsent
	Events  []*streaming.Event
}

// GetUserAppConsent retrieves user's consent for an OAuth app
func (s *Service) GetUserAppConsent(ctx context.Context, query *GetUserAppConsentQuery) (*GetUserAppConsentResult, error) {
	if query.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "required"}
	}
	if query.ClientID == "" {
		return nil, &ValidationError{Field: "client_id", Message: "required"}
	}

	consent, err := s.storage.Account().GetUserAppConsent(ctx, query.Username, query.ClientID)
	if err != nil {
		// Not found is not an error for consent check
		if err.Error() == "consent not found" {
			return &GetUserAppConsentResult{
				Consent: nil,
				Events:  []*streaming.Event{},
			}, nil
		}
		s.logger.Error("failed to get user app consent",
			zap.String("username", query.Username),
			zap.String("client_id", query.ClientID),
			zap.Error(err))
		return nil, err
	}

	return &GetUserAppConsentResult{
		Consent: consent,
		Events:  []*streaming.Event{},
	}, nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// GetInstanceStatsQuery contains parameters for getting instance statistics
type GetInstanceStatsQuery struct{}

// GetInstanceStatsResult contains instance statistics
type GetInstanceStatsResult struct {
	TotalUsers      int
	ActiveMonth     int
	ActiveHalfyear  int
	LocalPosts      int
	LocalComments   int
	Events          []*streaming.Event
}

// GetInstanceStats retrieves instance-level statistics
func (s *Service) GetInstanceStats(ctx context.Context, query *GetInstanceStatsQuery) (*GetInstanceStatsResult, error) {
	// Get user statistics
	totalUsers, err := s.storage.Analytics().GetTotalUserCount(ctx)
	if err != nil {
		s.logger.Warn("failed to get total user count", zap.Error(err))
		totalUsers = 1 // Default fallback
	}

	activeMonth, err := s.storage.Analytics().GetActiveUserCount(ctx, 30) // Last 30 days
	if err != nil {
		s.logger.Warn("failed to get monthly active users", zap.Error(err))
		activeMonth = 1 // Default fallback
	}

	activeHalfyear, err := s.storage.Analytics().GetActiveUserCount(ctx, 180) // Last 6 months
	if err != nil {
		s.logger.Warn("failed to get halfyear active users", zap.Error(err))
		activeHalfyear = activeMonth // Fallback to monthly
	}

	// Get post statistics
	localPosts, err := s.storage.Instance().GetLocalPostCount(ctx)
	if err != nil {
		s.logger.Warn("failed to get local post count", zap.Error(err))
		localPosts = 0
	}

	// Get comment statistics (posts with InReplyToID)
	localComments, err := s.storage.Instance().GetLocalCommentCount(ctx)
	if err != nil {
		s.logger.Warn("failed to get local comment count", zap.Error(err))
		localComments = 0
	}

	return &GetInstanceStatsResult{
		TotalUsers:     totalUsers,
		ActiveMonth:    activeMonth,
		ActiveHalfyear: activeHalfyear,
		LocalPosts:     int(localPosts),
		LocalComments:  int(localComments),
		Events:         []*streaming.Event{},
	}, nil
}

// GetAccountMetadataQuery represents a request to get account metadata
type GetAccountMetadataQuery struct {
	Username string `json:"username" validate:"required"`
}

// GetAccountMetadataResult represents account metadata
type GetAccountMetadataResult struct {
	Actor    *activitypub.Actor    `json:"actor"`
	Metadata *storage.ActorMetadata `json:"metadata"`
}

// GetAccountMetadata retrieves an account with its metadata
func (s *Service) GetAccountMetadata(ctx context.Context, query *GetAccountMetadataQuery) (*GetAccountMetadataResult, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage not available")
	}

	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, fmt.Errorf("actor repository not available")
	}

	actor, err := actorRepo.GetActor(ctx, query.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return &GetAccountMetadataResult{
		Actor:    actor,
		Metadata: nil, // Metadata can be added later if needed
	}, nil
}

// IsAccountPinnedQuery represents a request to check if an account is pinned
type IsAccountPinnedQuery struct {
	UserID     string `json:"user_id" validate:"required"`
	PinnedActorID string `json:"pinned_actor_id" validate:"required"`
}

// IsAccountPinned checks if a user has pinned an account
func (s *Service) IsAccountPinned(ctx context.Context, userID, pinnedActorID string) (bool, error) {
	if s.storage == nil {
		return false, fmt.Errorf("storage not available")
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return false, fmt.Errorf("user repository not available")
	}

	isPinned, err := userRepo.IsAccountPinned(ctx, userID, pinnedActorID)
	if err != nil {
		return false, fmt.Errorf("failed to check if account is pinned: %w", err)
	}

	return isPinned, nil
}

// GetUserQuery represents a request to get a user by username
type GetUserQuery struct {
	Username string `json:"username" validate:"required"`
}

// GetUserResult contains the user and events
type GetUserResult struct {
	User   *storage.User      `json:"user"`
	Events []*streaming.Event `json:"events"`
}

// GetUser retrieves a user by username
func (s *Service) GetUser(ctx context.Context, username string) (*storage.User, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage not available")
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return nil, fmt.Errorf("user repository not available")
	}

	user, err := userRepo.GetUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

// GetPreferenceQuery represents a request to get a user preference
type GetPreferenceQuery struct {
	UserID string `json:"user_id" validate:"required"`
	Key    string `json:"key" validate:"required"`
}

// GetPreferenceResult contains the preference value and events
type GetPreferenceResult struct {
	Value  string             `json:"value"`
	Events []*streaming.Event `json:"events"`
}

// GetPreference retrieves a user preference value
func (s *Service) GetPreference(ctx context.Context, userID, key string) (string, error) {
	if s.storage == nil {
		return "", fmt.Errorf("storage not available")
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return "", fmt.Errorf("user repository not available")
	}

	preferences, err := userRepo.GetUserPreferences(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user preferences: %w", err)
	}

	if preferences == nil {
		return "", nil // No preferences found
	}

	// Look for the preference in the preferences map
	if value, exists := preferences.Preferences[key]; exists {
		return value, nil
	}

	// Return empty string if preference not found
	return "", nil
}

// GetFollowRequestStateQuery represents a request to get follow request state
type GetFollowRequestStateQuery struct {
	RequesterID string `json:"requester_id" validate:"required"`
	TargetID    string `json:"target_id" validate:"required"`
}

// GetFollowRequestStateResult contains the state and events
type GetFollowRequestStateResult struct {
	State  string             `json:"state"`
	Events []*streaming.Event `json:"events"`
}

// GetFollowRequestState retrieves the state of a follow request
func (s *Service) GetFollowRequestState(ctx context.Context, requesterID, targetID string) (string, error) {
	if s.storage == nil {
		return "", fmt.Errorf("storage not available")
	}

	relationshipRepo := s.storage.Relationship()
	if relationshipRepo == nil {
		return "", fmt.Errorf("relationship repository not available")
	}

	// Check if there's a pending follow request
	request, err := relationshipRepo.GetFollowRequest(ctx, requesterID, targetID)
	if err != nil {
		return "none", nil // No request found
	}

	return request.Status, nil
}

// IsBlockedDomainQuery represents a request to check if a domain is blocked
type IsBlockedDomainQuery struct {
	UserID       string `json:"user_id" validate:"required"`
	TargetDomain string `json:"target_domain" validate:"required"`
}

// IsBlockedDomainResult contains the blocked status and events
type IsBlockedDomainResult struct {
	IsBlocked bool               `json:"is_blocked"`
	Events    []*streaming.Event `json:"events"`
}

// IsBlockedDomain checks if a domain is blocked for a user
func (s *Service) IsBlockedDomain(ctx context.Context, userID, targetDomain string) (bool, error) {
	if s.storage == nil {
		return false, fmt.Errorf("storage not available")
	}

	domainBlockRepo := s.storage.DomainBlock()
	if domainBlockRepo == nil {
		return false, fmt.Errorf("domain block repository not available")
	}

	// Check user-level domain block (userID is typically the username in this context)
	blocked, err := domainBlockRepo.IsBlockedDomain(ctx, userID, targetDomain)
	if err != nil {
		return false, fmt.Errorf("failed to check if domain is blocked by user: %w", err)
	}
	
	return blocked, nil
}

// GetFieldVerificationQuery represents a request to get field verification
type GetFieldVerificationQuery struct {
	Username  string `json:"username" validate:"required"`
	FieldName string `json:"field_name" validate:"required"`
}

// GetFieldVerificationResult contains the verification status and events
type GetFieldVerificationResult struct {
	Field  *storage.ActorField `json:"field"`
	Events []*streaming.Event  `json:"events"`
}

// GetFieldVerification retrieves field verification information
func (s *Service) GetFieldVerification(ctx context.Context, username, fieldName string) (*storage.ActorField, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage not available")
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return nil, fmt.Errorf("account repository not available")
	}

	field, err := accountRepo.GetFieldVerification(ctx, username, fieldName)
	if err != nil {
		return nil, fmt.Errorf("failed to get field verification: %w", err)
	}
	return field, nil
}

// GetAccountNoteQuery represents a request to get an account note
type GetAccountNoteQuery struct {
	CurrentUsername string `json:"current_username" validate:"required"`
	TargetActorID   string `json:"target_actor_id" validate:"required"`
}

// GetAccountNoteResult contains the note and events
type GetAccountNoteResult struct {
	Note   string             `json:"note"`
	Events []*streaming.Event `json:"events"`
}

// GetAccountNote retrieves a private note set on an account
func (s *Service) GetAccountNote(ctx context.Context, currentUsername, targetActorID string) (string, error) {
	if s.storage == nil {
		return "", fmt.Errorf("storage not available")
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return "", fmt.Errorf("user repository not available")
	}

	note, err := userRepo.GetAccountNote(ctx, currentUsername, targetActorID)
	if err != nil {
		return "", fmt.Errorf("failed to get account note: %w", err)
	}
	
	if note == nil {
		return "", nil // No note found
	}
	
	return note.Note, nil
}