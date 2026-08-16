// Package accounts provides the core Accounts Service for the Lesser project's API alignment.
// This service handles all account/user profile operations including profile updates,
// preferences management, and account discovery. It emits appropriate events for real-time
// streaming and queues federation delivery for remote followers.
package accounts

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"reflect"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Standardized error constants for account operations
// These align with the constants in pkg/services/errors.go
var (
	// Input validation errors
	ErrValidationFailed  = errors.New("validation failed")
	ErrEmptySearchQuery  = errors.New("search query cannot be empty")
	ErrUsernameRequired  = errors.New("username is required")
	ErrUpdaterIDRequired = errors.New("updater_id is required")

	// Account operations
	ErrGetAccount                = errors.New("failed to get account")
	ErrUpdateProfile             = errors.New("failed to update profile")
	ErrStoreAccount              = errors.New("failed to store account")
	ErrGetPreferences            = errors.New("failed to get preferences")
	ErrUpdatePreferences         = errors.New("failed to update preferences")
	ErrAccountNotFound           = errors.New("account not found")
	ErrSearchAccounts            = errors.New("failed to search accounts")
	ErrProfileFieldNameEmpty     = errors.New("profile field name cannot be empty")
	ErrProfileFieldNameTooLong   = errors.New("profile field name too long (max 255 characters)")
	ErrProfileFieldValueTooLong  = errors.New("profile field value too long (max 255 characters)")
	ErrInvalidExpandMediaSetting = errors.New("invalid expand media setting")
	ErrInvalidTimelineOrder      = errors.New("invalid timeline order")
	ErrAccountNoActivityPubActor = errors.New("account has no ActivityPub actor")

	// Actor operations
	ErrGetActor             = errors.New("failed to get actor")
	ErrGetFollowersAccounts = errors.New("failed to get followers")
	ErrGetFollowingList     = errors.New("failed to get following list")
	ErrGetViewerActor       = errors.New("failed to get viewer actor")
	ErrGetViewerFollowing   = errors.New("failed to get viewer following")

	// Repository availability errors
	ErrRelationshipRepositoryNotAvailable = errors.New("relationship repository not available")
	ErrActorRepositoryNotAvailable        = errors.New("actor repository not available")

	// Account relationships
	ErrTargetAccountNotFound = errors.New("target account not found")
	ErrAccountAlreadyPinned  = errors.New("account already pinned")
	ErrPinAccount            = errors.New("failed to pin account")
	ErrUnpinAccount          = errors.New("failed to unpin account")
	ErrGetAccountPins        = errors.New("failed to get account pins")
	ErrSetAccountNote        = errors.New("failed to set account note")
	ErrRemoveFollower        = errors.New("failed to remove follower")

	// Account creation
	ErrEmailRequired              = errors.New("email is required")
	ErrMustAgreeToTerms           = errors.New("must agree to terms of service")
	ErrUsernameAlreadyTaken       = errors.New("username already taken")
	ErrGenerateKeypair            = errors.New("failed to generate keypair")
	ErrEncodePublicKey            = errors.New("failed to encode public key")
	ErrHashPassword               = errors.New("failed to hash password")
	ErrCreateAccount              = errors.New("failed to create account")
	ErrCryptoServiceNotConfigured = errors.New("crypto service not configured")
	ErrAuthServiceNotConfigured   = errors.New("auth service not configured")
	ErrStorageNotAvailable        = errors.New("storage not available")

	// User operations
	ErrCheckAccountPinned                = errors.New("failed to check if account is pinned")
	ErrGetUser                           = errors.New("failed to get user")
	ErrGetUserPreferences                = errors.New("failed to get user preferences")
	ErrCheckDomainBlockedByUser          = errors.New("failed to check if domain is blocked by user")
	ErrGetFieldVerification              = errors.New("failed to get field verification")
	ErrGetAccountNote                    = errors.New("failed to get account note")
	ErrUserRepositoryNotAvailable        = errors.New("user repository not available")
	ErrDomainBlockRepositoryNotAvailable = errors.New("domain block repository not available")
	ErrAccountRepositoryNotAvailable     = errors.New("account repository not available")

	// Permission errors
	ErrCannotUpdateProfileForOtherUser     = errors.New("cannot update profile for another user")
	ErrCannotUpdatePreferencesForOtherUser = errors.New("cannot update preferences for another user")
	ErrCannotPinAccountForOtherUser        = errors.New("cannot pin account for another user")
	ErrCannotUnpinAccountForOtherUser      = errors.New("cannot unpin account for another user")
	ErrCannotSetNoteForOtherUser           = errors.New("cannot set note for another user")
	ErrCannotRemoveFollowerForOtherUser    = errors.New("cannot remove follower for another user")

	// Quote permissions errors
	ErrQuoteRepositoryNotAvailable = errors.New("quote repository not available")
)

// Collection type constants
const (
	collectionFollowers = "followers"
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

// quotePermissionsCreator captures the subset of quote repository capabilities needed during registration.
type quotePermissionsCreator interface {
	CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error
}

// accountRegistrationRepository captures account operations needed during registration.
type accountRegistrationRepository interface {
	UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error
	DeleteAccount(ctx context.Context, username string) error
}

// Compile-time checks ensuring the concrete repositories satisfy the narrow interfaces above.
var (
	_ quotePermissionsCreator       = (*repositories.QuoteRepository)(nil)
	_ accountRegistrationRepository = (*repositories.AccountRepository)(nil)
)

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

	logger.Info("accounts service: initializing",
		zap.String("domain", domainName),
		zap.Bool("storage_present", storage != nil))

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

	svc := &Service{
		storage:          storage,
		publisher:        publisher,
		federation:       federation,
		crypto:           crypto,
		auth:             auth,
		logger:           logger,
		domainName:       domainName,
		businessLogic:    businessLogic,
		activityPubLogic: activityPubLogic,
		mastodonLogic:    mastodonLogic,
		streamingEmitter: streamingEmitter,
	}

	logger.Info("accounts service: initialized",
		zap.String("domain", domainName))

	return svc
}

func (s *Service) normalizeUsername(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return trimmed
	}

	trimmed = strings.TrimPrefix(trimmed, "acct:")
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.TrimSuffix(trimmed, "/")

	remoteDomain := ""
	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		urlWithoutScheme := strings.TrimSuffix(trimmed, "/")

		// Extract the remote domain before stripping the URL structure so we
		// can reconstruct user@domain for remote actors. Without this step,
		// https://remote.example/users/admin would normalize to "admin" and
		// collide with a same-named local account. (CSR-052)
		parsed, parseErr := neturl.Parse(urlWithoutScheme)
		if parseErr == nil && parsed != nil && strings.TrimSpace(parsed.Hostname()) != "" && !s.isLocalDomain(parsed.Hostname()) {
			remoteDomain = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}

		if idx := strings.Index(urlWithoutScheme, "/users/"); idx != -1 && idx+7 < len(urlWithoutScheme) {
			trimmed = urlWithoutScheme[idx+7:]
		} else if idx := strings.LastIndex(urlWithoutScheme, "/@"); idx != -1 && idx+2 < len(urlWithoutScheme) {
			trimmed = urlWithoutScheme[idx+2:]
		} else {
			parts := strings.Split(urlWithoutScheme, "/")
			if len(parts) > 0 {
				trimmed = parts[len(parts)-1]
			}
		}
		trimmed = strings.TrimPrefix(trimmed, "@")
	}

	// Reconstruct user@domain for remote actors when the URL had a foreign host.
	if remoteDomain != "" && !strings.Contains(trimmed, "@") {
		trimmed = fmt.Sprintf("%s@%s", trimmed, remoteDomain)
	}

	if at := strings.LastIndex(trimmed, "@"); at != -1 {
		localPart := trimmed[:at]
		domainPart := trimmed[at+1:]
		if s.isLocalDomain(domainPart) {
			trimmed = localPart
		}
	}

	return strings.ToLower(strings.TrimSpace(trimmed))
}

func storedUsernameMatches(storedUsername, candidate string) bool {
	storedUsername = strings.TrimSpace(storedUsername)
	candidate = strings.TrimSpace(candidate)
	if storedUsername == "" || candidate == "" {
		return false
	}

	return strings.EqualFold(storedUsername, candidate)
}

func (s *Service) isLocalDomain(domain string) bool {
	if domain == "" {
		return false
	}
	return normalizeDomain(domain) == normalizeDomain(s.domainName)
}

func normalizeDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

// Command structs for operations

// RegisterAccountCommand contains all data needed to register a new account
type RegisterAccountCommand struct {
	Username                 string `json:"username" validate:"required,min=3,max=30"`
	Email                    string `json:"email" validate:"required,email"`
	Password                 string `json:"password"` // Optional for WebAuthn registration
	Locale                   string `json:"locale"`
	Agreement                bool   `json:"agreement" validate:"required"`
	Reason                   string `json:"reason"` // Registration reason (for approval)
	InviteCode               string `json:"invite_code"`
	DefaultPostingVisibility string `json:"default_posting_visibility"`

	// RegistrationChallengeID is a registration-time proof reference (e.g. wallet challenge ID).
	// Successful registration binds that proof to the typed wallet challenge row.
	RegistrationChallengeID string `json:"registration_challenge_id,omitempty"`

	// PasskeyRegistrationProof is the single-use proof emitted by the public WebAuthn signup finish ceremony.
	PasskeyRegistrationProof string `json:"passkey_registration_proof,omitempty"`
}

// UpdateProfileCommand contains all data needed to update a user's profile
type UpdateProfileCommand struct {
	Username     string         `json:"username" validate:"required"`
	DisplayName  string         `json:"display_name" validate:"max=100"`
	Bio          string         `json:"bio" validate:"max=5000"`
	Avatar       string         `json:"avatar"`                         // URL to avatar image
	Header       string         `json:"header"`                         // URL to header image
	Locked       bool           `json:"locked"`                         // Account locked (requires approval for follows)
	Bot          bool           `json:"bot"`                            // Bot account flag
	Fields       []ProfileField `json:"fields"`                         // Custom profile fields
	Discoverable bool           `json:"discoverable"`                   // Show in directory
	NoIndex      bool           `json:"no_index"`                       // Don't index for search
	Sensitive    bool           `json:"sensitive"`                      // Mark media as sensitive by default
	Language     string         `json:"language"`                       // Default language
	UpdaterID    string         `json:"updater_id" validate:"required"` // Must be the account owner
}

// GetPreferencesQuery contains parameters for retrieving user preferences
type GetPreferencesQuery struct {
	Username string `json:"username" validate:"required"`
}

// UpdatePreferencesCommand contains all data needed to update user preferences
type UpdatePreferencesCommand struct {
	Username                  string          `json:"username" validate:"required"`
	Language                  string          `json:"language"`
	DefaultPostingVisibility  string          `json:"default_posting_visibility" validate:"oneof=public unlisted private direct"`
	DefaultMediaSensitive     bool            `json:"default_media_sensitive"`
	DirectMessagesFrom        string          `json:"direct_messages_from" validate:"oneof=FOLLOWING_ONLY ANYONE"`
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
	Query      string                       `json:"query" validate:"required"`
	ViewerID   string                       `json:"viewer_id"` // User performing the search
	Pagination interfaces.PaginationOptions `json:"pagination"`
	Resolve    bool                         `json:"resolve"`   // Resolve remote accounts
	Following  bool                         `json:"following"` // Only search following
	Followers  bool                         `json:"followers"` // Only search followers
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
	Account *storage.Account   `json:"account"`
	Events  []*streaming.Event `json:"events"`
}

// PreferencesResult contains user preferences and any events
type PreferencesResult struct {
	Preferences map[string]interface{} `json:"preferences"`
	Events      []*streaming.Event     `json:"events"`
}

// RegisterAccountResult contains the result of account registration
type RegisterAccountResult struct {
	Account *storage.Account   `json:"account"`
	Actor   *activitypub.Actor `json:"actor"`
	Events  []*streaming.Event `json:"events"`
}

// AccountSearchResult contains search results and pagination information
type AccountSearchResult struct {
	Accounts   []*storage.Account                            `json:"accounts"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
	Events     []*streaming.Event                            `json:"events"`
}

// Additional command structs for account operations

// LookupAccountQuery contains parameters for looking up an account by username@domain
type LookupAccountQuery struct {
	Acct     string `json:"acct" validate:"required"` // username@domain format
	ViewerID string `json:"viewer_id"`                // User performing the lookup
}

// GetFollowersQuery contains parameters for retrieving account followers
type GetFollowersQuery struct {
	Username   string                       `json:"username" validate:"required"`
	ViewerID   string                       `json:"viewer_id"` // User viewing the followers
	Pagination interfaces.PaginationOptions `json:"pagination"`
}

// GetFollowingQuery contains parameters for retrieving accounts being followed
type GetFollowingQuery struct {
	Username   string                       `json:"username" validate:"required"`
	ViewerID   string                       `json:"viewer_id"` // User viewing the following
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
	Username   string `json:"username" validate:"required"`
	FollowerID string `json:"follower_id" validate:"required"`
	RemoverID  string `json:"remover_id" validate:"required"`
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
	Followers  []*storage.Account                            `json:"followers"`
	Pagination *interfaces.PaginatedResult[*storage.Account] `json:"pagination"`
	Events     []*streaming.Event                            `json:"events"`
}

// FollowingResult contains following list and pagination
type FollowingResult struct {
	Following  []*storage.Account                            `json:"following"`
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
		return nil, ErrValidationFailed
	}

	// Get existing account
	account, err := s.storage.Account().GetAccount(ctx, cmd.Username)
	if err != nil {
		return nil, ErrGetAccount
	}

	// Verify permission (only account owner can update)
	if !storedUsernameMatches(account.User.Username, cmd.UpdaterID) {
		return nil, common.ErrForbidden(ErrCannotUpdateProfileForOtherUser)
	}

	// Update profile fields
	if err := s.updateAccountProfile(account, cmd); err != nil {
		return nil, ErrUpdateProfile
	}

	// Store the updated account
	if err := s.storage.Account().UpdateAccount(ctx, account); err != nil {
		return nil, ErrStoreAccount
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
		return nil, ErrGetPreferences
	}

	// If no preferences found, return defaults
	if preferences == nil {
		preferences = map[string]interface{}{
			"language":                   "en",
			"default_posting_visibility": "public",
			"default_media_sensitive":    false,
			"expand_spoilers":            false,
			"expand_media":               "default",
			"auto_play_gif":              false,
			"reduce_motion":              false,
			"use_blurhash":               true,
			"use_pending_items":          false,
			"show_trends":                true,
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
		return nil, ErrValidationFailed
	}

	// Verify permission (only account owner can update preferences)
	if cmd.Username != cmd.UpdaterID {
		return nil, common.ErrForbidden(ErrCannotUpdatePreferencesForOtherUser)
	}

	// Build preferences map from command
	preferences := map[string]interface{}{
		"language":                    cmd.Language,
		"default_posting_visibility":  cmd.DefaultPostingVisibility,
		"default_media_sensitive":     cmd.DefaultMediaSensitive,
		"direct_messages_from":        cmd.DirectMessagesFrom,
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
		return nil, ErrUpdatePreferences
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
	canonicalUsername := s.normalizeUsername(username)
	s.logger.Info("accounts service: getting account",
		zap.String("username", username),
		zap.String("canonical_username", canonicalUsername))

	// Get the account
	account, err := s.storage.Account().GetAccount(ctx, canonicalUsername)
	if err != nil {
		s.logger.Error("accounts service: storage GetAccount failed",
			zap.String("username", canonicalUsername),
			zap.Error(err))
		return nil, ErrGetAccount
	}

	// Check if account is suspended or deleted - hide from public
	if account.User.Suspended {
		s.logger.Warn("accounts service: account suspended",
			zap.String("username", username))
		return nil, ErrAccountNotFound // Don't reveal it's suspended
	}

	s.logger.Info("accounts service: account retrieved",
		zap.String("username", canonicalUsername))

	s.hydrateAccountActor(account)

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
	if err := common.ValidateRequiredParam("query", strings.TrimSpace(query.Query)); err != nil {
		return nil, ErrEmptySearchQuery
	}

	// Perform the search
	result, err := s.storage.Account().SearchAccounts(ctx, query.Query, query.Pagination)
	if err != nil {
		return nil, ErrSearchAccounts
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
	// Use centralized validation patterns from business logic
	if err := common.ValidateRequiredParam("username", cmd.Username); err != nil {
		return ErrUsernameRequired
	}

	if err := common.ValidateRequiredParam("updater_id", cmd.UpdaterID); err != nil {
		return ErrUpdaterIDRequired
	}

	// Use Mastodon business logic for profile validation
	if err := s.mastodonLogic.ValidateDisplayName(cmd.DisplayName); err != nil {
		return err
	}
	if err := s.mastodonLogic.ValidateBio(cmd.Bio); err != nil {
		return err
	}

	// Use business logic pattern for field validation
	if err := common.ValidateSliceLength("fields", cmd.Fields, 4); err != nil {
		return err
	}

	for _, field := range cmd.Fields {
		if err := common.ValidateRequiredParam("field_name", strings.TrimSpace(field.Name)); err != nil {
			return ErrProfileFieldNameEmpty
		}
		if err := common.ValidateStringLength("field_name", field.Name, 0, 255); err != nil {
			return ErrProfileFieldNameTooLong
		}
		if err := common.ValidateStringLength("field_value", field.Value, 0, 255); err != nil {
			return ErrProfileFieldValueTooLong
		}
	}

	return nil
}

func (s *Service) validateUpdatePreferencesCommand(_ context.Context, cmd *UpdatePreferencesCommand) error {
	if err := common.ValidateRequiredParam("username", cmd.Username); err != nil {
		return ErrUsernameRequired
	}

	if err := common.ValidateRequiredParam("updater_id", cmd.UpdaterID); err != nil {
		return ErrUpdaterIDRequired
	}

	if cmd.DefaultPostingVisibility != "" && !isValidPostingVisibility(cmd.DefaultPostingVisibility) {
		return common.ErrValidation("default_posting_visibility", fmt.Sprintf("Visibility %q is not valid", cmd.DefaultPostingVisibility)).InternalError
	}

	if cmd.DirectMessagesFrom != "" {
		switch strings.ToUpper(strings.TrimSpace(cmd.DirectMessagesFrom)) {
		case "FOLLOWING_ONLY", "ANYONE":
			// ok
		default:
			return common.ErrValidation("direct_messages_from", fmt.Sprintf("direct_messages_from %q is not valid", cmd.DirectMessagesFrom)).InternalError
		}
	}

	validExpandMedia := map[string]bool{
		"default":  true,
		"show_all": true,
		"hide_all": true,
	}

	if cmd.ExpandMedia != "" && !validExpandMedia[cmd.ExpandMedia] {
		return ErrInvalidExpandMediaSetting
	}

	validTimelineOrder := map[string]bool{
		"newest": true,
		"oldest": true,
	}

	if cmd.PreferredTimelineOrder != "" && !validTimelineOrder[cmd.PreferredTimelineOrder] {
		return ErrInvalidTimelineOrder
	}

	return nil
}

func (s *Service) hydrateAccountActor(account *storage.Account) {
	if account == nil || account.User == nil {
		return
	}

	baseURL := s.normalizeBaseURL(s.domainName)
	if baseURL == "" {
		baseURL = s.normalizeBaseURL(account.User.URL)
	}

	account.Actor = activitypubutil.BuildLocalActor(account.User.Username, baseURL, account.User, account.Actor)
}

func (s *Service) normalizeBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return fmt.Sprintf("https://%s", trimmed)
}

func (s *Service) updateAccountProfile(account *storage.Account, cmd *UpdateProfileCommand) error {
	sanitizedBio := ""
	if cmd.Bio != "" {
		sanitizedBio = strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.Bio))
	}

	// Update User fields
	if cmd.DisplayName != "" {
		account.User.DisplayName = cmd.DisplayName
	}
	if cmd.Bio != "" {
		account.User.Note = sanitizedBio
	}
	if cmd.Avatar != "" {
		account.User.Avatar = cmd.Avatar
	}
	if cmd.Header != "" {
		account.User.Header = cmd.Header
	}

	account.User.Locked = cmd.Locked
	account.User.Discoverable = cmd.Discoverable

	if len(cmd.Fields) > 0 {
		fields := make([]map[string]string, 0, len(cmd.Fields))
		for _, field := range cmd.Fields {
			sanitizedValue := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(field.Value))
			fields = append(fields, map[string]string{
				"name":  field.Name,
				"value": sanitizedValue,
			})
		}
		account.User.Fields = fields
	}

	// Update Actor fields (ActivityPub profile)
	if account.Actor == nil {
		actorType := activitypub.PersonType
		if account.User != nil && account.User.IsAgent {
			actorType = activitypub.ServiceType
		}

		// Initialize Actor if missing (shouldn't happen, but handle gracefully)
		account.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				Type:    actorType,
				ID:      fmt.Sprintf("https://%s/users/%s", s.domainName, account.User.Username),
			},
			PreferredUsername: account.User.Username,
		}
		s.logger.Warn("account missing Actor, initializing",
			zap.String("username", account.User.Username))
	}

	if cmd.DisplayName != "" {
		account.Actor.Name = cmd.DisplayName
	}

	if cmd.Bio != "" {
		account.Actor.Summary = sanitizedBio
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
	if account.User != nil && account.User.IsAgent {
		account.Actor.Type = activitypub.ServiceType
	} else if cmd.Bot {
		account.Actor.Type = activitypub.ServiceType
	} else {
		account.Actor.Type = activitypub.PersonType
	}

	// Update profile fields using Attachment format
	if len(cmd.Fields) > 0 {
		attachments := make([]activitypub.Attachment, len(cmd.Fields))
		for i, field := range cmd.Fields {
			sanitizedValue := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(field.Value))
			attachments[i] = activitypub.Attachment{
				Type:  "PropertyValue",
				Name:  field.Name,
				Value: sanitizedValue,
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

func accountStreamingPayload(payload map[string]interface{}, public bool) map[string]interface{} {
	if len(payload) == 0 {
		return payload
	}

	cloned := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if public {
			switch typed := value.(type) {
			case *storage.Account:
				cloned[key] = publicAccountForStreaming(typed)
				continue
			case storage.Account:
				cloned[key] = *publicAccountForStreaming(&typed)
				continue
			}
		}
		cloned[key] = value
	}
	return cloned
}

func publicAccountForStreaming(account *storage.Account) *storage.Account {
	if account == nil {
		return nil
	}

	sanitized := &storage.Account{PrivateKey: ""}
	if account.User != nil {
		userCopy := *account.User
		userCopy.Email = ""
		userCopy.PasswordHash = ""
		userCopy.RecoveryMethods = nil
		userCopy.Metadata = nil
		userCopy.Locale = ""
		userCopy.Role = ""
		userCopy.AllowNSFW = false
		userCopy.RequireNSFWWarning = false
		userCopy.AgentCapabilities = nil
		userCopy.AgentOwner = ""
		userCopy.AgentCreatedBy = ""
		sanitized.User = &userCopy
	}
	if account.Actor != nil {
		actorCopy := *account.Actor
		sanitized.Actor = &actorCopy
	}
	return sanitized
}

func (s *Service) emitAccountUpdatedEvents(ctx context.Context, account *storage.Account) []*streaming.Event {
	// Use centralized business logic for event creation
	businessEvents := common.EmitEntityUpdatedEvents(ctx, "account", account.User.Username, account.User.Username, account, map[string]interface{}{
		"profile_fields": account.User.Fields,
		"last_updated":   time.Now(),
	})

	// Convert to streaming events and emit
	var streamingEvents []*streaming.Event
	for _, businessEvent := range businessEvents {
		streamingEvent := &streaming.Event{
			Type:      businessEvent.Type,
			Stream:    fmt.Sprintf("user:%s", account.User.Username),
			Timestamp: businessEvent.Timestamp,
			Payload:   accountStreamingPayload(businessEvent.Metadata, false),
		}

		// Emit to user's stream
		if err := s.publisher.PublishToUser(ctx, account.User.Username, streamingEvent); err != nil {
			s.logger.Error("failed to publish account update to user stream", zap.Error(err))
		} else {
			streamingEvents = append(streamingEvents, streamingEvent)
		}

		// Also emit to followers' streams
		followersEvent := *streamingEvent
		followersEvent.Stream = fmt.Sprintf("followers:%s", account.User.Username)
		followersEvent.Payload = accountStreamingPayload(businessEvent.Metadata, true)
		if err := s.publisher.PublishToStream(ctx, followersEvent.Stream, &followersEvent); err != nil {
			s.logger.Error("failed to publish to followers stream", zap.Error(err))
		} else {
			streamingEvents = append(streamingEvents, &followersEvent)
		}
	}

	return streamingEvents
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
			Context:   activitypub.Context,
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
		return nil, ErrAccountNotFound
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
		return nil, ErrAccountNotFound
	}

	// Get followers using relationship repository
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return nil, ErrRelationshipRepositoryNotAvailable
	}

	// Get actor for the target account
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, ErrActorRepositoryNotAvailable
	}

	targetActor, err := actorRepo.GetActor(ctx, query.Username)
	if err != nil {
		return nil, ErrGetActor
	}

	// Get followers
	followerIDs, cursor, err := relationshipRepo.GetFollowers(ctx, targetActor.ID, query.Pagination.Limit, query.Pagination.Cursor)
	if err != nil {
		return nil, ErrGetFollowersAccounts
	}

	// Convert follower IDs to accounts
	accountRepo := s.storage.Account()
	followers := make([]*storage.Account, 0, len(followerIDs))
	for _, followerID := range followerIDs {
		// Extract username from actor ID (format: https://domain/users/username)
		parts := strings.Split(followerID, "/")
		if err := common.ValidateSliceNotEmpty("follower_id_parts", parts); err == nil {
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
		return nil, ErrAccountNotFound
	}

	// Get following relationships from storage
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return nil, ErrRelationshipRepositoryNotAvailable
	}

	followingUsernames, nextCursor, err := relationshipRepo.GetFollowing(ctx, query.Username, query.Pagination.Limit, query.Pagination.Cursor)
	if err != nil {
		return nil, ErrGetFollowingList
	}

	// Convert relationship results to accounts
	accounts := make([]*storage.Account, 0, len(followingUsernames))
	for _, username := range followingUsernames {
		// Get full account details
		account, err := s.storage.Account().GetAccount(ctx, username)
		if err != nil {
			s.logger.Warn("failed to get account details for following user",
				zap.String("username", username),
				zap.Error(err))
			continue // Skip accounts we can't fetch
		}
		accounts = append(accounts, account)
	}

	// Build pagination result
	pagination := &interfaces.PaginatedResult[*storage.Account]{
		Items:      accounts,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		Total:      int64(len(accounts)), // Note: This is only current page count, not total
	}

	return &FollowingResult{
		Following:  accounts,
		Pagination: pagination,
		Events:     []*streaming.Event{},
	}, nil
}

// GetFamiliarFollowers returns accounts that the requesting user follows and who also follow the given accounts
func (s *Service) GetFamiliarFollowers(ctx context.Context, query *GetFamiliarFollowersQuery) (*FamiliarFollowersResult, error) {
	s.logger.Debug("getting familiar followers",
		zap.Strings("account_ids", query.AccountIDs),
		zap.String("viewer_id", query.ViewerID))

	// Get relationship repository
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return nil, ErrRelationshipRepositoryNotAvailable
	}

	// Get actor repository
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, ErrActorRepositoryNotAvailable
	}

	// Get viewer's actor to get who they follow
	viewerActor, err := actorRepo.GetActor(ctx, query.ViewerID)
	if err != nil {
		return nil, ErrGetViewerActor
	}

	// Get who the viewer follows (their following list)
	viewerFollowing, _, err := relationshipRepo.GetFollowing(ctx, viewerActor.ID, 1000, "") // Get up to 1000 following
	if err != nil {
		return nil, ErrGetViewerFollowing
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
				if err := common.ValidateSliceNotEmpty("follower_id_parts", parts); err == nil {
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
		return nil, common.ErrForbidden(ErrCannotPinAccountForOtherUser)
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, ErrTargetAccountNotFound
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
			return nil, ErrAccountAlreadyPinned
		}
		return nil, ErrPinAccount
	}

	// Get actual relationship status after pinning
	relationship, err := s.getAccountRelationship(ctx, cmd.Username, cmd.TargetAccount)
	if err != nil {
		s.logger.Error("failed to get relationship status after pinning",
			zap.String("username", cmd.Username),
			zap.String("target", cmd.TargetAccount),
			zap.Error(err))
		// Return basic relationship data even if we can't get full details
		relationship = map[string]any{
			"id":          cmd.TargetAccount,
			"endorsed":    true,
			"following":   false,
			"followed_by": false,
		}
	} else {
		// Ensure endorsed is set to true since we just pinned
		relationship["endorsed"] = true
	}

	return &RelationshipResult{
		Relationship: relationship,
		Events:       []*streaming.Event{},
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
		return nil, common.ErrForbidden(ErrCannotUnpinAccountForOtherUser)
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, ErrTargetAccountNotFound
	}

	// Delete pin using repository
	if err := s.storage.Account().DeleteAccountPin(ctx, cmd.Username, targetAccount.Actor.ID); err != nil {
		return nil, ErrUnpinAccount
	}

	// Get complete relationship status
	relationship, err := s.getAccountRelationship(ctx, cmd.Username, cmd.TargetAccount)
	if err != nil {
		s.logger.Warn("failed to get relationship status after unpinning",
			zap.String("username", cmd.Username),
			zap.String("target", cmd.TargetAccount),
			zap.Error(err))
		// Return minimal relationship data as fallback
		relationship = map[string]any{
			"id":       cmd.TargetAccount,
			"endorsed": false,
		}
	}

	return &RelationshipResult{
		Relationship: relationship,
		Events:       []*streaming.Event{},
	}, nil
}

// GetAccountPins retrieves all accounts pinned by a user (endorsements)
func (s *Service) GetAccountPins(ctx context.Context, query *GetAccountPinsQuery) (*AccountPinsResult, error) {
	s.logger.Info("getting account pins",
		zap.String("username", query.Username))

	// Get account pins from storage
	pins, err := s.storage.Social().GetAccountPins(ctx, query.Username)
	if err != nil {
		return nil, ErrGetAccountPins
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
		if err := common.ValidateRequiredParam("username", username); err != nil {
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
		return nil, common.ErrForbidden(ErrCannotSetNoteForOtherUser)
	}

	// Verify target account exists
	targetAccount, err := s.storage.Account().GetAccount(ctx, cmd.TargetAccount)
	if err != nil {
		return nil, ErrTargetAccountNotFound
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
		return nil, ErrSetAccountNote
	}

	// Get complete relationship status including the new note
	relationship, err := s.getAccountRelationship(ctx, cmd.Username, cmd.TargetAccount)
	if err != nil {
		s.logger.Warn("failed to get relationship status after setting note",
			zap.String("username", cmd.Username),
			zap.String("target", cmd.TargetAccount),
			zap.Error(err))
		// Return minimal relationship data with the note as fallback
		relationship = map[string]any{
			"id":   cmd.TargetAccount,
			"note": cmd.Note,
		}
	}

	return &RelationshipResult{
		Relationship: relationship,
		Events:       []*streaming.Event{},
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
		return nil, common.ErrForbidden(ErrCannotRemoveFollowerForOtherUser)
	}

	// Get relationship repository
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return nil, ErrRelationshipRepositoryNotAvailable
	}

	// Get actor repository
	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, ErrActorRepositoryNotAvailable
	}

	// Get actors for both accounts
	removerActor, err := actorRepo.GetActor(ctx, cmd.RemoverID)
	if err != nil {
		return nil, ErrGetActor
	}

	followerActor, err := actorRepo.GetActor(ctx, cmd.FollowerID)
	if err != nil {
		return nil, ErrGetActor
	}

	// Remove the follower relationship
	// Extract usernames from actor IDs for the repository method
	followerUsername := cmd.FollowerID
	removerUsername := cmd.RemoverID
	err = relationshipRepo.DeleteRelationship(ctx, followerUsername, removerUsername)
	if err != nil {
		return nil, ErrRemoveFollower
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
			"id":                   cmd.FollowerID,
			"following":            following,
			"followed_by":          followedBy,
			"blocking":             s.checkBlocking(ctx, relationshipRepo, cmd.RemoverID, cmd.FollowerID),
			"blocked_by":           s.checkBlocking(ctx, relationshipRepo, cmd.FollowerID, cmd.RemoverID),
			"muting":               false,
			"muting_notifications": false,
			"requested":            false,
			"domain_blocking":      false,
			"showing_reblogs":      true,
			"endorsed":             false,
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
		return nil, ErrAccountNotFound
	}

	// Check privacy permissions
	if !s.canViewCollection(query, account) {
		return s.createEmptyCollection(account.Actor.ID, query.CollectionType), nil
	}

	collectionID := fmt.Sprintf("%s/%s", account.Actor.ID, query.CollectionType)

	// Return collection metadata if not requesting a page
	if !query.Page {
		return s.buildCollectionMetadata(ctx, query, collectionID)
	}

	// Return page data with actual follower/following data
	return s.buildCollectionPage(ctx, query, collectionID)
}

// canViewCollection checks if the viewer has permission to see the collection
func (s *Service) canViewCollection(query *GetActivityPubCollectionQuery, account *storage.Account) bool {
	if query.CollectionType != collectionFollowers {
		return true
	}
	if !account.Actor.ManuallyApprovesFollowers {
		return true
	}
	return query.ViewerID == query.Username
}

// createEmptyCollection returns an empty collection for privacy protection
func (s *Service) createEmptyCollection(actorID, collectionType string) *ActivityPubCollectionResult {
	return &ActivityPubCollectionResult{
		Collection: map[string]any{
			"@context":   "https://www.w3.org/ns/activitystreams",
			"id":         fmt.Sprintf("%s/%s", actorID, collectionType),
			"type":       "OrderedCollection",
			"totalItems": 0,
		},
		Events: []*streaming.Event{},
	}
}

// buildCollectionMetadata creates collection metadata with total counts
func (s *Service) buildCollectionMetadata(ctx context.Context, query *GetActivityPubCollectionQuery, collectionID string) (*ActivityPubCollectionResult, error) {
	totalItems := s.getCollectionCount(ctx, query)

	collection := map[string]any{
		"@context":   "https://www.w3.org/ns/activitystreams",
		"id":         collectionID,
		"type":       "OrderedCollection",
		"totalItems": totalItems,
	}

	if totalItems > 0 {
		collection["first"] = fmt.Sprintf("%s?page=1", collectionID)
	}

	return &ActivityPubCollectionResult{
		Collection: collection,
		Events:     []*streaming.Event{},
	}, nil
}

// getCollectionCount returns the total count for the collection type
func (s *Service) getCollectionCount(ctx context.Context, query *GetActivityPubCollectionQuery) int {
	switch query.CollectionType {
	case collectionFollowers:
		relationshipRepo := s.storage.Relationship()
		if isNilInterface(relationshipRepo) {
			s.logger.Warn("relationship repository unavailable for followers count")
			return 0
		}
		if count, err := relationshipRepo.CountFollowers(ctx, query.Username); err == nil {
			return count
		}
		s.logger.Warn("failed to get followers count")
	case "following":
		relationshipRepo := s.storage.Relationship()
		if isNilInterface(relationshipRepo) {
			s.logger.Warn("relationship repository unavailable for following count")
			return 0
		}
		if count, err := relationshipRepo.CountFollowing(ctx, query.Username); err == nil {
			return count
		}
		s.logger.Warn("failed to get following count")
	}
	return 0
}

// buildCollectionPage creates a collection page with actual data
func (s *Service) buildCollectionPage(ctx context.Context, query *GetActivityPubCollectionQuery, collectionID string) (*ActivityPubCollectionResult, error) {
	pageID := s.buildPageID(collectionID, query.Cursor)
	orderedItems, nextPageID := s.getPageData(ctx, query, collectionID)

	page := map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           pageID,
		"type":         "OrderedCollectionPage",
		"partOf":       collectionID,
		"orderedItems": orderedItems,
	}

	if nextPageID != "" {
		page["next"] = nextPageID
	}

	return &ActivityPubCollectionResult{
		Collection: page,
		Events:     []*streaming.Event{},
	}, nil
}

// buildPageID constructs the page ID with optional cursor
func (s *Service) buildPageID(collectionID, cursor string) string {
	pageID := fmt.Sprintf("%s?page=1", collectionID)
	if cursor != "" {
		pageID = fmt.Sprintf("%s&cursor=%s", pageID, cursor)
	}
	return pageID
}

// getPageData fetches and converts usernames to ActivityPub actor URLs
func (s *Service) getPageData(ctx context.Context, query *GetActivityPubCollectionQuery, collectionID string) ([]any, string) {
	switch query.CollectionType {
	case collectionFollowers:
		return s.getFollowersPageData(ctx, query, collectionID)
	case "following":
		return s.getFollowingPageData(ctx, query, collectionID)
	default:
		return []any{}, ""
	}
}

// getFollowersPageData handles followers collection page data
func (s *Service) getFollowersPageData(ctx context.Context, query *GetActivityPubCollectionQuery, collectionID string) ([]any, string) {
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		s.logger.Error("relationship repository unavailable for ActivityPub followers collection")
		return []any{}, ""
	}

	usernames, nextCursor, err := relationshipRepo.GetFollowers(ctx, query.Username, query.Limit, query.Cursor)
	if err != nil {
		s.logger.Error("failed to get followers for ActivityPub collection", zap.Error(err))
		return []any{}, ""
	}

	orderedItems := s.convertUsernamesToActorIDs(ctx, usernames, "follower")
	nextPageID := s.buildNextPageID(collectionID, nextCursor)
	return orderedItems, nextPageID
}

// getFollowingPageData handles following collection page data
func (s *Service) getFollowingPageData(ctx context.Context, query *GetActivityPubCollectionQuery, collectionID string) ([]any, string) {
	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		s.logger.Error("relationship repository unavailable for ActivityPub following collection")
		return []any{}, ""
	}

	usernames, nextCursor, err := relationshipRepo.GetFollowing(ctx, query.Username, query.Limit, query.Cursor)
	if err != nil {
		s.logger.Error("failed to get following for ActivityPub collection", zap.Error(err))
		return []any{}, ""
	}

	orderedItems := s.convertUsernamesToActorIDs(ctx, usernames, "following")
	nextPageID := s.buildNextPageID(collectionID, nextCursor)
	return orderedItems, nextPageID
}

// convertUsernamesToActorIDs converts a list of usernames to ActivityPub actor IDs
func (s *Service) convertUsernamesToActorIDs(ctx context.Context, usernames []string, logContext string) []any {
	orderedItems := make([]any, 0, len(usernames))

	for _, username := range usernames {
		account, err := s.storage.Account().GetAccount(ctx, username)
		if err != nil {
			s.logger.Warn("failed to get account details",
				zap.String("username", username),
				zap.String("context", logContext),
				zap.Error(err))
			continue
		}
		orderedItems = append(orderedItems, account.Actor.ID)
	}

	return orderedItems
}

// buildNextPageID constructs the next page URL if cursor is available
func (s *Service) buildNextPageID(collectionID, nextCursor string) string {
	if err := common.ValidateRequiredParam("next_cursor", nextCursor); err != nil {
		return ""
	}
	return fmt.Sprintf("%s?page=1&cursor=%s", collectionID, nextCursor)
}

// RegisterAccount creates a new user account with actor
func (s *Service) RegisterAccount(ctx context.Context, cmd *RegisterAccountCommand) (*RegisterAccountResult, error) {
	cmd.Username = s.normalizeUsername(cmd.Username)

	// Validate command
	if err := s.validateRegisterAccountCommand(ctx, cmd); err != nil {
		return nil, ErrValidationFailed
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return nil, ErrAccountRepositoryNotAvailable
	}

	// Check if username is already taken
	existingAccount, _ := accountRepo.GetAccount(ctx, cmd.Username)
	if existingAccount != nil {
		return nil, ErrUsernameAlreadyTaken
	}

	username := cmd.Username
	registrationChallengeID := strings.TrimSpace(cmd.RegistrationChallengeID)
	passkeyRegistrationProofID := strings.TrimSpace(cmd.PasskeyRegistrationProof)

	passkeyProof, err := s.loadRegistrationPasskeyProof(ctx, accountRepo, username, passkeyRegistrationProofID)
	if err != nil {
		return nil, err
	}

	// Generate RSA keypair for the actor
	privateKey, err := s.generateRSAKeyPair()
	if err != nil {
		s.logger.Error("failed to generate RSA keypair", zap.Error(err))
		return nil, ErrGenerateKeypair
	}

	// Encode public key to PEM format
	publicKeyPEM, err := s.encodePublicKeyPEM(privateKey)
	if err != nil {
		s.logger.Error("failed to encode public key", zap.Error(err))
		return nil, ErrEncodePublicKey
	}

	// Encode private key to PEM format for storage
	privateKeyPEMBytes, err := s.crypto.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		s.logger.Error("failed to encode private key", zap.Error(err))
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}
	privateKeyPEM := string(privateKeyPEMBytes)

	// Create user object
	user := &storage.User{
		Username:     username,
		Email:        cmd.Email,
		PasswordHash: "",   // Will be set if password provided
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
			return nil, ErrHashPassword
		}
		user.PasswordHash = passwordHash
	}

	// Create corresponding actor
	actorID := fmt.Sprintf("https://%s/users/%s", s.domainName, username)
	actor := activitypub.NewActor(activitypub.PersonType, actorID, username)
	actor.Name = username
	actor.URL = fmt.Sprintf("https://%s/@%s", s.domainName, username)
	actor.CreatedAt = &user.CreatedAt
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: string(publicKeyPEM),
	}

	// Set manifest-driven local actor identifiers.
	activitypubutil.ApplyLocalActorIdentifiers(actor, fmt.Sprintf("https://%s", s.domainName), username)

	// Create account with actor
	account := &storage.Account{
		User:       user,
		Actor:      actor,
		PrivateKey: privateKeyPEM, // Pass private key for actor creation
	}

	// Save to storage
	if err := accountRepo.CreateAccountIfNotExists(ctx, account); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil, ErrUsernameAlreadyTaken
		}
		s.logger.Error("failed to create account",
			zap.String("username", username),
			zap.Error(err))
		// Return the actual error instead of wrapping it
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	initialVisibility := s.initialPostingVisibility(cmd.DefaultPostingVisibility)
	if err := s.ensureQuotePermissionsForNewUser(ctx, s.storage.Quote(), username, initialVisibility); err != nil {
		s.rollbackAccountCreation(ctx, accountRepo, username, err)
		return nil, fmt.Errorf("failed to create default quote permissions: %w", err)
	}

	if err := s.persistDefaultPostingVisibility(ctx, accountRepo, username, initialVisibility); err != nil {
		s.rollbackAccountCreation(ctx, accountRepo, username, err)
		return nil, err
	}

	if err := s.finalizeRegistrationProof(ctx, accountRepo, username, registrationChallengeID, passkeyProof); err != nil {
		return nil, err
	}

	s.logger.Info("account created successfully, recording activity",
		zap.String("username", username))

	// Record registration activity for metrics
	if err := s.storage.Activity().RecordActivity(ctx, "registration", actor.ID, time.Now()); err != nil {
		// Log the error but don't fail the request
		s.logger.Warn("failed to record registration activity", zap.Error(err))
	}

	s.logger.Info("emitting account created events",
		zap.String("username", username))

	// Create events for streaming
	events := s.emitAccountCreatedEvents(ctx, account)

	s.logger.Info("returning registration result",
		zap.String("username", username),
		zap.String("actor_id", actor.ID))

	return &RegisterAccountResult{
		Account: account,
		Actor:   actor,
		Events:  events,
	}, nil
}

// validateRegisterAccountCommand validates the registration command
func (s *Service) validateRegisterAccountCommand(_ context.Context, cmd *RegisterAccountCommand) error {
	if err := common.ValidateRequiredParam("username", cmd.Username); err != nil {
		return ErrUsernameRequired
	}
	if err := common.ValidateStringLength("username", cmd.Username, 3, 30); err != nil {
		return err
	}
	// Email is deprecated and disallowed - must be empty for wallet-based registration
	if cmd.Email != "" {
		return errors.New("email is not supported - wallet authentication only")
	}
	if !cmd.Agreement {
		return ErrMustAgreeToTerms
	}

	if cmd.DefaultPostingVisibility != "" && !isValidPostingVisibility(cmd.DefaultPostingVisibility) {
		return common.ErrValidation("default_posting_visibility", fmt.Sprintf("Visibility '%s' is not valid", cmd.DefaultPostingVisibility)).InternalError
	}
	hasWalletChallenge := strings.TrimSpace(cmd.RegistrationChallengeID) != ""
	hasPasskeyProof := strings.TrimSpace(cmd.PasskeyRegistrationProof) != ""
	if hasWalletChallenge == hasPasskeyProof {
		return errors.New("exactly one of wallet challenge and passkey registration proof must be provided")
	}
	// Additional validation can be added here
	return nil
}

func isValidPostingVisibility(value string) bool {
	switch value {
	case models.VisibilityPublic, models.VisibilityUnlisted, models.VisibilityPrivate, models.VisibilityDirect:
		return true
	default:
		return false
	}
}

func (s *Service) initialPostingVisibility(requested string) string {
	if requested != "" {
		return strings.ToLower(requested)
	}

	defaultPrefs := models.GetDefaultPreferences()
	if defaultPrefs != nil && defaultPrefs.DefaultPostingVisibility != "" {
		return strings.ToLower(defaultPrefs.DefaultPostingVisibility)
	}

	return models.VisibilityPublic
}

func (s *Service) loadRegistrationPasskeyProof(ctx context.Context, repo *repositories.AccountRepository, username, proofID string) (*models.PasskeyRegistrationProof, error) {
	if strings.TrimSpace(proofID) == "" {
		return nil, nil
	}

	proof, err := repo.GetPasskeyRegistrationProof(ctx, proofID)
	if err != nil {
		return nil, fmt.Errorf("failed to get passkey registration proof: %w", err)
	}
	if strings.TrimSpace(proof.Username) != username {
		return nil, fmt.Errorf("passkey registration proof was created for a different username")
	}

	return proof, nil
}

func (s *Service) finalizeRegistrationProof(ctx context.Context, repo *repositories.AccountRepository, username, walletChallengeID string, passkeyProof *models.PasskeyRegistrationProof) error {
	if passkeyProof != nil {
		return s.finalizePasskeyRegistrationProof(ctx, repo, username, passkeyProof)
	}
	if strings.TrimSpace(walletChallengeID) == "" {
		return nil
	}

	if err := repo.MarkWalletChallengeRegistrationCompleted(ctx, walletChallengeID); err != nil {
		s.rollbackAccountCreation(ctx, repo, username, err)
		return fmt.Errorf("failed to finalize wallet registration challenge: %w", err)
	}

	return nil
}

func (s *Service) finalizePasskeyRegistrationProof(ctx context.Context, repo *repositories.AccountRepository, username string, passkeyProof *models.PasskeyRegistrationProof) error {
	credential := passkeyRegistrationProofToCredential(passkeyProof)
	if err := repo.StoreWebAuthnCredential(ctx, credential); err != nil {
		s.rollbackAccountCreation(ctx, repo, username, err)
		return fmt.Errorf("failed to store initial passkey credential: %w", err)
	}

	if _, err := repo.ConsumePasskeyRegistrationProof(ctx, passkeyProof.ID, passkeyProof.Username, passkeyProof.CeremonyID); err != nil {
		s.rollbackPasskeyCredentialCreation(ctx, repo, username, credential.ID, err)
		s.rollbackAccountCreation(ctx, repo, username, err)
		return fmt.Errorf("failed to finalize passkey registration proof: %w", err)
	}

	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}

func (s *Service) ensureQuotePermissionsForNewUser(ctx context.Context, repo quotePermissionsCreator, username, visibility string) error {
	if isNilInterface(repo) {
		return ErrQuoteRepositoryNotAvailable
	}

	perms := &models.QuotePermissions{
		Username: username,
	}
	perms.ApplyVisibilityDefaults(visibility)

	if err := perms.UpdateKeys(); err != nil {
		return err
	}

	return repo.CreateQuotePermissions(ctx, perms)
}

func (s *Service) persistDefaultPostingVisibility(ctx context.Context, repo accountRegistrationRepository, username, visibility string) error {
	if isNilInterface(repo) {
		return ErrAccountRepositoryNotAvailable
	}

	preferences := map[string]interface{}{
		"default_posting_visibility": visibility,
	}

	if err := repo.UpdateAccountPreferences(ctx, username, preferences); err != nil {
		return fmt.Errorf("failed to store default posting visibility: %w", err)
	}

	return nil
}

func (s *Service) rollbackAccountCreation(ctx context.Context, repo accountRegistrationRepository, username string, cause error) {
	if isNilInterface(repo) {
		s.logger.Error("unable to rollback account creation; account repository unavailable",
			zap.String("username", username),
			zap.NamedError("cause", cause))
		return
	}

	if err := repo.DeleteAccount(ctx, username); err != nil {
		s.logger.Error("failed to rollback account after registration failure",
			zap.String("username", username),
			zap.NamedError("rollback_error", err),
			zap.NamedError("cause", cause))
		return
	}

	s.logger.Warn("rolled back account after registration failure",
		zap.String("username", username),
		zap.NamedError("cause", cause))
}

func (s *Service) rollbackPasskeyCredentialCreation(ctx context.Context, repo *repositories.AccountRepository, username string, credentialID string, cause error) {
	if strings.TrimSpace(credentialID) != "" {
		if err := repo.DeleteWebAuthnCredential(ctx, credentialID); err != nil {
			s.logger.Error("failed to rollback passkey credential after registration failure",
				zap.String("username", username),
				zap.String("credential_id", credentialID),
				zap.NamedError("rollback_error", err),
				zap.NamedError("cause", cause))
		}
	}
}

func passkeyRegistrationProofToCredential(proof *models.PasskeyRegistrationProof) *storage.WebAuthnCredential {
	now := time.Now().UTC()
	return &storage.WebAuthnCredential{
		ID:              proof.CredentialID,
		UserID:          strings.TrimSpace(proof.Username),
		PublicKey:       append([]byte(nil), proof.PublicKey...),
		AttestationType: proof.AttestationType,
		AAGUID:          append([]byte(nil), proof.AAGUID...),
		SignCount:       proof.SignCount,
		CloneWarning:    proof.CloneWarning,
		BackupEligible:  proof.BackupEligible,
		BackupState:     proof.BackupState,
		CreatedAt:       now,
		LastUsedAt:      now,
		Name:            "Passkey 1",
	}
}

// Helper methods for account registration
func (s *Service) generateRSAKeyPair() (interface{}, error) {
	if s.crypto == nil {
		return nil, ErrCryptoServiceNotConfigured
	}
	return s.crypto.GenerateRSAKeyPair(4096)
}

func (s *Service) encodePublicKeyPEM(publicKey interface{}) ([]byte, error) {
	if s.crypto == nil {
		return nil, ErrCryptoServiceNotConfigured
	}
	return s.crypto.EncodePublicKeyPEM(publicKey)
}

func (s *Service) hashPassword(password string) (string, error) {
	if s.auth == nil {
		return "", ErrAuthServiceNotConfigured
	}
	return s.auth.HashPassword(password)
}

// emitAccountCreatedEvents creates events for account creation
func (s *Service) emitAccountCreatedEvents(ctx context.Context, account *storage.Account) []*streaming.Event {
	var events []*streaming.Event

	// Skip event emission if publisher is not configured (optional dependency)
	if s.publisher == nil {
		s.logger.Debug("publisher not configured, skipping event emission for account creation")
		return events
	}

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
	if err := common.ValidateRequiredParam("username", query.Username); err != nil {
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
	if err := common.ValidateRequiredParam("username", cmd.Username); err != nil {
		return nil, &ValidationError{Field: "username", Message: "required"}
	}
	if err := common.ValidateRequiredParam("timeline", cmd.Timeline); err != nil {
		return nil, &ValidationError{Field: "timeline", Message: "required"}
	}
	if err := common.ValidateRequiredParam("last_read_id", cmd.LastReadID); err != nil {
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
	if err := common.ValidateRequiredParam("state", cmd.State); err != nil {
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
	if err := common.ValidateRequiredParam("client_id", query.ClientID); err != nil {
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
	Resource string
}

// GetUserAppConsentResult contains the result of checking user consent
type GetUserAppConsentResult struct {
	Consent *storage.UserAppConsent
	Events  []*streaming.Event
}

// GetUserAppConsent retrieves user's consent for an OAuth app
func (s *Service) GetUserAppConsent(ctx context.Context, query *GetUserAppConsentQuery) (*GetUserAppConsentResult, error) {
	if err := common.ValidateRequiredParam("username", query.Username); err != nil {
		return nil, &ValidationError{Field: "username", Message: "required"}
	}
	if err := common.ValidateRequiredParam("client_id", query.ClientID); err != nil {
		return nil, &ValidationError{Field: "client_id", Message: "required"}
	}

	consent, err := s.storage.Account().GetUserAppConsent(ctx, query.Username, query.ClientID, query.Resource)
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
			zap.String("resource", query.Resource),
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
	TotalUsers     int
	ActiveMonth    int
	ActiveHalfyear int
	LocalPosts     int
	LocalComments  int
	Events         []*streaming.Event
}

// GetInstanceStats retrieves instance-level statistics
func (s *Service) GetInstanceStats(ctx context.Context, _ *GetInstanceStatsQuery) (*GetInstanceStatsResult, error) {
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
	Actor    *activitypub.Actor     `json:"actor"`
	Metadata *storage.ActorMetadata `json:"metadata"`
}

// GetAccountMetadata retrieves an account with its metadata
func (s *Service) GetAccountMetadata(ctx context.Context, query *GetAccountMetadataQuery) (*GetAccountMetadataResult, error) {
	if s.storage == nil {
		return nil, ErrStorageNotAvailable
	}

	actorRepo := s.storage.Actor()
	if actorRepo == nil {
		return nil, ErrActorRepositoryNotAvailable
	}

	actor, err := actorRepo.GetActor(ctx, query.Username)
	if err != nil {
		return nil, ErrGetActor
	}

	return &GetAccountMetadataResult{
		Actor:    actor,
		Metadata: nil, // Metadata can be added later if needed
	}, nil
}

// IsAccountPinnedQuery represents a request to check if an account is pinned
type IsAccountPinnedQuery struct {
	UserID        string `json:"user_id" validate:"required"`
	PinnedActorID string `json:"pinned_actor_id" validate:"required"`
}

// IsAccountPinned checks if a user has pinned an account
func (s *Service) IsAccountPinned(ctx context.Context, userID, pinnedActorID string) (bool, error) {
	if s.storage == nil {
		return false, ErrStorageNotAvailable
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return false, ErrUserRepositoryNotAvailable
	}

	isPinned, err := userRepo.IsAccountPinned(ctx, userID, pinnedActorID)
	if err != nil {
		return false, ErrCheckAccountPinned
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
		return nil, ErrStorageNotAvailable
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return nil, ErrUserRepositoryNotAvailable
	}

	user, err := userRepo.GetUser(ctx, username)
	if err != nil {
		return nil, ErrGetUser
	}
	return user, nil
}

// GetAgentGovernanceState loads typed governance state for an agent account.
func (s *Service) GetAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	if s.storage == nil {
		return nil, ErrStorageNotAvailable
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return nil, ErrAccountRepositoryNotAvailable
	}

	return accountRepo.GetAgentGovernanceState(ctx, username)
}

// GetAgentGovernanceStatesByUsernames batch-loads typed governance state for agent accounts.
func (s *Service) GetAgentGovernanceStatesByUsernames(ctx context.Context, usernames []string) (map[string]*storage.AgentGovernanceState, error) {
	if s.storage == nil {
		return nil, ErrStorageNotAvailable
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return nil, ErrAccountRepositoryNotAvailable
	}

	return accountRepo.GetAgentGovernanceStatesByUsernames(ctx, usernames)
}

// PutAgentGovernanceState upserts typed governance state for an agent account.
func (s *Service) PutAgentGovernanceState(ctx context.Context, state *storage.AgentGovernanceState) error {
	if s.storage == nil {
		return ErrStorageNotAvailable
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return ErrAccountRepositoryNotAvailable
	}

	return accountRepo.PutAgentGovernanceState(ctx, state)
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
		return "", ErrStorageNotAvailable
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return "", ErrUserRepositoryNotAvailable
	}

	preferences, err := userRepo.GetUserPreferences(ctx, userID)
	if err != nil {
		return "", ErrGetUserPreferences
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
		return "", ErrStorageNotAvailable
	}

	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return "", ErrRelationshipRepositoryNotAvailable
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
		return false, ErrStorageNotAvailable
	}

	domainBlockRepo := s.storage.DomainBlock()
	if domainBlockRepo == nil {
		return false, ErrDomainBlockRepositoryNotAvailable
	}

	// Check user-level domain block (userID is typically the username in this context)
	blocked, err := domainBlockRepo.IsBlockedDomain(ctx, userID, targetDomain)
	if err != nil {
		return false, ErrCheckDomainBlockedByUser
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
		return nil, ErrStorageNotAvailable
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return nil, ErrAccountRepositoryNotAvailable
	}

	field, err := accountRepo.GetFieldVerification(ctx, username, fieldName)
	if err != nil {
		return nil, ErrGetFieldVerification
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
		return "", ErrStorageNotAvailable
	}

	userRepo := s.storage.User()
	if userRepo == nil {
		return "", ErrUserRepositoryNotAvailable
	}

	note, err := userRepo.GetAccountNote(ctx, currentUsername, targetActorID)
	if err != nil {
		return "", ErrGetAccountNote
	}

	if note == nil {
		return "", nil // No note found
	}

	return note.Note, nil
}

// checkBlocking checks if one user has blocked another user
func (s *Service) checkBlocking(ctx context.Context, relationshipRepo interfaces.ConcreteRelationshipRepository, blockerID, blockedID string) bool {
	if isNilInterface(relationshipRepo) {
		return false
	}

	// Check if blockerID has blocked blockedID
	blocked, err := relationshipRepo.IsBlocked(ctx, blockerID, blockedID)
	if err != nil {
		// Log the error but don't fail the request
		s.logger.Warn("failed to check blocking status",
			zap.String("blocker_id", blockerID),
			zap.String("blocked_id", blockedID),
			zap.Error(err))
		return false
	}

	return blocked
}

// getAccountRelationship retrieves the full relationship status between two accounts
func (s *Service) getAccountRelationship(ctx context.Context, username, targetAccount string) (map[string]any, error) {
	if err := s.validateRelationshipStorage(); err != nil {
		return nil, err
	}

	relationshipData := s.buildRelationshipData(ctx, username, targetAccount)

	return map[string]any{
		"id":                   targetAccount,
		"following":            relationshipData.Following,
		"followed_by":          relationshipData.FollowedBy,
		"blocking":             relationshipData.Blocking,
		"blocked_by":           relationshipData.BlockedBy,
		"muting":               relationshipData.Muting,
		"muting_notifications": relationshipData.MutingNotifications,
		"requested":            relationshipData.Requested,
		"domain_blocking":      relationshipData.DomainBlocking,
		"showing_reblogs":      true, // Default to true unless we have specific reblog preference tracking
		"endorsed":             relationshipData.Endorsed,
		"note":                 relationshipData.AccountNote,
	}, nil
}

// relationshipData holds all relationship status information
type relationshipData struct {
	Following           bool
	FollowedBy          bool
	Blocking            bool
	BlockedBy           bool
	Muting              bool
	MutingNotifications bool
	Requested           bool
	DomainBlocking      bool
	Endorsed            bool
	AccountNote         string
}

// validateRelationshipStorage checks if storage and relationship repo are available
func (s *Service) validateRelationshipStorage() error {
	if s.storage == nil {
		return ErrStorageNotAvailable
	}
	if isNilInterface(s.storage.Relationship()) {
		return ErrRelationshipRepositoryNotAvailable
	}
	return nil
}

// buildRelationshipData collects all relationship information
func (s *Service) buildRelationshipData(ctx context.Context, username, targetAccount string) *relationshipData {
	data := &relationshipData{}

	relationshipRepo := s.storage.Relationship()
	if isNilInterface(relationshipRepo) {
		return data
	}

	// Basic relationship checks
	data.Following = s.checkFollowingStatus(ctx, relationshipRepo, username, targetAccount)
	data.FollowedBy = s.checkFollowingStatus(ctx, relationshipRepo, targetAccount, username)
	data.Blocking = s.checkBlocking(ctx, relationshipRepo, username, targetAccount)
	data.BlockedBy = s.checkBlocking(ctx, relationshipRepo, targetAccount, username)

	// Muting checks
	data.Muting = s.checkMutingStatus(ctx, relationshipRepo, username, targetAccount)
	data.MutingNotifications = s.checkMutingNotifications(ctx, relationshipRepo, username, targetAccount, data.Muting)

	// Additional relationship status
	data.Requested = s.checkFollowRequest(ctx, relationshipRepo, username, targetAccount)
	data.DomainBlocking = s.checkDomainBlocking(ctx, username, targetAccount)
	data.Endorsed = s.checkEndorsementStatus(ctx, username, targetAccount)
	data.AccountNote = s.getAccountNoteText(ctx, username, targetAccount)

	return data
}

// checkFollowingStatus checks if one user follows another
func (s *Service) checkFollowingStatus(ctx context.Context, repo interfaces.ConcreteRelationshipRepository, follower, followee string) bool {
	if isNilInterface(repo) {
		return false
	}

	following, err := repo.IsFollowing(ctx, follower, followee)
	if err != nil {
		s.logger.Warn("failed to check following status",
			zap.String("follower", follower),
			zap.String("followee", followee),
			zap.Error(err))
		return false
	}
	return following
}

// checkMutingStatus checks if one user has muted another
func (s *Service) checkMutingStatus(ctx context.Context, repo interfaces.ConcreteRelationshipRepository, muter, muted string) bool {
	if isNilInterface(repo) {
		return false
	}

	muting, err := repo.IsMuted(ctx, muter, muted)
	if err != nil {
		s.logger.Warn("failed to check muting status",
			zap.String("muter", muter),
			zap.String("muted", muted),
			zap.Error(err))
		return false
	}
	return muting
}

// checkMutingNotifications checks if notifications are muted for a muted user
func (s *Service) checkMutingNotifications(ctx context.Context, repo interfaces.ConcreteRelationshipRepository, username, targetAccount string, isMuting bool) bool {
	if !isMuting || isNilInterface(repo) {
		return false
	}

	muteDetails, err := repo.GetMute(ctx, username, targetAccount)
	if err != nil || muteDetails == nil {
		return false
	}

	return muteDetails.HideNotifications || !muteDetails.Notifications
}

// checkFollowRequest checks if there's a pending follow request
func (s *Service) checkFollowRequest(ctx context.Context, repo interfaces.ConcreteRelationshipRepository, requester, target string) bool {
	if isNilInterface(repo) {
		return false
	}

	requested, err := repo.HasFollowRequest(ctx, requester, target)
	if err != nil {
		return false
	}
	return requested
}

// checkDomainBlocking checks if the user has blocked the target's domain
func (s *Service) checkDomainBlocking(ctx context.Context, username, targetAccount string) bool {
	domain := s.extractDomainFromAccount(targetAccount)
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return false
	}

	accountRepo := s.storage.Account()
	if accountRepo == nil {
		return false
	}

	domainBlocking, err := accountRepo.IsBlockedDomain(ctx, username, domain)
	if err != nil {
		return false
	}
	return domainBlocking
}

// extractDomainFromAccount extracts domain from an account identifier
func (s *Service) extractDomainFromAccount(account string) string {
	if !strings.Contains(account, "@") {
		return ""
	}

	parts := strings.Split(account, "@")
	if len(parts) <= 1 {
		return ""
	}

	return parts[len(parts)-1]
}

// checkEndorsementStatus checks if the user has endorsed/pinned the target account
func (s *Service) checkEndorsementStatus(ctx context.Context, username, targetAccount string) bool {
	userRepo := s.storage.User()
	if userRepo == nil {
		return false
	}

	pins, err := userRepo.GetAccountPins(ctx, username)
	if err != nil {
		return false
	}

	for _, pin := range pins {
		if pin.PinnedActorID == targetAccount {
			return true
		}
	}

	return false
}

// getAccountNoteText retrieves the account note for the target account
func (s *Service) getAccountNoteText(ctx context.Context, username, targetAccount string) string {
	userRepo := s.storage.User()
	if userRepo == nil {
		return ""
	}

	note, err := userRepo.GetAccountNote(ctx, username, targetAccount)
	if err != nil || note == nil {
		return ""
	}

	return note.Note
}
