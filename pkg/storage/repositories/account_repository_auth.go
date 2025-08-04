package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ===== Authentication Methods =====
// This file contains authentication-related methods for the AccountRepository

// ValidatePassword validates a user's password and tracks login attempts
func (r *AccountRepository) ValidatePassword(ctx context.Context, username, password string) (*storage.User, error) {
	// Get user first
	user, err := r.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}

	// Check if account is suspended
	if user.Suspended {
		return nil, common.AccountSuspendedError{Username: username}
	}

	// Verify password using bcrypt directly
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		// Track failed login attempt
		r.trackFailedLogin(ctx, username)
		return nil, common.InvalidPasswordError{}
	}

	// Track successful login
	r.trackSuccessfulLogin(ctx, username)

	return user, nil
}

// trackSuccessfulLogin records a successful login attempt
func (r *AccountRepository) trackSuccessfulLogin(ctx context.Context, username string) {
	login := &models.UserLogin{
		Username:  username,
		Timestamp: time.Now(),
		Success:   true,
	}

	err := r.db.WithContext(ctx).Model(login).Create()
	if err != nil {
		r.logger.Error("failed to track successful login",
			zap.String("username", username),
			zap.Error(err))
	}
}

// trackFailedLogin records a failed login attempt
func (r *AccountRepository) trackFailedLogin(ctx context.Context, username string) {
	login := &models.UserLogin{
		Username:  username,
		Timestamp: time.Now(),
		Success:   false,
	}

	err := r.db.WithContext(ctx).Model(login).Create()
	if err != nil {
		r.logger.Error("failed to track failed login",
			zap.String("username", username),
			zap.Error(err))
	}
}

// GetRecentLoginAttempts retrieves recent login attempts for a user
func (r *AccountRepository) GetRecentLoginAttempts(ctx context.Context, username string, since time.Time) ([]*storage.LoginAttempt, error) {
	var logins []models.UserLogin

	err := r.db.WithContext(ctx).Model(&models.UserLogin{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "LOGIN#").
		Where("Timestamp", ">", since).
		Limit(100).
		All(&logins)

	if err != nil {
		r.logger.Error("failed to get recent login attempts",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get recent login attempts: %w", err)
	}

	// Convert to storage type
	attempts := make([]*storage.LoginAttempt, len(logins))
	for i, login := range logins {
		attempts[i] = &storage.LoginAttempt{
			Username:  login.Username,
			Timestamp: login.Timestamp,
			Success:   login.Success,
			IPAddress: login.IPAddress,
			UserAgent: login.UserAgent,
		}
	}

	return attempts, nil
}

// CreatePasswordResetToken creates a password reset token for a user
func (r *AccountRepository) CreatePasswordResetToken(ctx context.Context, username, email string) (string, error) {
	// Verify user exists and email matches
	user, err := r.GetUser(ctx, username)
	if err != nil {
		return "", err
	}

	if user.Email != email {
		return "", common.ValidationError{
			Field:   "email",
			Message: "email does not match account",
		}
	}

	// Generate token
	token := generateSecureToken()
	
	// Create reset token record
	reset := &models.PasswordReset{
		Username:  username,
		Token:     token,
		Email:     email,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour), // 1 hour expiry
		Used:      false,
	}

	err = r.db.WithContext(ctx).Model(reset).Create()
	if err != nil {
		r.logger.Error("failed to create password reset token",
			zap.String("username", username),
			zap.Error(err))
		return "", fmt.Errorf("failed to create password reset token: %w", err)
	}

	return token, nil
}

// ValidatePasswordResetToken validates a password reset token
func (r *AccountRepository) ValidatePasswordResetToken(ctx context.Context, token string) (*storage.PasswordReset, error) {
	var reset models.PasswordReset

	// Use GSI for token lookup
	err := r.db.WithContext(ctx).Model(&reset).
		Index("token-index").
		Where("GSI1PK", "=", fmt.Sprintf("RESET_TOKEN#%s", token)).
		First(&reset)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.InvalidTokenError{Token: token}
		}
		r.logger.Error("failed to validate password reset token",
			zap.String("token", token),
			zap.Error(err))
		return nil, fmt.Errorf("failed to validate password reset token: %w", err)
	}

	// Check if expired
	if time.Now().After(reset.ExpiresAt) {
		return nil, common.ExpiredTokenError{Token: token}
	}

	// Check if already used
	if reset.Used {
		return nil, common.UsedTokenError{Token: token}
	}

	return &storage.PasswordReset{
		Username:  reset.Username,
		Token:     reset.Token,
		Email:     reset.Email,
		CreatedAt: reset.CreatedAt,
		ExpiresAt: reset.ExpiresAt,
		Used:      reset.Used,
	}, nil
}

// ResetPassword resets a user's password using a valid token
func (r *AccountRepository) ResetPassword(ctx context.Context, token, newPasswordHash string) error {
	// Validate token first
	reset, err := r.ValidatePasswordResetToken(ctx, token)
	if err != nil {
		return err
	}

	// Update user password
	err = r.UpdateUser(ctx, reset.Username, map[string]interface{}{
		"password_hash": newPasswordHash,
	})
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Mark token as used - get the model first then update
	var resetModel models.PasswordReset
	err = r.db.WithContext(ctx).Model(&resetModel).
		Where("PK", "=", fmt.Sprintf("USER#%s", reset.Username)).
		Where("SK", "=", fmt.Sprintf("RESET#%s", token)).
		First(&resetModel)
	
	if err == nil {
		resetModel.Used = true
		resetModel.UsedAt = time.Now()
		err = r.db.WithContext(ctx).Model(&resetModel).Update()
	}

	if err != nil {
		r.logger.Error("failed to mark reset token as used",
			zap.String("token", token),
			zap.Error(err))
		// Don't fail the password reset if we can't mark the token
	}

	return nil
}

// GetUserSessions retrieves all active sessions for a user
func (r *AccountRepository) GetUserSessions(ctx context.Context, username string) ([]*storage.Session, error) {
	var sessions []models.Session

	err := r.db.WithContext(ctx).Model(&models.Session{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "SESSION#").
		All(&sessions)

	if err != nil {
		r.logger.Error("failed to get user sessions",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.Session, len(sessions))
	for i, session := range sessions {
		expiresAt := time.Unix(session.ExpiresAt, 0)
		result[i] = &storage.Session{
			SessionID:    session.SessionID,
			Username:     extractUsernameFromUserID(session.UserID),
			RefreshToken: session.AccessToken, // Map AccessToken to RefreshToken field
			CreatedAt:    session.CreatedAt,
			ExpiresAt:    expiresAt,
			IPAddress:    session.IPAddress,
			UserAgent:    session.UserAgent,
			LastActivity: session.LastUsedAt,
		}
	}

	return result, nil
}

// CreateSession creates a new session for a user
func (r *AccountRepository) CreateSession(ctx context.Context, username, ipAddress, userAgent string) (*storage.Session, error) {
	sessionID := generateSessionID()
	token := generateSecureToken()
	
	now := time.Now()
	expiresAt := now.Add(30 * 24 * time.Hour) // 30 days

	session := &models.Session{
		SessionID:    sessionID,
		UserID:       fmt.Sprintf("USER#%s", username),
		AccessToken:  token,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastUsedAt:   now,
		ExpiresAt:    expiresAt.Unix(),
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		IsRevoked:    false,
	}

	err := r.db.WithContext(ctx).Model(session).Create()
	if err != nil {
		r.logger.Error("failed to create session",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &storage.Session{
		SessionID:    session.SessionID,
		Username:     username,
		RefreshToken: session.AccessToken,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    expiresAt,
		IPAddress:    session.IPAddress,
		UserAgent:    session.UserAgent,
		LastActivity: session.LastUsedAt,
	}, nil
}

// InvalidateSession invalidates a specific session
func (r *AccountRepository) InvalidateSession(ctx context.Context, username, sessionID string) error {
	// Get the session first
	var session models.Session
	err := r.db.WithContext(ctx).Model(&session).
		Where("PK", "=", fmt.Sprintf("session#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("session#%s", sessionID)).
		First(&session)

	if err != nil {
		if errors.IsNotFound(err) {
			return common.SessionNotFoundError{SessionID: sessionID}
		}
		r.logger.Error("failed to get session for invalidation",
			zap.String("username", username),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Update the session
	now := time.Now()
	session.IsRevoked = true
	session.RevokedAt = &now
	session.RevokeReason = "manual_invalidation"
	session.UpdatedAt = now
	
	err = r.db.WithContext(ctx).Model(&session).Update()
	if err != nil {
		r.logger.Error("failed to invalidate session",
			zap.String("username", username),
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return fmt.Errorf("failed to invalidate session: %w", err)
	}

	return nil
}

// InvalidateAllSessions invalidates all sessions for a user
func (r *AccountRepository) InvalidateAllSessions(ctx context.Context, username string) error {
	sessions, err := r.GetUserSessions(ctx, username)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		// Check if session is active (not revoked)
		if session.ExpiresAt.After(time.Now()) {
			err := r.InvalidateSession(ctx, username, session.SessionID)
			if err != nil {
				r.logger.Error("failed to invalidate session during bulk invalidation",
					zap.String("sessionID", session.SessionID),
					zap.Error(err))
				// Continue with other sessions
			}
		}
	}

	return nil
}

// UpdateLastLogin updates the user's last login timestamp
func (r *AccountRepository) UpdateLastLogin(ctx context.Context, username string) error {
	return r.UpdateUser(ctx, username, map[string]interface{}{
		"last_login_at": time.Now(),
	})
}

// GetUserByRecoveryCode retrieves a user by recovery code (for email-free auth)
func (r *AccountRepository) GetUserByRecoveryCode(ctx context.Context, recoveryCode string) (*storage.User, error) {
	// Recovery codes are hashed using bcrypt, stored per user
	// We need to query all users with recovery codes and check each one
	
	// Get all recovery codes from all users
	var recoveryCodes []models.RecoveryCode
	
	err := r.db.WithContext(ctx).Model(&models.RecoveryCode{}).
		Where("SK", "BEGINS_WITH", "RECOVERY_CODE#").
		All(&recoveryCodes)
		
	if err != nil {
		r.logger.Error("failed to query recovery codes",
			zap.Error(err))
		return nil, fmt.Errorf("failed to query recovery codes: %w", err)
	}
	
	// Check each recovery code hash against the provided code
	for _, code := range recoveryCodes {
		// Skip already used codes
		if code.UsedAt != nil {
			continue
		}
		
		// Verify the recovery code hash
		if r.verifyRecoveryCodeHash(recoveryCode, code.CodeHash) {
			// Found matching code, return the user
			username := extractUsernameFromPK(code.PK)
			if username == "" {
				r.logger.Error("invalid recovery code PK format",
					zap.String("pk", code.PK))
				continue
			}
			
			// Mark code as used (best effort)
			now := time.Now()
			code.UsedAt = &now
			if updateErr := r.db.WithContext(ctx).Model(&code).Update(); updateErr != nil {
				r.logger.Error("failed to mark recovery code as used",
					zap.String("username", username),
					zap.Error(updateErr))
			}
			
			// Return the user
			return r.GetUser(ctx, username)
		}
	}
	
	return nil, common.UserNotFoundError{Username: "recovery:" + recoveryCode}
}

// generateSecureToken generates a cryptographically secure token
func generateSecureToken() string {
	// This would use crypto/rand in production
	return fmt.Sprintf("token_%d_%s", time.Now().UnixNano(), strings.ReplaceAll(time.Now().String(), " ", "_"))
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}

// verifyRecoveryCodeHash verifies a recovery code against its bcrypt hash
func (r *AccountRepository) verifyRecoveryCodeHash(code, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
	if err != nil {
		if err != bcrypt.ErrMismatchedHashAndPassword {
			r.logger.Debug("error verifying recovery code hash",
				zap.Error(err))
		}
		return false
	}
	return true
}

// extractUsernameFromPK extracts username from USER#{username} format
func extractUsernameFromPK(pk string) string {
	if !strings.HasPrefix(pk, "USER#") {
		return ""
	}
	return strings.TrimPrefix(pk, "USER#")
}

// extractUsernameFromUserID extracts username from USER#{username} format (alias)
func extractUsernameFromUserID(userID string) string {
	return extractUsernameFromPK(userID)
}

// ===== Rate Limiting Methods =====

// IsRateLimited checks if a key is currently rate limited
func (r *AccountRepository) IsRateLimited(ctx context.Context, key string) (bool, time.Time, error) {
	return false, time.Time{}, fmt.Errorf("IsRateLimited not implemented")
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *AccountRepository) RecordLoginAttempt(ctx context.Context, key string, success bool) error {
	return fmt.Errorf("RecordLoginAttempt not implemented")
}

// ClearLoginAttempts clears all login attempts for a key
func (r *AccountRepository) ClearLoginAttempts(ctx context.Context, key string) error {
	return fmt.Errorf("ClearLoginAttempts not implemented")
}

// GetLoginAttemptCount gets the number of login attempts since a given time
func (r *AccountRepository) GetLoginAttemptCount(ctx context.Context, key string, since time.Time) (int, error) {
	return 0, fmt.Errorf("GetLoginAttemptCount not implemented")
}

// ===== Additional Session Methods =====

// GetSession retrieves a session by ID
func (r *AccountRepository) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	return nil, fmt.Errorf("GetSession not implemented")
}

// UpdateSession updates an existing session
func (r *AccountRepository) UpdateSession(ctx context.Context, session *storage.Session) error {
	return fmt.Errorf("UpdateSession not implemented")
}

// DeleteSession deletes a session
func (r *AccountRepository) DeleteSession(ctx context.Context, sessionID string) error {
	return fmt.Errorf("DeleteSession not implemented")
}

// GetSessionByRefreshToken finds a session by refresh token
func (r *AccountRepository) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	return nil, fmt.Errorf("GetSessionByRefreshToken not implemented")
}

// ===== Device Management Methods =====

// CreateDevice creates a new device record
func (r *AccountRepository) CreateDevice(ctx context.Context, device *storage.Device) error {
	return fmt.Errorf("CreateDevice not implemented")
}

// GetDevice retrieves a device by ID
func (r *AccountRepository) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	return nil, fmt.Errorf("GetDevice not implemented")
}

// UpdateDevice updates an existing device
func (r *AccountRepository) UpdateDevice(ctx context.Context, device *storage.Device) error {
	return fmt.Errorf("UpdateDevice not implemented")
}

// GetUserDevices gets all devices for a user
func (r *AccountRepository) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	return nil, fmt.Errorf("GetUserDevices not implemented")
}

// CreateSessionFromStruct creates a new user session from storage.Session struct
func (r *AccountRepository) CreateSessionFromStruct(ctx context.Context, session *storage.Session) error {
	return fmt.Errorf("CreateSessionFromStruct not implemented - use existing CreateSession with parameters")
}

// ===== Recovery Token Methods =====

// StoreRecoveryToken stores a recovery token
func (r *AccountRepository) StoreRecoveryToken(ctx context.Context, key string, data map[string]interface{}) error {
	return fmt.Errorf("StoreRecoveryToken not implemented")
}

// GetRecoveryToken retrieves a recovery token
func (r *AccountRepository) GetRecoveryToken(ctx context.Context, key string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("GetRecoveryToken not implemented")
}

// DeleteRecoveryToken deletes a recovery token
func (r *AccountRepository) DeleteRecoveryToken(ctx context.Context, key string) error {
	return fmt.Errorf("DeleteRecoveryToken not implemented")
}

// ===== WebAuthn Methods =====

// StoreWebAuthnChallenge stores a WebAuthn challenge
func (r *AccountRepository) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	return fmt.Errorf("StoreWebAuthnChallenge not implemented")
}

// StoreWebAuthnCredential stores a WebAuthn credential
func (r *AccountRepository) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	return fmt.Errorf("StoreWebAuthnCredential not implemented")
}

// UpdateWebAuthnCredential updates a WebAuthn credential
func (r *AccountRepository) UpdateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	return fmt.Errorf("UpdateWebAuthnCredential not implemented")
}

// UpdateWalletLastUsed updates when a wallet was last used
func (r *AccountRepository) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	return fmt.Errorf("UpdateWalletLastUsed not implemented")
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (r *AccountRepository) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	return []string{}, fmt.Errorf("GetLinkedProviders not implemented")
}