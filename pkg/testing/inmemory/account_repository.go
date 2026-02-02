// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// AccountRepository is a thread-safe in-memory implementation of interfaces.AccountRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type AccountRepository struct {
	mu sync.RWMutex

	// Core account data
	accounts        map[string]*storage.Account // keyed by username
	accountsByEmail map[string]string           // email -> username mapping
	accountsByURL   map[string]string           // actorURL -> username mapping

	// Session data
	sessions          map[string]*storage.Session // sessionID -> session
	sessionsByUser    map[string][]string         // username -> sessionIDs
	sessionsByRefresh map[string]string           // refreshToken -> sessionID

	// Password reset data
	passwordResets map[string]*storage.PasswordReset // token -> reset

	// Device data
	devices       map[string]*storage.Device // deviceID -> device
	devicesByUser map[string][]string        // username -> deviceIDs

	// Recovery tokens
	recoveryTokens map[string]map[string]interface{} // key -> data

	// WebAuthn data
	webAuthnChallenges  map[string]*storage.WebAuthnChallenge  // challengeID -> challenge
	webAuthnCredentials map[string]*storage.WebAuthnCredential // credentialID -> credential

	// Account pins
	accountPins map[string][]*storage.AccountPin // username -> pins

	// Account notes
	accountNotes map[string]map[string]*storage.AccountNote // username -> targetActorID -> note

	// Preferences
	accountPreferences map[string]map[string]interface{} // username -> preferences
	accountFeatures    map[string]map[string]bool        // username -> features

	// Login tracking
	loginAttempts map[string][]*storage.LoginAttempt // username -> attempts
	rateLimits    map[string]*rateLimitEntry         // key -> rate limit entry

	// Follow request state
	followRequestStates map[string]string // "requester:target" -> state

	// Blocked domains
	blockedDomains map[string]map[string]bool // userID -> domain -> blocked

	// Bookmarks
	bookmarks map[string][]*storage.Bookmark // username -> bookmarks
}

// rateLimitEntry tracks rate limiting state
type rateLimitEntry struct {
	count     int
	resetAt   time.Time
	blocked   bool
	blockedAt time.Time
}

// NewAccountRepository creates a new in-memory account repository
func NewAccountRepository() *AccountRepository {
	return &AccountRepository{
		accounts:            make(map[string]*storage.Account),
		accountsByEmail:     make(map[string]string),
		accountsByURL:       make(map[string]string),
		sessions:            make(map[string]*storage.Session),
		sessionsByUser:      make(map[string][]string),
		sessionsByRefresh:   make(map[string]string),
		passwordResets:      make(map[string]*storage.PasswordReset),
		devices:             make(map[string]*storage.Device),
		devicesByUser:       make(map[string][]string),
		recoveryTokens:      make(map[string]map[string]interface{}),
		webAuthnChallenges:  make(map[string]*storage.WebAuthnChallenge),
		webAuthnCredentials: make(map[string]*storage.WebAuthnCredential),
		accountPins:         make(map[string][]*storage.AccountPin),
		accountNotes:        make(map[string]map[string]*storage.AccountNote),
		accountPreferences:  make(map[string]map[string]interface{}),
		accountFeatures:     make(map[string]map[string]bool),
		loginAttempts:       make(map[string][]*storage.LoginAttempt),
		rateLimits:          make(map[string]*rateLimitEntry),
		followRequestStates: make(map[string]string),
		blockedDomains:      make(map[string]map[string]bool),
		bookmarks:           make(map[string][]*storage.Bookmark),
	}
}

// Core account operations

// CreateAccount creates a new account
func (r *AccountRepository) CreateAccount(_ context.Context, account *storage.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if account == nil || account.User == nil {
		return fmt.Errorf("account and user are required")
	}

	username := account.User.Username
	if _, exists := r.accounts[username]; exists {
		return storage.ErrAlreadyExists
	}

	// Make a copy to avoid external mutations
	accountCopy := copyAccount(account)
	r.accounts[username] = accountCopy

	// Index by email if present
	if account.User.Email != "" {
		r.accountsByEmail[account.User.Email] = username
	}

	// Index by actor URL if present
	if account.Actor != nil && account.Actor.ID != "" {
		r.accountsByURL[account.Actor.ID] = username
	}

	return nil
}

// GetAccount retrieves an account by username
func (r *AccountRepository) GetAccount(_ context.Context, username string) (*storage.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyAccount(account), nil
}

// GetAccountByURL retrieves an account by actor URL
func (r *AccountRepository) GetAccountByURL(_ context.Context, actorURL string) (*storage.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	username, exists := r.accountsByURL[actorURL]
	if !exists {
		return nil, storage.ErrNotFound
	}

	account, exists := r.accounts[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyAccount(account), nil
}

// GetAccountByEmail retrieves an account by email
func (r *AccountRepository) GetAccountByEmail(_ context.Context, email string) (*storage.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	username, exists := r.accountsByEmail[email]
	if !exists {
		return nil, storage.ErrNotFound
	}

	account, exists := r.accounts[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return copyAccount(account), nil
}

// UpdateAccount updates an existing account
func (r *AccountRepository) UpdateAccount(_ context.Context, account *storage.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if account == nil || account.User == nil {
		return fmt.Errorf("account and user are required")
	}

	username := account.User.Username
	if _, exists := r.accounts[username]; !exists {
		return storage.ErrNotFound
	}

	accountCopy := copyAccount(account)
	r.accounts[username] = accountCopy

	return nil
}

// DeleteAccount deletes an account
func (r *AccountRepository) DeleteAccount(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists {
		return storage.ErrNotFound
	}

	// Clean up indexes
	if account.User != nil && account.User.Email != "" {
		delete(r.accountsByEmail, account.User.Email)
	}
	if account.Actor != nil && account.Actor.ID != "" {
		delete(r.accountsByURL, account.Actor.ID)
	}

	delete(r.accounts, username)
	return nil
}

// User operations

// GetUser retrieves a user by username
func (r *AccountRepository) GetUser(_ context.Context, username string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return nil, storage.ErrNotFound
	}

	return copyUser(account.User), nil
}

// GetUserByEmail retrieves a user by email
func (r *AccountRepository) GetUserByEmail(_ context.Context, email string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	username, exists := r.accountsByEmail[email]
	if !exists {
		return nil, storage.ErrNotFound
	}

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return nil, storage.ErrNotFound
	}

	return copyUser(account.User), nil
}

// UpdateUser updates user data
func (r *AccountRepository) UpdateUser(_ context.Context, username string, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	// Apply updates
	for key, value := range updates {
		switch key {
		case "display_name":
			if v, ok := value.(string); ok {
				account.User.DisplayName = v
			}
		case "approved":
			if v, ok := value.(bool); ok {
				account.User.Approved = v
			}
		case "suspended":
			if v, ok := value.(bool); ok {
				account.User.Suspended = v
			}
		case "silenced":
			if v, ok := value.(bool); ok {
				account.User.Silenced = v
			}
		case "role":
			if v, ok := value.(string); ok {
				account.User.Role = v
			}
		}
	}

	account.User.UpdatedAt = time.Now()
	return nil
}

// Actor operations

// GetActor retrieves an actor by username
func (r *AccountRepository) GetActor(_ context.Context, username string) (*activitypub.Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists || account.Actor == nil {
		return nil, storage.ErrNotFound
	}

	return copyActor(account.Actor), nil
}

// GetActorByUsername is an alias for GetActor
func (r *AccountRepository) GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error) {
	return r.GetActor(ctx, username)
}

// GetActorPrivateKey retrieves an actor's private key
func (r *AccountRepository) GetActorPrivateKey(_ context.Context, username string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists {
		return "", storage.ErrNotFound
	}

	return account.PrivateKey, nil
}

// Account discovery and search

// SearchAccounts searches for accounts
func (r *AccountRepository) SearchAccounts(_ context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Account
	for _, account := range r.accounts {
		if account.User != nil && (containsIgnoreCase(account.User.Username, query) || containsIgnoreCase(account.User.DisplayName, query)) {
			results = append(results, copyAccount(account))
		}
	}

	// Apply pagination
	start := 0
	if opts.Cursor != "" {
		for i, a := range results {
			if a.User != nil && a.User.Username == opts.Cursor {
				start = i + 1
				break
			}
		}
	}

	end := start + opts.Limit
	if end > len(results) {
		end = len(results)
	}

	paginatedResults := results[start:end]
	nextCursor := ""
	if end < len(results) && len(paginatedResults) > 0 {
		nextCursor = paginatedResults[len(paginatedResults)-1].User.Username
	}

	return &interfaces.PaginatedResult[*storage.Account]{
		Items:      paginatedResults,
		NextCursor: nextCursor,
		HasMore:    end < len(results),
		Total:      int64(len(results)),
	}, nil
}

// GetSuggestedAccounts returns suggested accounts
func (r *AccountRepository) GetSuggestedAccounts(_ context.Context, _ string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.AccountSuggestion
	for _, account := range r.accounts {
		if account.Actor != nil {
			results = append(results, &storage.AccountSuggestion{
				Actor:  copyActor(account.Actor),
				Reason: "in_memory",
				Score:  1.0,
			})
		}
	}

	// Apply pagination
	end := opts.Limit
	if end > len(results) {
		end = len(results)
	}

	return &interfaces.PaginatedResult[*storage.AccountSuggestion]{
		Items:   results[:end],
		HasMore: end < len(results),
		Total:   int64(len(results)),
	}, nil
}

// GetFeaturedAccounts returns featured accounts
func (r *AccountRepository) GetFeaturedAccounts(_ context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Account
	for _, account := range r.accounts {
		results = append(results, copyAccount(account))
	}

	// Apply pagination
	end := opts.Limit
	if end > len(results) {
		end = len(results)
	}

	return &interfaces.PaginatedResult[*storage.Account]{
		Items:   results[:end],
		HasMore: end < len(results),
		Total:   int64(len(results)),
	}, nil
}

// Account verification and moderation

// ApproveAccount approves an account
func (r *AccountRepository) ApproveAccount(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.Approved = true
	account.User.UpdatedAt = time.Now()
	return nil
}

// SuspendAccount suspends an account
func (r *AccountRepository) SuspendAccount(_ context.Context, username string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.Suspended = true
	account.User.UpdatedAt = time.Now()
	return nil
}

// UnsuspendAccount unsuspends an account
func (r *AccountRepository) UnsuspendAccount(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.Suspended = false
	account.User.UpdatedAt = time.Now()
	return nil
}

// SilenceAccount silences an account
func (r *AccountRepository) SilenceAccount(_ context.Context, username string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.Silenced = true
	account.User.UpdatedAt = time.Now()
	return nil
}

// UnsilenceAccount unsilences an account
func (r *AccountRepository) UnsilenceAccount(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.Silenced = false
	account.User.UpdatedAt = time.Now()
	return nil
}

// Account metadata and preferences

// UpdateAccountPreferences updates account preferences
func (r *AccountRepository) UpdateAccountPreferences(_ context.Context, username string, preferences map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return storage.ErrNotFound
	}

	if r.accountPreferences[username] == nil {
		r.accountPreferences[username] = make(map[string]interface{})
	}

	for k, v := range preferences {
		r.accountPreferences[username][k] = v
	}

	return nil
}

// GetAccountPreferences gets account preferences
func (r *AccountRepository) GetAccountPreferences(_ context.Context, username string) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.accounts[username]; !exists {
		return nil, storage.ErrNotFound
	}

	prefs := r.accountPreferences[username]
	if prefs == nil {
		return make(map[string]interface{}), nil
	}

	// Return a copy
	result := make(map[string]interface{}, len(prefs))
	for k, v := range prefs {
		result[k] = v
	}
	return result, nil
}

// UpdateAccountFeatures updates account features
func (r *AccountRepository) UpdateAccountFeatures(_ context.Context, username string, features map[string]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return storage.ErrNotFound
	}

	if r.accountFeatures[username] == nil {
		r.accountFeatures[username] = make(map[string]bool)
	}

	for k, v := range features {
		r.accountFeatures[username][k] = v
	}

	return nil
}

// GetAccountFeatures gets account features
func (r *AccountRepository) GetAccountFeatures(_ context.Context, username string) (map[string]bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.accounts[username]; !exists {
		return nil, storage.ErrNotFound
	}

	features := r.accountFeatures[username]
	if features == nil {
		return make(map[string]bool), nil
	}

	// Return a copy
	result := make(map[string]bool, len(features))
	for k, v := range features {
		result[k] = v
	}
	return result, nil
}

// GetPreference gets a single preference
func (r *AccountRepository) GetPreference(_ context.Context, username, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	prefs := r.accountPreferences[username]
	if prefs == nil {
		return "", nil
	}

	if v, ok := prefs[key]; ok {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}

	return "", nil
}

// Authentication and session management

// ValidateCredentials validates user credentials
func (r *AccountRepository) ValidateCredentials(_ context.Context, username, _ string) (*storage.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// In-memory implementation doesn't actually validate password
	return copyAccount(account), nil
}

// ValidatePassword validates a password
func (r *AccountRepository) ValidatePassword(_ context.Context, username, _ string) (*storage.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return nil, storage.ErrNotFound
	}

	// In-memory implementation doesn't actually validate password
	return copyUser(account.User), nil
}

// UpdatePassword updates a user's password
func (r *AccountRepository) UpdatePassword(_ context.Context, username, newPasswordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.PasswordHash = newPasswordHash
	account.User.UpdatedAt = time.Now()
	return nil
}

// CreatePasswordReset creates a password reset token
func (r *AccountRepository) CreatePasswordReset(_ context.Context, reset *storage.PasswordReset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	resetCopy := *reset
	r.passwordResets[reset.Token] = &resetCopy
	return nil
}

// GetPasswordReset gets a password reset by token
func (r *AccountRepository) GetPasswordReset(_ context.Context, token string) (*storage.PasswordReset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reset, exists := r.passwordResets[token]
	if !exists {
		return nil, storage.ErrNotFound
	}

	resetCopy := *reset
	return &resetCopy, nil
}

// UsePasswordReset marks a password reset as used
func (r *AccountRepository) UsePasswordReset(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	reset, exists := r.passwordResets[token]
	if !exists {
		return storage.ErrNotFound
	}

	reset.Used = true
	return nil
}

// CreatePasswordResetToken creates a password reset token
func (r *AccountRepository) CreatePasswordResetToken(_ context.Context, username, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return "", storage.ErrNotFound
	}

	token := fmt.Sprintf("reset-%s-%d", username, time.Now().UnixNano())
	r.passwordResets[token] = &storage.PasswordReset{
		Token:     token,
		Username:  username,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	return token, nil
}

// ValidatePasswordResetToken validates a password reset token
func (r *AccountRepository) ValidatePasswordResetToken(_ context.Context, token string) (*storage.PasswordReset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reset, exists := r.passwordResets[token]
	if !exists {
		return nil, storage.ErrNotFound
	}

	if reset.Used || time.Now().After(reset.ExpiresAt) {
		return nil, fmt.Errorf("token expired or already used")
	}

	resetCopy := *reset
	return &resetCopy, nil
}

// ResetPassword resets a password using a token
func (r *AccountRepository) ResetPassword(_ context.Context, token, newPasswordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	reset, exists := r.passwordResets[token]
	if !exists {
		return storage.ErrNotFound
	}

	if reset.Used || time.Now().After(reset.ExpiresAt) {
		return fmt.Errorf("token expired or already used")
	}

	account, exists := r.accounts[reset.Username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.PasswordHash = newPasswordHash
	account.User.UpdatedAt = time.Now()
	reset.Used = true

	return nil
}

// Session management

// CreateSession creates a new session
func (r *AccountRepository) CreateSession(_ context.Context, username, ipAddress, userAgent string) (*storage.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return nil, storage.ErrNotFound
	}

	sessionID := fmt.Sprintf("session-%s-%d", username, time.Now().UnixNano())
	refreshToken := fmt.Sprintf("refresh-%s-%d", username, time.Now().UnixNano())

	session := &storage.Session{
		ID:           sessionID,
		Username:     username,
		RefreshToken: refreshToken,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	r.sessions[sessionID] = session
	r.sessionsByUser[username] = append(r.sessionsByUser[username], sessionID)
	r.sessionsByRefresh[refreshToken] = sessionID

	sessionCopy := *session
	return &sessionCopy, nil
}

// CreateSessionFromStruct creates a session from a struct
func (r *AccountRepository) CreateSessionFromStruct(_ context.Context, session *storage.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionCopy := *session
	r.sessions[session.ID] = &sessionCopy
	r.sessionsByUser[session.Username] = append(r.sessionsByUser[session.Username], session.ID)
	if session.RefreshToken != "" {
		r.sessionsByRefresh[session.RefreshToken] = session.ID
	}

	return nil
}

// GetSession gets a session by ID
func (r *AccountRepository) GetSession(_ context.Context, sessionID string) (*storage.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	sessionCopy := *session
	return &sessionCopy, nil
}

// GetSessionByRefreshToken gets a session by refresh token
func (r *AccountRepository) GetSessionByRefreshToken(_ context.Context, refreshToken string) (*storage.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessionID, exists := r.sessionsByRefresh[refreshToken]
	if !exists {
		return nil, storage.ErrNotFound
	}

	session, exists := r.sessions[sessionID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	sessionCopy := *session
	return &sessionCopy, nil
}

// GetUserSessions gets all sessions for a user
func (r *AccountRepository) GetUserSessions(_ context.Context, username string) ([]*storage.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessionIDs := r.sessionsByUser[username]
	var sessions []*storage.Session
	for _, id := range sessionIDs {
		if session, exists := r.sessions[id]; exists {
			sessionCopy := *session
			sessions = append(sessions, &sessionCopy)
		}
	}

	return sessions, nil
}

// UpdateSession updates a session
func (r *AccountRepository) UpdateSession(_ context.Context, sessionID, refreshToken, ipAddress string, lastActivity, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return storage.ErrNotFound
	}

	// Update refresh token index if changed
	if session.RefreshToken != refreshToken {
		delete(r.sessionsByRefresh, session.RefreshToken)
		r.sessionsByRefresh[refreshToken] = sessionID
	}

	session.RefreshToken = refreshToken
	session.IPAddress = ipAddress
	session.LastActivity = lastActivity
	session.ExpiresAt = expiresAt

	return nil
}

// DeleteSession deletes a session
func (r *AccountRepository) DeleteSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return nil // Not an error to delete non-existent session
	}

	// Clean up indexes
	delete(r.sessionsByRefresh, session.RefreshToken)

	// Remove from user's sessions
	userSessions := r.sessionsByUser[session.Username]
	for i, id := range userSessions {
		if id == sessionID {
			r.sessionsByUser[session.Username] = append(userSessions[:i], userSessions[i+1:]...)
			break
		}
	}

	delete(r.sessions, sessionID)
	return nil
}

// InvalidateSession invalidates a session
func (r *AccountRepository) InvalidateSession(_ context.Context, username, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[sessionID]
	if !exists {
		return storage.ErrNotFound
	}

	if session.Username != username {
		return storage.ErrNotFound
	}

	// Clean up indexes
	delete(r.sessionsByRefresh, session.RefreshToken)

	// Remove from user's sessions
	userSessions := r.sessionsByUser[username]
	for i, id := range userSessions {
		if id == sessionID {
			r.sessionsByUser[username] = append(userSessions[:i], userSessions[i+1:]...)
			break
		}
	}

	delete(r.sessions, sessionID)
	return nil
}

// InvalidateAllSessions invalidates all sessions for a user
func (r *AccountRepository) InvalidateAllSessions(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionIDs := r.sessionsByUser[username]
	for _, id := range sessionIDs {
		if session, exists := r.sessions[id]; exists {
			delete(r.sessionsByRefresh, session.RefreshToken)
			delete(r.sessions, id)
		}
	}

	delete(r.sessionsByUser, username)
	return nil
}

// Login tracking

// RecordLogin records a login attempt
func (r *AccountRepository) RecordLogin(_ context.Context, attempt *storage.LoginAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	attemptCopy := *attempt
	r.loginAttempts[attempt.Username] = append(r.loginAttempts[attempt.Username], &attemptCopy)
	return nil
}

// GetLoginHistory gets login history for a user
func (r *AccountRepository) GetLoginHistory(_ context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	attempts := r.loginAttempts[username]

	// Apply pagination
	end := opts.Limit
	if end > len(attempts) {
		end = len(attempts)
	}

	var results []*storage.LoginAttempt
	for i := 0; i < end; i++ {
		attemptCopy := *attempts[i]
		results = append(results, &attemptCopy)
	}

	return &interfaces.PaginatedResult[*storage.LoginAttempt]{
		Items:   results,
		HasMore: end < len(attempts),
		Total:   int64(len(attempts)),
	}, nil
}

// GetRecentLoginAttempts gets recent login attempts
func (r *AccountRepository) GetRecentLoginAttempts(_ context.Context, username string, since time.Time) ([]*storage.LoginAttempt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.LoginAttempt
	for _, attempt := range r.loginAttempts[username] {
		if attempt.Timestamp.After(since) {
			attemptCopy := *attempt
			results = append(results, &attemptCopy)
		}
	}

	return results, nil
}

// UpdateLastLogin updates the last login time
func (r *AccountRepository) UpdateLastLogin(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.UpdatedAt = time.Now()
	return nil
}

// UpdateLastActivity updates the last activity time
func (r *AccountRepository) UpdateLastActivity(_ context.Context, username string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[username]
	if !exists || account.User == nil {
		return storage.ErrNotFound
	}

	account.User.UpdatedAt = time.Now()
	return nil
}

// IsRateLimited checks if a key is rate limited
func (r *AccountRepository) IsRateLimited(_ context.Context, key string) (bool, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.rateLimits[key]
	if !exists {
		return false, time.Time{}, nil
	}

	if entry.blocked && time.Now().Before(entry.blockedAt.Add(15*time.Minute)) {
		return true, entry.blockedAt.Add(15 * time.Minute), nil
	}

	return false, time.Time{}, nil
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *AccountRepository) RecordLoginAttempt(_ context.Context, key string, success bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.rateLimits[key]
	if !exists {
		entry = &rateLimitEntry{
			resetAt: time.Now().Add(15 * time.Minute),
		}
		r.rateLimits[key] = entry
	}

	if success {
		entry.count = 0
		entry.blocked = false
	} else {
		entry.count++
		if entry.count >= 5 {
			entry.blocked = true
			entry.blockedAt = time.Now()
		}
	}

	return nil
}

// ClearLoginAttempts clears login attempts for a key
func (r *AccountRepository) ClearLoginAttempts(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.rateLimits, key)
	return nil
}

// GetLoginAttemptCount gets the count of login attempts
func (r *AccountRepository) GetLoginAttemptCount(_ context.Context, key string, _ time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.rateLimits[key]
	if !exists {
		return 0, nil
	}

	return entry.count, nil
}

// Device management

// CreateDevice creates a device
func (r *AccountRepository) CreateDevice(_ context.Context, device *storage.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	deviceCopy := *device
	r.devices[device.ID] = &deviceCopy
	r.devicesByUser[device.Username] = append(r.devicesByUser[device.Username], device.ID)
	return nil
}

// GetDevice gets a device by ID
func (r *AccountRepository) GetDevice(_ context.Context, deviceID string) (*storage.Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	deviceCopy := *device
	return &deviceCopy, nil
}

// UpdateDevice updates a device
func (r *AccountRepository) UpdateDevice(_ context.Context, device *storage.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[device.ID]; !exists {
		return storage.ErrNotFound
	}

	deviceCopy := *device
	r.devices[device.ID] = &deviceCopy
	return nil
}

// GetUserDevices gets all devices for a user
func (r *AccountRepository) GetUserDevices(_ context.Context, username string) ([]*storage.Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	deviceIDs := r.devicesByUser[username]
	var devices []*storage.Device
	for _, id := range deviceIDs {
		if device, exists := r.devices[id]; exists {
			deviceCopy := *device
			devices = append(devices, &deviceCopy)
		}
	}

	return devices, nil
}

// Recovery operations

// GetUserByRecoveryCode gets a user by recovery code
func (r *AccountRepository) GetUserByRecoveryCode(_ context.Context, _ string) (*storage.User, error) {
	// In-memory implementation doesn't support recovery codes
	return nil, storage.ErrNotFound
}

// StoreRecoveryToken stores a recovery token
func (r *AccountRepository) StoreRecoveryToken(_ context.Context, key string, data map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Make a copy
	dataCopy := make(map[string]interface{}, len(data))
	for k, v := range data {
		dataCopy[k] = v
	}
	r.recoveryTokens[key] = dataCopy
	return nil
}

// GetRecoveryToken gets a recovery token
func (r *AccountRepository) GetRecoveryToken(_ context.Context, key string) (map[string]interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.recoveryTokens[key]
	if !exists {
		return nil, storage.ErrNotFound
	}

	// Return a copy
	dataCopy := make(map[string]interface{}, len(data))
	for k, v := range data {
		dataCopy[k] = v
	}
	return dataCopy, nil
}

// DeleteRecoveryToken deletes a recovery token
func (r *AccountRepository) DeleteRecoveryToken(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.recoveryTokens, key)
	return nil
}

// WebAuthn operations

// StoreWebAuthnChallenge stores a WebAuthn challenge
func (r *AccountRepository) StoreWebAuthnChallenge(_ context.Context, challenge *storage.WebAuthnChallenge) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	challengeCopy := *challenge
	r.webAuthnChallenges[challenge.ID] = &challengeCopy
	return nil
}

// StoreWebAuthnCredential stores a WebAuthn credential
func (r *AccountRepository) StoreWebAuthnCredential(_ context.Context, credential *storage.WebAuthnCredential) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	credentialCopy := *credential
	r.webAuthnCredentials[credential.ID] = &credentialCopy
	return nil
}

// UpdateWebAuthnCredential updates a WebAuthn credential
func (r *AccountRepository) UpdateWebAuthnCredential(_ context.Context, credentialID string, signCount uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	credential, exists := r.webAuthnCredentials[credentialID]
	if !exists {
		return storage.ErrNotFound
	}

	credential.SignCount = signCount
	return nil
}

// OAuth and provider operations

// GetLinkedProviders gets linked OAuth providers
func (r *AccountRepository) GetLinkedProviders(_ context.Context, _ string) ([]string, error) {
	// In-memory implementation returns empty list
	return []string{}, nil
}

// UpdateWalletLastUsed updates the last used time for a wallet
func (r *AccountRepository) UpdateWalletLastUsed(_ context.Context, username, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return storage.ErrNotFound
	}

	return nil
}

// Account pins

// CreateAccountPin creates an account pin
func (r *AccountRepository) CreateAccountPin(_ context.Context, pin *storage.AccountPin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already pinned
	pins := r.accountPins[pin.Username]
	for _, p := range pins {
		if p.PinnedActorID == pin.PinnedActorID {
			return storage.ErrAlreadyExists
		}
	}

	pinCopy := *pin
	r.accountPins[pin.Username] = append(r.accountPins[pin.Username], &pinCopy)
	return nil
}

// DeleteAccountPin deletes an account pin
func (r *AccountRepository) DeleteAccountPin(_ context.Context, username, targetActorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pins := r.accountPins[username]
	for i, p := range pins {
		if p.PinnedActorID == targetActorID {
			r.accountPins[username] = append(pins[:i], pins[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

// IsAccountPinned checks if an account is pinned
func (r *AccountRepository) IsAccountPinned(_ context.Context, username, targetActorID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pins := r.accountPins[username]
	for _, p := range pins {
		if p.PinnedActorID == targetActorID {
			return true, nil
		}
	}
	return false, nil
}

// Account notes

// CreateAccountNote creates an account note
func (r *AccountRepository) CreateAccountNote(_ context.Context, note *storage.AccountNote) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.accountNotes[note.Username] == nil {
		r.accountNotes[note.Username] = make(map[string]*storage.AccountNote)
	}

	noteCopy := *note
	r.accountNotes[note.Username][note.TargetActorID] = &noteCopy
	return nil
}

// Relationship helpers

// GetFollowRequestState gets the state of a follow request
func (r *AccountRepository) GetFollowRequestState(_ context.Context, requesterID, targetID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", requesterID, targetID)
	state, exists := r.followRequestStates[key]
	if !exists {
		return "", nil
	}
	return state, nil
}

// IsBlockedDomain checks if a domain is blocked
func (r *AccountRepository) IsBlockedDomain(_ context.Context, userID, domain string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	domains := r.blockedDomains[userID]
	if domains == nil {
		return false, nil
	}
	return domains[domain], nil
}

// Bookmark operations

// AddBookmark adds a bookmark for a user
func (r *AccountRepository) AddBookmark(_ context.Context, username, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[username]; !exists {
		return storage.ErrNotFound
	}

	// Check if already bookmarked
	for _, b := range r.bookmarks[username] {
		if b.ObjectID == objectID {
			return storage.ErrAlreadyExists
		}
	}

	bookmark := &storage.Bookmark{
		Username:  username,
		ObjectID:  objectID,
		CreatedAt: time.Now(),
	}
	r.bookmarks[username] = append(r.bookmarks[username], bookmark)
	return nil
}

// RemoveBookmark removes a bookmark for a user
func (r *AccountRepository) RemoveBookmark(_ context.Context, username, objectID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	bookmarks := r.bookmarks[username]
	for i, b := range bookmarks {
		if b.ObjectID == objectID {
			r.bookmarks[username] = append(bookmarks[:i], bookmarks[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

// GetBookmarks gets bookmarks for a user with pagination
func (r *AccountRepository) GetBookmarks(_ context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.accounts[username]; !exists {
		return nil, "", storage.ErrNotFound
	}

	bookmarks := r.bookmarks[username]
	if len(bookmarks) == 0 {
		return []*storage.Bookmark{}, "", nil
	}

	// Find start position based on cursor
	start := 0
	if cursor != "" {
		for i, b := range bookmarks {
			if b.ObjectID == cursor {
				start = i + 1
				break
			}
		}
	}

	// Apply limit
	end := start + limit
	if end > len(bookmarks) {
		end = len(bookmarks)
	}

	// Copy results
	var results []*storage.Bookmark
	for i := start; i < end; i++ {
		bookmarkCopy := *bookmarks[i]
		results = append(results, &bookmarkCopy)
	}

	// Determine next cursor
	nextCursor := ""
	if end < len(bookmarks) && len(results) > 0 {
		nextCursor = results[len(results)-1].ObjectID
	}

	return results, nextCursor, nil
}

// GetBookmarkedStatuses gets bookmarked statuses for a user
func (r *AccountRepository) GetBookmarkedStatuses(_ context.Context, username string, _ interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.accounts[username]; !exists {
		return nil, storage.ErrNotFound
	}

	// In-memory implementation returns empty result since we don't have status storage here
	// The actual status lookup would need to be done by the caller using the bookmark object IDs
	return &interfaces.PaginatedResult[*models.Status]{
		Items:   []*models.Status{},
		HasMore: false,
		Total:   0,
	}, nil
}

// GetFieldVerification gets field verification
func (r *AccountRepository) GetFieldVerification(_ context.Context, _, _ string) (*storage.ActorField, error) {
	// In-memory implementation doesn't support field verification
	return nil, storage.ErrNotFound
}

// Batch operations

// GetAccountsByUsernames gets accounts by usernames
func (r *AccountRepository) GetAccountsByUsernames(_ context.Context, usernames []string) ([]*storage.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var accounts []*storage.Account
	for _, username := range usernames {
		if account, exists := r.accounts[username]; exists {
			accounts = append(accounts, copyAccount(account))
		}
	}
	return accounts, nil
}

// GetAccountsCount gets the total count of accounts
func (r *AccountRepository) GetAccountsCount(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.accounts)), nil
}

// Helper functions

func copyAccount(account *storage.Account) *storage.Account {
	if account == nil {
		return nil
	}
	accountCopy := &storage.Account{
		PrivateKey: account.PrivateKey,
	}
	if account.User != nil {
		accountCopy.User = copyUser(account.User)
	}
	if account.Actor != nil {
		accountCopy.Actor = copyActor(account.Actor)
	}
	return accountCopy
}

func copyUser(user *storage.User) *storage.User {
	if user == nil {
		return nil
	}
	userCopy := *user
	return &userCopy
}

func copyActor(actor *activitypub.Actor) *activitypub.Actor {
	if actor == nil {
		return nil
	}
	actorCopy := *actor
	return &actorCopy
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchesIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func matchesIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Ensure AccountRepository implements interfaces.AccountRepository
var _ interfaces.AccountRepository = (*AccountRepository)(nil)
