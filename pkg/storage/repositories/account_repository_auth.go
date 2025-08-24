package repositories

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
		return nil, ErrorHandler.HandleQueryError(err, "login attempt", "recent attempts")
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
		return "", ErrorHandler.HandleCreateError(err, "password reset token", username)
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
		return nil, ErrorHandler.HandleGetError(err, "password reset token", token)
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
		return ErrorHandler.HandleUpdateError(err, "password", reset.Username)
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
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, "user sessions")
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
		SessionID:   sessionID,
		UserID:      fmt.Sprintf("USER#%s", username),
		AccessToken: token,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastUsedAt:  now,
		ExpiresAt:   expiresAt.Unix(),
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		IsRevoked:   false,
	}

	err := r.db.WithContext(ctx).Model(session).Create()
	if err != nil {
		r.logger.Error("failed to create session",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleCreateError(err, EntitySession, session.SessionID)
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
		return ErrorHandler.HandleGetError(err, EntitySession, sessionID)
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
		return ErrorHandler.HandleUpdateError(err, EntitySession, sessionID)
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
		return nil, ErrorHandler.HandleQueryError(err, "recovery code", "query")
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
			if err := common.ValidateRequiredParam("username", username); err != nil {
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
	// Generate 32 bytes of random data using crypto/rand
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// Fallback to timestamp-based token if crypto/rand fails (should never happen in production)
		return fmt.Sprintf("fallback_token_%d", time.Now().UnixNano())
	}

	// Return hex-encoded secure random token
	return hex.EncodeToString(randomBytes)
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
	// Check if there's an active lockout
	var lockout models.RateLimitLockout
	err := r.db.WithContext(ctx).Model(&models.RateLimitLockout{}).
		Where("PK", "=", fmt.Sprintf("RATELIMIT#%s", key)).
		Where("SK", "=", "LOCKOUT").
		First(&lockout)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, ErrorHandler.HandleQueryError(err, "rate limit", "check")
	}

	// Check if lockout is still active
	if time.Now().Before(lockout.UnlockTime) {
		return true, lockout.UnlockTime, nil
	}

	return false, time.Time{}, nil
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *AccountRepository) RecordLoginAttempt(ctx context.Context, key string, success bool) error {
	// Create a new login attempt record
	attempt := models.NewLoginAttempt(key, success)

	err := r.db.WithContext(ctx).Model(attempt).Create()
	if err != nil {
		r.logger.Error("failed to record login attempt",
			zap.String("key", key),
			zap.Bool("success", success),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "login attempt", "attempt")
	}

	r.logger.Debug("recorded login attempt",
		zap.String("key", key),
		zap.Bool("success", success))

	return nil
}

// ClearLoginAttempts clears all login attempts for a key
func (r *AccountRepository) ClearLoginAttempts(ctx context.Context, key string) error {
	// Query all attempts for this key
	var attempts []models.LoginAttempt
	query := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
		Where("PK", "=", fmt.Sprintf("RATELIMIT#%s", key))
	err := query.Scan(&attempts)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, "login attempt", "query")
	}

	// Delete all items
	for _, attempt := range attempts {
		err := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
			Where("PK", "=", attempt.PK).
			Where("SK", "=", attempt.SK).
			Delete()
		if err != nil {
			r.logger.Error("failed to delete login attempt",
				zap.String("pk", attempt.PK),
				zap.String("sk", attempt.SK),
				zap.Error(err))
		}
	}

	// Also clear any lockout record
	err = r.db.WithContext(ctx).Model(&models.RateLimitLockout{}).
		Where("PK", "=", fmt.Sprintf("RATELIMIT#%s", key)).
		Where("SK", "=", "LOCKOUT").
		Delete()
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to delete lockout record",
			zap.String("key", key),
			zap.Error(err))
	}

	r.logger.Debug("cleared login attempts", zap.String("key", key))
	return nil
}

// GetLoginAttemptCount gets the number of login attempts since a given time
func (r *AccountRepository) GetLoginAttemptCount(ctx context.Context, key string, since time.Time) (int, error) {
	// Query attempts since the given time
	var attempts []models.LoginAttempt
	query := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
		Where("PK", "=", fmt.Sprintf("RATELIMIT#%s", key)).
		Where("SK", ">", since.Format(time.RFC3339Nano))
	err := query.Scan(&attempts)

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "login attempt", "count")
	}

	return len(attempts), nil
}

// ===== Additional Session Methods =====

// GetSession retrieves a session by ID
func (r *AccountRepository) GetSession(ctx context.Context, sessionID string) (*storage.Session, error) {
	var session models.Session

	err := r.db.WithContext(ctx).Model(&session).
		Where("PK", "=", fmt.Sprintf("session#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("session#%s", sessionID)).
		First(&session)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.SessionNotFoundError{SessionID: sessionID}
		}
		r.logger.Error("failed to get session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Convert to storage type
	expiresAt := time.Unix(session.ExpiresAt, 0)
	storageSession := &storage.Session{
		ID:           session.SessionID,
		SessionID:    session.SessionID,
		Username:     extractUsernameFromUserID(session.UserID),
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    expiresAt,
		RefreshToken: session.RefreshToken,
		LastActivity: session.LastUsedAt,
		UserAgent:    session.UserAgent,
		IPAddress:    session.IPAddress,
		DeviceID:     session.DeviceID,
		AuthMethod:   "", // Not stored in new model
		// Handle previous refresh token if needed
		PreviousRefreshToken: "", // Would need to be added to model if required
		TokenRotatedAt:       time.Time{},
	}

	return storageSession, nil
}

// UpdateSession updates an existing session with specific fields
func (r *AccountRepository) UpdateSession(ctx context.Context, sessionID, refreshToken, ipAddress string, lastActivity, expiresAt time.Time) error {
	// Get the existing session first to find which record to update
	var session models.Session
	err := r.db.WithContext(ctx).Model(&session).
		Where("PK", "=", fmt.Sprintf("session#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("session#%s", sessionID)).
		First(&session)

	if err != nil {
		if errors.IsNotFound(err) {
			return common.SessionNotFoundError{SessionID: sessionID}
		}
		r.logger.Error("failed to get session for update",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntitySession, sessionID)
	}

	// Update the session fields
	session.RefreshToken = refreshToken
	session.IPAddress = ipAddress
	session.LastUsedAt = lastActivity
	session.UpdatedAt = time.Now()
	session.ExpiresAt = expiresAt.Unix()

	// Update GSI keys since refresh token changed
	session.GSI2PK = "TOKEN#" + hashTokenForGSI(session.AccessToken) // Keep access token GSI the same
	// Note: Legacy used REFRESHTOKEN# prefix in GSI1, but our model uses TOKEN# for access tokens
	// We may need to adapt this if refresh token lookup is needed via GSI

	err = r.db.WithContext(ctx).Model(&session).Update()
	if err != nil {
		r.logger.Error("failed to update session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntitySession, sessionID)
	}

	r.logger.Debug("session updated successfully",
		zap.String("sessionID", sessionID))

	return nil
}

// DeleteSession deletes a session
func (r *AccountRepository) DeleteSession(ctx context.Context, sessionID string) error {
	err := r.db.WithContext(ctx).Model(&models.Session{}).
		Where("PK", "=", fmt.Sprintf("session#%s", sessionID)).
		Where("SK", "=", fmt.Sprintf("session#%s", sessionID)).
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			return common.SessionNotFoundError{SessionID: sessionID}
		}
		r.logger.Error("failed to delete session",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntitySession, sessionID)
	}

	r.logger.Debug("session deleted successfully",
		zap.String("sessionID", sessionID))

	return nil
}

// GetSessionByRefreshToken finds a session by refresh token
func (r *AccountRepository) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (*storage.Session, error) {
	// The current model doesn't have a GSI for refresh tokens like the legacy did
	// We need to scan for sessions with matching refresh token
	// In production, you might want to add a GSI for refresh token lookup
	var sessions []models.Session

	err := r.db.WithContext(ctx).Model(&models.Session{}).
		Where("RefreshToken", "=", refreshToken).
		Limit(1).
		All(&sessions)

	if err != nil {
		r.logger.Error("failed to query session by refresh token",
			zap.String("refreshToken", refreshToken[:minInt(len(refreshToken), 10)]+"..."), // Log only first 10 chars for security
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntitySession, "refresh token")
	}

	if err := common.ValidateSliceNotEmpty("sessions", sessions); err != nil {
		// Check if this might be a previous refresh token (for grace period)
		// This would require scanning all sessions, which is expensive
		// For now, just return not found
		return nil, common.SessionNotFoundError{SessionID: "refresh:" + refreshToken[:minInt(len(refreshToken), 10)]}
	}

	session := sessions[0]

	// Convert to storage type
	expiresAt := time.Unix(session.ExpiresAt, 0)
	storageSession := &storage.Session{
		ID:                   session.SessionID,
		SessionID:            session.SessionID,
		Username:             extractUsernameFromUserID(session.UserID),
		CreatedAt:            session.CreatedAt,
		ExpiresAt:            expiresAt,
		RefreshToken:         session.RefreshToken,
		LastActivity:         session.LastUsedAt,
		UserAgent:            session.UserAgent,
		IPAddress:            session.IPAddress,
		DeviceID:             session.DeviceID,
		AuthMethod:           "", // Not stored in new model
		PreviousRefreshToken: "", // Would need to be added to model if required
		TokenRotatedAt:       time.Time{},
	}

	return storageSession, nil
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hashTokenForGSI creates a cryptographically secure hash of the token for GSI indexing
func hashTokenForGSI(token string) string {
	// Use SHA-256 to create a secure hash of the token
	hash := sha256.Sum256([]byte(token))
	// Return the first 16 characters of the hex-encoded hash for GSI key
	// This ensures deterministic, collision-resistant hashing suitable for DynamoDB GSI keys
	hexHash := hex.EncodeToString(hash[:])
	if len(hexHash) > 16 {
		return hexHash[:16]
	}
	return hexHash
}

// ===== Device Management Methods =====

// CreateDevice creates a new device record
func (r *AccountRepository) CreateDevice(ctx context.Context, device *storage.Device) error {
	if device == nil {
		return ErrorHandler.HandleCreateError(ErrDeviceValidationFailed, "device", "nil")
	}

	// Convert storage.Device to models.Device
	modelDevice := &models.Device{
		DeviceID:      device.DeviceID,
		Username:      device.Username,
		DeviceName:    device.DeviceName,
		DeviceType:    device.DeviceType,
		LastIPAddress: device.LastIPAddress,
		LastUserAgent: device.LastUserAgent,
		CreatedAt:     device.CreatedAt,
		LastSeenAt:    device.LastSeenAt,
		TrustLevel:    device.TrustLevel,
		Platform:      "",   // Not in storage.Device
		AppVersion:    "",   // Not in storage.Device
		Location:      "",   // Not in storage.Device
		Active:        true, // Default to active
	}

	// Ensure we have the required fields
	if err := common.ValidateRequiredParam("TrustLevel", modelDevice.TrustLevel); err != nil {
		modelDevice.TrustLevel = "untrusted" // Default trust level
	}

	// Update the keys using the model's method
	modelDevice.UpdateKeys()

	err := r.db.WithContext(ctx).Model(modelDevice).Create()
	if err != nil {
		r.logger.Error("failed to create device",
			zap.String("username", device.Username),
			zap.String("deviceID", device.DeviceID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "device", device.DeviceID)
	}

	r.logger.Debug("device created successfully",
		zap.String("username", device.Username),
		zap.String("deviceID", device.DeviceID))

	return nil
}

// GetDevice retrieves a device by ID
func (r *AccountRepository) GetDevice(ctx context.Context, deviceID string) (*storage.Device, error) {
	// Legacy implementation used inefficient scan, but we need to find by deviceID
	// Since we don't have the username, we need to scan or use a GSI
	// Using scan to match legacy behavior exactly
	var devices []models.Device

	err := r.db.WithContext(ctx).Model(&models.Device{}).
		Where("DeviceID", "=", deviceID).
		Where("SK", "BEGINS_WITH", "DEVICE#").
		Limit(1).
		All(&devices)

	if err != nil {
		r.logger.Error("failed to scan for device",
			zap.String("deviceID", deviceID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "device", deviceID)
	}

	if err := common.ValidateSliceNotEmpty("devices", devices); err != nil {
		return nil, ErrorHandler.HandleGetError(ErrDeviceNotFound, "device", deviceID)
	}

	device := devices[0]

	// Convert models.Device to storage.Device
	storageDevice := &storage.Device{
		ID:                device.DeviceID, // Map DeviceID to ID
		DeviceID:          device.DeviceID,
		Username:          device.Username,
		Endpoint:          "", // Not in models.Device
		PublicKey:         "", // Not in models.Device
		AuthKey:           "", // Not in models.Device
		ServerKey:         "", // Not in models.Device
		CreatedAt:         device.CreatedAt,
		UpdatedAt:         device.CreatedAt, // Use CreatedAt since we don't have UpdatedAt
		LastSeenAt:        device.LastSeenAt,
		UserAgent:         device.LastUserAgent,
		NotificationTypes: []string{}, // Not in models.Device
		DeviceName:        device.DeviceName,
		DeviceType:        device.DeviceType,
		LastIPAddress:     device.LastIPAddress,
		LastUserAgent:     device.LastUserAgent,
		TrustLevel:        device.TrustLevel,
	}

	return storageDevice, nil
}

// UpdateDevice updates an existing device
func (r *AccountRepository) UpdateDevice(ctx context.Context, device *storage.Device) error {
	if device == nil {
		return ErrorHandler.HandleUpdateError(ErrDeviceValidationFailed, "device", "nil")
	}

	// Get the existing device first
	var existingDevice models.Device
	err := r.db.WithContext(ctx).Model(&existingDevice).
		Where("PK", "=", fmt.Sprintf("USER#%s", device.Username)).
		Where("SK", "=", fmt.Sprintf("DEVICE#%s", device.DeviceID)).
		First(&existingDevice)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, "device", device.DeviceID)
		}
		r.logger.Error("failed to get device for update",
			zap.String("username", device.Username),
			zap.String("deviceID", device.DeviceID),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, "device", device.DeviceID)
	}

	// Update fields that legacy UpdateDevice modified
	existingDevice.TrustLevel = device.TrustLevel
	existingDevice.LastSeenAt = device.LastSeenAt
	existingDevice.LastIPAddress = device.LastIPAddress
	existingDevice.LastUserAgent = device.LastUserAgent

	// Update GSI keys since LastSeenAt changed
	existingDevice.UpdateKeys()

	err = r.db.WithContext(ctx).Model(&existingDevice).Update()
	if err != nil {
		r.logger.Error("failed to update device",
			zap.String("username", device.Username),
			zap.String("deviceID", device.DeviceID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "device", device.DeviceID)
	}

	r.logger.Debug("device updated successfully",
		zap.String("username", device.Username),
		zap.String("deviceID", device.DeviceID))

	return nil
}

// GetUserDevices gets all devices for a user
func (r *AccountRepository) GetUserDevices(ctx context.Context, username string) ([]*storage.Device, error) {
	var devices []models.Device

	err := r.db.WithContext(ctx).Model(&models.Device{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "DEVICE#").
		All(&devices)

	if err != nil {
		r.logger.Error("failed to query user devices",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "device", username)
	}

	// Convert models.Device slice to storage.Device slice
	result := make([]*storage.Device, len(devices))
	for i, device := range devices {
		result[i] = &storage.Device{
			ID:                device.DeviceID, // Map DeviceID to ID
			DeviceID:          device.DeviceID,
			Username:          device.Username,
			Endpoint:          "", // Not in models.Device
			PublicKey:         "", // Not in models.Device
			AuthKey:           "", // Not in models.Device
			ServerKey:         "", // Not in models.Device
			CreatedAt:         device.CreatedAt,
			UpdatedAt:         device.CreatedAt, // Use CreatedAt since we don't have UpdatedAt
			LastSeenAt:        device.LastSeenAt,
			UserAgent:         device.LastUserAgent,
			NotificationTypes: []string{}, // Not in models.Device
			DeviceName:        device.DeviceName,
			DeviceType:        device.DeviceType,
			LastIPAddress:     device.LastIPAddress,
			LastUserAgent:     device.LastUserAgent,
			TrustLevel:        device.TrustLevel,
		}
	}

	r.logger.Debug("retrieved user devices",
		zap.String("username", username),
		zap.Int("count", len(devices)))

	return result, nil
}

// CreateSessionFromStruct creates a new user session from storage.Session struct
func (r *AccountRepository) CreateSessionFromStruct(ctx context.Context, session *storage.Session) error {
	if session == nil {
		return ErrorHandler.HandleCreateError(ErrSessionValidationFailed, EntitySession, "nil")
	}

	// Create a models.Session from the storage.Session
	modelSession := &models.Session{
		SessionID:    session.SessionID,
		UserID:       fmt.Sprintf("USER#%s", session.Username),
		AccessToken:  session.RefreshToken, // Using RefreshToken as AccessToken for now
		RefreshToken: session.RefreshToken,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    time.Now(),
		LastUsedAt:   session.LastActivity,
		ExpiresAt:    session.ExpiresAt.Unix(),
		IPAddress:    session.IPAddress,
		UserAgent:    session.UserAgent,
		DeviceID:     session.DeviceID,
		IsRevoked:    false,
	}

	// Set scopes if needed (empty for now since not in storage.Session)
	modelSession.Scopes = []string{}

	err := r.db.WithContext(ctx).Model(modelSession).Create()
	if err != nil {
		r.logger.Error("failed to create session from struct",
			zap.String("sessionID", session.SessionID),
			zap.String("username", session.Username),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntitySession, session.SessionID)
	}

	r.logger.Debug("session created from struct successfully",
		zap.String("sessionID", session.SessionID),
		zap.String("username", session.Username))

	return nil
}

// ===== Recovery Token Methods =====

// StoreRecoveryToken stores a recovery token
func (r *AccountRepository) StoreRecoveryToken(ctx context.Context, key string, data map[string]interface{}) error {
	// Create recovery token model
	recoveryToken := &models.RecoveryToken{
		PK:        key, // Use key directly as PK
		Data:      data,
		CreatedAt: time.Now(),
	}

	// Update keys to set SK and TTL
	if err := recoveryToken.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Store in DynamoDB
	err := r.db.WithContext(ctx).Model(recoveryToken).Create()
	if err != nil {
		r.logger.Error("failed to store recovery token",
			zap.String("key", key),
			zap.Error(err))
		return err
	}

	r.logger.Debug("stored recovery token", zap.String("key", key))
	return nil
}

// GetRecoveryToken retrieves a recovery token
func (r *AccountRepository) GetRecoveryToken(ctx context.Context, key string) (map[string]interface{}, error) {
	var recoveryToken models.RecoveryToken

	// Query by PK and SK
	err := r.db.WithContext(ctx).Model(&models.RecoveryToken{}).
		Where("PK", "=", key).
		Where("SK", "=", "TOKEN").
		First(&recoveryToken)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return nil for not found (matching legacy behavior)
			r.logger.Debug("recovery token not found", zap.String("key", key))
			return nil, nil
		}
		r.logger.Error("failed to get recovery token",
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}

	r.logger.Debug("retrieved recovery token", zap.String("key", key))
	return recoveryToken.Data, nil
}

// DeleteRecoveryToken deletes a recovery token
func (r *AccountRepository) DeleteRecoveryToken(ctx context.Context, key string) error {
	// Delete by PK and SK
	err := r.db.WithContext(ctx).Model(&models.RecoveryToken{}).
		Where("PK", "=", key).
		Where("SK", "=", "TOKEN").
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to delete recovery token",
			zap.String("key", key),
			zap.Error(err))
		return err
	}

	r.logger.Debug("deleted recovery token", zap.String("key", key))
	return nil
}

// ===== WebAuthn Methods =====

// StoreWebAuthnChallenge stores a WebAuthn challenge
func (r *AccountRepository) StoreWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	if challenge == nil {
		return ErrorHandler.HandleCreateError(ErrWebAuthnValidationFailed, EntityWebAuthnChallenge, "nil")
	}

	// Convert storage.WebAuthnChallenge to models.WebAuthnChallenge
	modelChallenge := &models.WebAuthnChallenge{
		Challenge: challenge.Challenge,
		UserID:    challenge.UserID,
		ExpiresAt: challenge.ExpiresAt,
		Type:      challenge.Type,
	}

	// Convert SessionData to bytes if needed
	if challenge.SessionData != nil {
		// In real implementation, you'd properly serialize this
		// For now, assuming it's already serialized or can be converted
		if sessionBytes, ok := challenge.SessionData.([]byte); ok {
			modelChallenge.SessionData = sessionBytes
		}
	}

	// Call BeforeCreate to set keys and TTL
	err := modelChallenge.BeforeCreate()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnChallenge, challenge.Challenge)
	}

	// Store in DynamoDB
	err = r.db.WithContext(ctx).Model(modelChallenge).Create()
	if err != nil {
		r.logger.Error("failed to store WebAuthn challenge",
			zap.String("challenge", challenge.Challenge),
			zap.String("userID", challenge.UserID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnChallenge, challenge.Challenge)
	}

	r.logger.Debug("WebAuthn challenge stored successfully",
		zap.String("challenge", challenge.Challenge),
		zap.String("userID", challenge.UserID))

	return nil
}

// StoreWebAuthnCredential stores a WebAuthn credential
func (r *AccountRepository) StoreWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	if credential == nil {
		return ErrorHandler.HandleCreateError(ErrWebAuthnValidationFailed, EntityWebAuthnCredential, "nil")
	}

	// Convert storage.WebAuthnCredential to models.WebAuthnCredential
	modelCredential := &models.WebAuthnCredential{
		ID:              credential.ID,
		UserID:          credential.UserID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.AAGUID,
		SignCount:       credential.SignCount,
		CloneWarning:    credential.CloneWarning,
		BackupEligible:  credential.BackupEligible,
		BackupState:     credential.BackupState,
		CreatedAt:       credential.CreatedAt,
		LastUsedAt:      credential.LastUsedAt,
		Name:            credential.Name,
	}

	// Use LastUsed if LastUsedAt is zero
	if modelCredential.LastUsedAt.IsZero() && !credential.LastUsed.IsZero() {
		modelCredential.LastUsedAt = credential.LastUsed
	}

	// Call BeforeCreate to set keys
	err := modelCredential.BeforeCreate()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnCredential, credential.ID)
	}

	// Store in DynamoDB
	err = r.db.WithContext(ctx).Model(modelCredential).Create()
	if err != nil {
		r.logger.Error("failed to store WebAuthn credential",
			zap.String("credentialID", credential.ID),
			zap.String("userID", credential.UserID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnCredential, credential.ID)
	}

	r.logger.Debug("WebAuthn credential stored successfully",
		zap.String("credentialID", credential.ID),
		zap.String("userID", credential.UserID))

	return nil
}

// UpdateWebAuthnCredential updates a WebAuthn credential
func (r *AccountRepository) UpdateWebAuthnCredential(ctx context.Context, credentialID string, signCount uint32) error {
	if err := common.ValidateRequiredParam("credentialID", credentialID); err != nil {
		return ErrorHandler.HandleUpdateError(ErrWebAuthnValidationFailed, EntityWebAuthnCredential, "empty")
	}

	// We need to find the credential first since we don't have the username
	// This requires scanning, which matches the legacy inefficient approach
	var credentials []models.WebAuthnCredential

	err := r.db.WithContext(ctx).Model(&models.WebAuthnCredential{}).Where("ID", "=", credentialID).All(&credentials)
	if err != nil {
		r.logger.Error("failed to find WebAuthn credential for update",
			zap.String("credentialID", credentialID),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityWebAuthnCredential, credentialID)
	}

	if err := common.ValidateSliceNotEmpty("credentials", credentials); err != nil {
		return ErrorHandler.HandleGetError(ErrWebAuthnCredentialNotFound, EntityWebAuthnCredential, credentialID)
	}

	// Update the credential
	credential := credentials[0]
	credential.SignCount = signCount
	credential.LastUsedAt = time.Now()

	err = r.db.WithContext(ctx).Model(&credential).Update()
	if err != nil {
		r.logger.Error("failed to update WebAuthn credential",
			zap.String("credentialID", credentialID),
			zap.Uint32("signCount", signCount),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityWebAuthnCredential, credentialID)
	}

	r.logger.Debug("WebAuthn credential updated successfully",
		zap.String("credentialID", credentialID),
		zap.Uint32("signCount", signCount))

	return nil
}

// UpdateWalletLastUsed updates when a wallet was last used
func (r *AccountRepository) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return ErrorHandler.HandleUpdateError(ErrWalletValidationFailed, EntityWalletCredential, "username")
	}
	if err := common.ValidateRequiredParam("address", address); err != nil {
		return ErrorHandler.HandleUpdateError(ErrWalletValidationFailed, EntityWalletCredential, "address")
	}

	// Get the existing wallet credential
	var wallet models.WalletCredential
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).Where("PK", "=", fmt.Sprintf("USER#%s", username)).Where("SK", "=", fmt.Sprintf("WALLET#%s", strings.ToLower(address))).First(&wallet)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, EntityWalletCredential, address)
		}
		r.logger.Error("failed to get wallet credential for update",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityWalletCredential, address)
	}

	// Update the last used timestamp
	wallet.LastUsed = time.Now()

	err = r.db.WithContext(ctx).Model(&wallet).Update()
	if err != nil {
		r.logger.Error("failed to update wallet last used",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityWalletCredential, address)
	}

	r.logger.Debug("wallet last used updated successfully",
		zap.String("username", username),
		zap.String("address", address))

	return nil
}

// GetLinkedProviders gets all linked OAuth providers for a user
func (r *AccountRepository) GetLinkedProviders(ctx context.Context, username string) ([]string, error) {
	var providerAccounts []models.ProviderAccount

	// Query GSI2 to get all provider accounts for the user
	err := r.db.WithContext(ctx).Model(&models.ProviderAccount{}).
		Index("user-providers-index").
		Where("GSI2PK", "=", fmt.Sprintf("USER_PROVIDERS#%s", username)).
		All(&providerAccounts)

	if err != nil {
		if errors.IsNotFound(err) {
			// User has no linked providers
			return []string{}, nil
		}
		r.logger.Error("failed to get linked providers",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "provider", username)
	}

	// Extract unique provider names from active accounts
	providerSet := make(map[string]bool)
	for _, account := range providerAccounts {
		if account.IsActive {
			providerSet[account.Provider] = true
		}
	}

	// Convert set to slice
	providers := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		providers = append(providers, provider)
	}

	r.logger.Debug("retrieved linked providers",
		zap.String("username", username),
		zap.Strings("providers", providers))

	return providers, nil
}
