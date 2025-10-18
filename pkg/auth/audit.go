package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/privacy"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// AuditEventType represents the type of authentication event
type AuditEventType string

const (
	// AuditLoginSuccess represents a successful login attempt
	AuditLoginSuccess AuditEventType = "auth.login.success"
	// AuditLoginFailed represents a failed login attempt
	AuditLoginFailed AuditEventType = "auth.login.failed"
	// AuditLoginRateLimited represents a login attempt blocked by rate limiting
	AuditLoginRateLimited AuditEventType = "auth.login.rate_limited"
	// AuditLoginSuspended represents a login attempt by a suspended user
	AuditLoginSuspended AuditEventType = "auth.login.suspended"
	// AuditLoginNotApproved represents a login attempt that was not approved
	AuditLoginNotApproved AuditEventType = "auth.login.not_approved"
	// AuditLoginTwoFactorRequired represents a login attempt requiring 2FA
	AuditLoginTwoFactorRequired AuditEventType = "auth.login.2fa_required"
	// AuditLoginTwoFactorFailed represents a failed 2FA login attempt
	AuditLoginTwoFactorFailed AuditEventType = "auth.login.2fa_failed"

	// AuditLogout represents a user logout event
	AuditLogout AuditEventType = "auth.logout"
	// AuditLogoutAllDevices represents a logout from all devices event
	AuditLogoutAllDevices AuditEventType = "auth.logout.all_devices"

	// AuditRegistrationStarted represents the start of user registration
	AuditRegistrationStarted AuditEventType = "auth.registration.started"
	// AuditRegistrationCompleted represents the completion of user registration
	AuditRegistrationCompleted AuditEventType = "auth.registration.completed"
	// AuditRegistrationFailed represents a failed user registration
	AuditRegistrationFailed AuditEventType = "auth.registration.failed"
	// AuditEmailVerification represents an email verification event
	AuditEmailVerification AuditEventType = "auth.registration.email_verified"

	// AuditPasswordResetRequested represents a password reset request
	AuditPasswordResetRequested AuditEventType = "auth.password.reset_requested"
	// AuditPasswordResetCompleted represents a completed password reset
	AuditPasswordResetCompleted AuditEventType = "auth.password.reset_completed"
	// AuditPasswordChanged represents a successful password change
	AuditPasswordChanged AuditEventType = "auth.password.changed"
	// AuditPasswordChangeFailed represents a failed password change attempt
	AuditPasswordChangeFailed AuditEventType = "auth.password.change_failed"

	// AuditOAuthAuthorizeStarted represents the start of OAuth authorization
	AuditOAuthAuthorizeStarted AuditEventType = "auth.oauth.authorize_started" //nolint:gosec // false positive - audit event type
	// AuditOAuthAuthorizeCompleted represents completed OAuth authorization
	AuditOAuthAuthorizeCompleted AuditEventType = "auth.oauth.authorize_completed" //nolint:gosec // false positive - audit event type
	// AuditOAuthAuthorizeFailed represents a failed OAuth authorization
	AuditOAuthAuthorizeFailed AuditEventType = "auth.oauth.authorize_failed" //nolint:gosec // false positive - audit event type
	// AuditOAuthTokenIssued represents an OAuth token being issued
	AuditOAuthTokenIssued AuditEventType = "auth.oauth.token_issued" //nolint:gosec // false positive - audit event type
	// AuditOAuthTokenRefreshed represents an OAuth token being refreshed
	AuditOAuthTokenRefreshed AuditEventType = "auth.oauth.token_refreshed" //nolint:gosec // false positive - audit event type
	// AuditOAuthTokenRevoked represents an OAuth token being revoked
	AuditOAuthTokenRevoked AuditEventType = "auth.oauth.token_revoked" //nolint:gosec // false positive - audit event type
	// AuditOAuthTokenFailed represents a failed OAuth token operation
	AuditOAuthTokenFailed AuditEventType = "auth.oauth.token_failed" //nolint:gosec // false positive - audit event type

	// AuditWebAuthnRegistrationStarted represents the start of WebAuthn credential registration
	AuditWebAuthnRegistrationStarted AuditEventType = "auth.webauthn.registration_started" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnRegistrationCompleted represents the completion of WebAuthn credential registration
	AuditWebAuthnRegistrationCompleted AuditEventType = "auth.webauthn.registration_completed" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnRegistrationFailed represents a failed WebAuthn credential registration
	AuditWebAuthnRegistrationFailed AuditEventType = "auth.webauthn.registration_failed" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnLoginStarted represents the start of a WebAuthn login attempt
	AuditWebAuthnLoginStarted AuditEventType = "auth.webauthn.login_started" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnLoginCompleted represents a successful WebAuthn login
	AuditWebAuthnLoginCompleted AuditEventType = "auth.webauthn.login_completed" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnLoginFailed represents a failed WebAuthn login attempt
	AuditWebAuthnLoginFailed AuditEventType = "auth.webauthn.login_failed" //nolint:gosec // false positive - audit event type
	// AuditWebAuthnCredentialRemoved represents a WebAuthn credential being removed
	AuditWebAuthnCredentialRemoved AuditEventType = "auth.webauthn.credential_removed" //nolint:gosec // false positive - audit event type

	// AuditWalletConnected represents a successful wallet connection
	AuditWalletConnected AuditEventType = "auth.wallet.connected"
	// AuditWalletDisconnected represents a wallet being disconnected
	AuditWalletDisconnected AuditEventType = "auth.wallet.disconnected"
	// AuditWalletLoginStarted represents the start of a wallet-based login attempt
	AuditWalletLoginStarted AuditEventType = "auth.wallet.login_started"
	// AuditWalletLoginCompleted represents a successful wallet-based login
	AuditWalletLoginCompleted AuditEventType = "auth.wallet.login_completed"
	// AuditWalletLoginFailed represents a failed wallet-based login attempt
	AuditWalletLoginFailed AuditEventType = "auth.wallet.login_failed"

	// AuditSessionCreated represents a new session creation
	AuditSessionCreated AuditEventType = "auth.session.created"
	// AuditSessionRefreshed represents a session being refreshed
	AuditSessionRefreshed AuditEventType = "auth.session.refreshed"
	// AuditSessionRevoked represents a session being manually revoked
	AuditSessionRevoked AuditEventType = "auth.session.revoked"
	// AuditSessionExpired represents a session expiring naturally
	AuditSessionExpired AuditEventType = "auth.session.expired"
	// AuditSessionInvalidated represents a session being invalidated
	AuditSessionInvalidated AuditEventType = "auth.session.invalidated"

	// AuditSuspiciousActivity represents detected suspicious authentication activity
	AuditSuspiciousActivity AuditEventType = "auth.security.suspicious_activity"
	// AuditAccountLocked represents an account being locked due to security concerns
	AuditAccountLocked AuditEventType = "auth.security.account_locked"
	// AuditAccountUnlocked represents an account being unlocked
	AuditAccountUnlocked AuditEventType = "auth.security.account_unlocked"
	// AuditIPBlocked represents an IP address being blocked
	AuditIPBlocked AuditEventType = "auth.security.ip_blocked"
	// AuditIPUnblocked represents an IP address being unblocked
	AuditIPUnblocked AuditEventType = "auth.security.ip_unblocked"
	// AuditBruteForceDetected represents detection of a brute force attack
	AuditBruteForceDetected AuditEventType = "auth.security.brute_force_detected"
	// AuditAnomalousLocation represents a login from an unusual location
	AuditAnomalousLocation AuditEventType = "auth.security.anomalous_location"
	// AuditDeviceNotRecognized represents a login from an unrecognized device
	AuditDeviceNotRecognized AuditEventType = "auth.security.device_not_recognized"

	// AuditAPIKeyCreated represents API key creation event
	AuditAPIKeyCreated AuditEventType = "auth.api_key.created" //nolint:gosec // false positive - audit event type
	// AuditAPIKeyRevoked represents an API key being revoked
	AuditAPIKeyRevoked AuditEventType = "auth.api_key.revoked" //nolint:gosec // false positive - audit event type
	// AuditAPIKeyUsed represents an API key being used for authentication
	AuditAPIKeyUsed AuditEventType = "auth.api_key.used" //nolint:gosec // false positive - audit event type
	// AuditAPIKeyFailed represents a failed API key authentication attempt
	AuditAPIKeyFailed AuditEventType = "auth.api_key.failed" //nolint:gosec // false positive - audit event type

	// AuditTwoFactorEnabled represents two-factor authentication being enabled
	AuditTwoFactorEnabled AuditEventType = "auth.2fa.enabled"
	// AuditTwoFactorDisabled represents two-factor authentication being disabled
	AuditTwoFactorDisabled AuditEventType = "auth.2fa.disabled"
	// AuditTwoFactorVerified represents successful two-factor authentication verification
	AuditTwoFactorVerified AuditEventType = "auth.2fa.verified"
	// AuditTwoFactorFailed represents a failed two-factor authentication attempt
	AuditTwoFactorFailed AuditEventType = "auth.2fa.failed"
	// AuditRecoveryCodeUsed represents a 2FA recovery code being used
	AuditRecoveryCodeUsed AuditEventType = "auth.2fa.recovery_code_used"
)

// AuditSeverity represents the severity level of an audit event
type AuditSeverity string

const (
	// SeverityInfo represents informational audit events
	SeverityInfo AuditSeverity = "info"
	// SeverityWarning represents warning-level audit events
	SeverityWarning AuditSeverity = "warning"
	// SeverityError represents an error-level audit event
	SeverityError AuditSeverity = "error"
	// SeverityCritical represents a critical-level audit event
	SeverityCritical AuditSeverity = "critical"
)

// AuditEvent represents an authentication audit log entry
type AuditEvent struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	EventType     AuditEventType         `json:"event_type"`
	Severity      AuditSeverity          `json:"severity"`
	Username      string                 `json:"username,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	IPAddress     string                 `json:"ip_address"`
	UserAgent     string                 `json:"user_agent,omitempty"`
	DeviceName    string                 `json:"device_name,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	RequestID     string                 `json:"request_id,omitempty"`
	Success       bool                   `json:"success"`
	FailureReason string                 `json:"failure_reason,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`

	// Geographic information
	Country   string  `json:"country,omitempty"`
	City      string  `json:"city,omitempty"`
	Region    string  `json:"region,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`

	// Risk assessment
	RiskScore float64  `json:"risk_score,omitempty"`
	RiskFlags []string `json:"risk_flags,omitempty"`

	// Compliance fields
	DataRetentionDays int      `json:"data_retention_days,omitempty"`
	ComplianceFlags   []string `json:"compliance_flags,omitempty"`
}

// AuditLogger handles authentication audit logging
type AuditLogger struct {
	repos         StorageProvider
	logger        *zap.Logger
	config        *AuditConfig
	privacyHasher *privacy.Hasher
}

// AuditConfig defines audit logging configuration
type AuditConfig struct {
	// Enable/disable audit logging
	Enabled bool

	// Storage configuration
	StoreToDB   bool
	StoreToFile bool
	StoreToSIEM bool // Security Information and Event Management

	// Log levels
	LogSuccessfulAuth bool
	LogFailedAuth     bool
	LogSecurityEvents bool
	LogAPIAccess      bool

	// Data retention
	RetentionDays int

	// Privacy settings
	HashIPAddresses    bool
	RedactSensitive    bool
	AnonymizeAfterDays int

	// Alert thresholds
	FailedLoginThreshold     int
	SuspiciousIPThreshold    int
	AlertOnAnomalousLocation bool
	AlertOnNewDevice         bool

	// SIEM integration
	SIEMEndpoint string
	SIEMAPIKey   string
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(repos StorageProvider, logger *zap.Logger, config *AuditConfig) *AuditLogger {
	if config == nil {
		config = DefaultAuditConfig()
	}

	// Initialize privacy hasher if privacy hashing is enabled
	var privacyHasher *privacy.Hasher
	appConfig := lesserconfig.Get()
	if appConfig.EnablePrivacyHashing && appConfig.PrivacyMasterKey != "" {
		hasher, err := privacy.NewHasherFromMasterKey(appConfig.PrivacyMasterKey)
		if err != nil {
			logger.Error("failed to initialize privacy hasher, privacy hashing disabled",
				zap.Error(err))
		} else {
			privacyHasher = hasher
			logger.Info("privacy hashing enabled for audit logs")
		}
	}

	return &AuditLogger{
		repos:         repos,
		logger:        logger,
		config:        config,
		privacyHasher: privacyHasher,
	}
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		Enabled:                  true,
		StoreToDB:                true,
		StoreToFile:              false,
		StoreToSIEM:              false,
		LogSuccessfulAuth:        true,
		LogFailedAuth:            true,
		LogSecurityEvents:        true,
		LogAPIAccess:             false,
		RetentionDays:            90,
		HashIPAddresses:          false,
		RedactSensitive:          true,
		AnonymizeAfterDays:       365,
		FailedLoginThreshold:     5,
		SuspiciousIPThreshold:    10,
		AlertOnAnomalousLocation: true,
		AlertOnNewDevice:         true,
	}
}

// LogEvent logs an authentication event
func (al *AuditLogger) LogEvent(ctx context.Context, event *AuditEvent) error {
	if !al.config.Enabled {
		return nil
	}

	// Set default values
	if err := common.ValidateRequiredParam("event.ID", event.ID); err != nil {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if err := common.ValidateRequiredParam("event.Severity", string(event.Severity)); err != nil {
		event.Severity = al.determineSeverity(event.EventType, event.Success)
	}

	// Apply privacy settings
	if al.config.HashIPAddresses && event.IPAddress != "" {
		event.IPAddress = al.hashIPSecure(event.IPAddress)
	}
	if al.config.RedactSensitive {
		al.redactSensitiveData(event)
	}

	// Apply privacy hashing if enabled
	if al.privacyHasher != nil {
		al.applyPrivacyHashing(event)
	}

	// Calculate risk score if not provided
	if event.RiskScore == 0 {
		event.RiskScore = al.calculateRiskScore(ctx, event)
	}

	// Store to different destinations
	var lastErr error

	if al.config.StoreToDB {
		if err := al.storeToDB(ctx, event); err != nil {
			al.logger.Error("failed to store audit event to DB",
				zap.String("event_id", event.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	if al.config.StoreToFile {
		if err := al.storeToFile(event); err != nil {
			al.logger.Error("failed to store audit event to file",
				zap.String("event_id", event.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	if al.config.StoreToSIEM {
		if err := al.sendToSIEM(event); err != nil {
			al.logger.Error("failed to send audit event to SIEM",
				zap.String("event_id", event.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	// Log to application logger
	al.logToZap(event)

	// Check for alerts
	al.checkAlerts(ctx, event)

	return lastErr
}

// LogLogin logs a login attempt
func (al *AuditLogger) LogLogin(ctx context.Context, username, ipAddress, userAgent, deviceName string, success bool, failureReason string) {
	eventType := AuditLoginSuccess
	if !success {
		eventType = AuditLoginFailed
	}

	event := &AuditEvent{
		EventType:     eventType,
		Username:      username,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		DeviceName:    deviceName,
		Success:       success,
		FailureReason: failureReason,
		Metadata: map[string]interface{}{
			"authentication_method": "password",
		},
	}

	_ = al.LogEvent(ctx, event)
}

// LogOAuthToken logs OAuth token operations
func (al *AuditLogger) LogOAuthToken(ctx context.Context, clientID, username, ipAddress string, eventType AuditEventType, scopes []string, success bool, err error) {
	failureReason := ""
	if err != nil {
		failureReason = err.Error()
	}

	event := &AuditEvent{
		EventType:     eventType,
		Username:      username,
		IPAddress:     ipAddress,
		Success:       success,
		FailureReason: failureReason,
		Metadata: map[string]interface{}{
			"client_id": clientID,
			"scopes":    scopes,
		},
	}

	_ = al.LogEvent(ctx, event)
}

// LogWebAuthn logs WebAuthn operations
func (al *AuditLogger) LogWebAuthn(ctx context.Context, username, ipAddress, userAgent string, eventType AuditEventType, credentialID string, success bool, err error) {
	failureReason := ""
	if err != nil {
		failureReason = err.Error()
	}

	event := &AuditEvent{
		EventType:     eventType,
		Username:      username,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Success:       success,
		FailureReason: failureReason,
		Metadata: map[string]interface{}{
			"credential_id":         credentialID,
			"authentication_method": "webauthn",
		},
	}

	_ = al.LogEvent(ctx, event)
}

// LogSession logs session operations
func (al *AuditLogger) LogSession(ctx context.Context, username, sessionID, ipAddress string, eventType AuditEventType) {
	event := &AuditEvent{
		EventType: eventType,
		Username:  username,
		SessionID: sessionID,
		IPAddress: ipAddress,
		Success:   true,
		Metadata: map[string]interface{}{
			"session_operation": strings.ToLower(string(eventType)),
		},
	}

	_ = al.LogEvent(ctx, event)
}

// LogSecurityEvent logs security-related events
func (al *AuditLogger) LogSecurityEvent(ctx context.Context, eventType AuditEventType, username, ipAddress string, metadata map[string]interface{}) {
	event := &AuditEvent{
		EventType: eventType,
		Username:  username,
		IPAddress: ipAddress,
		Success:   false,
		Severity:  SeverityWarning,
		Metadata:  metadata,
	}

	// Security events are typically higher severity
	if eventType == AuditBruteForceDetected || eventType == AuditAccountLocked {
		event.Severity = SeverityCritical
	}

	_ = al.LogEvent(ctx, event)
}

// storeToDB stores the audit event to the database
func (al *AuditLogger) storeToDB(ctx context.Context, event *AuditEvent) error {
	// Convert metadata to JSON string if present
	var metadataStr string
	if event.Metadata != nil {
		data, err := json.Marshal(event.Metadata)
		if err != nil {
			al.logger.Warn("failed to marshal audit metadata", zap.Error(err))
		} else {
			metadataStr = string(data)
		}
	}

	// Use the audit repository if available
	if auditRepo := al.repos.Audit(); auditRepo != nil {
		return auditRepo.StoreAuditEvent(
			ctx,
			string(event.EventType),
			string(event.Severity),
			event.Username,
			event.UserID,
			event.IPAddress,
			event.UserAgent,
			event.DeviceName,
			event.SessionID,
			event.RequestID,
			event.Success,
			event.FailureReason,
			event.Metadata,
		)
	}

	// Fallback to structured logging if repository not available
	fields := []zap.Field{
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.EventType)),
		zap.Time("timestamp", event.Timestamp),
		zap.String("username", event.Username),
		zap.String("user_id", event.UserID),
		zap.String("ip_address", event.IPAddress),
		zap.String("user_agent", event.UserAgent),
		zap.String("device_name", event.DeviceName),
		zap.String("session_id", event.SessionID),
		zap.String("request_id", event.RequestID),
		zap.Bool("success", event.Success),
		zap.String("failure_reason", event.FailureReason),
		zap.String("severity", string(event.Severity)),
		zap.Float64("risk_score", event.RiskScore),
		zap.Strings("risk_flags", event.RiskFlags),
		zap.String("country", event.Country),
		zap.String("city", event.City),
	}

	if metadataStr != "" {
		fields = append(fields, zap.String("metadata", metadataStr))
	}

	// Log as structured audit event
	al.logger.Info("AUDIT_LOG", fields...)

	return nil
}

// storeToFile appends the audit event to a log file
func (al *AuditLogger) storeToFile(event *AuditEvent) error {
	// Convert to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return errors.Join(ErrAuditEventMarshal, err)
	}

	// Append to audit log file (implementation depends on your file storage strategy)
	// This is a placeholder - you might want to use a rotating file logger
	al.logger.Info("audit_event", zap.ByteString("event", data))

	return nil
}

// sendToSIEM sends the event to a SIEM system
func (al *AuditLogger) sendToSIEM(event *AuditEvent) error {
	if err := common.ValidateRequiredParam("al.config.SIEMEndpoint", al.config.SIEMEndpoint); err != nil {
		return nil
	}

	// Convert to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return errors.Join(ErrAuditEventMarshal, err)
	}

	// Send to SIEM endpoint
	req, err := http.NewRequest("POST", al.config.SIEMEndpoint, strings.NewReader(string(data)))
	if err != nil {
		return errors.Join(ErrSIEMRequestCreation, err)
	}

	req.Header.Set("Content-Type", "application/json")
	if al.config.SIEMAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+al.config.SIEMAPIKey)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Join(ErrSIEMTransmission, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log but don't return error - this is cleanup
			_ = err
		}
	}()

	if resp.StatusCode >= 400 {
		al.logger.Error("SIEM response error",
			zap.Int("status_code", resp.StatusCode))
		return ErrSIEMResponseError
	}

	return nil
}

// logToZap logs the event using the application logger
func (al *AuditLogger) logToZap(event *AuditEvent) {
	fields := []zap.Field{
		zap.String("event_type", string(event.EventType)),
		zap.String("username", event.Username),
		zap.String("ip_address", event.IPAddress),
		zap.Bool("success", event.Success),
		zap.String("severity", string(event.Severity)),
		zap.Float64("risk_score", event.RiskScore),
	}

	if event.FailureReason != "" {
		fields = append(fields, zap.String("failure_reason", event.FailureReason))
	}

	switch event.Severity {
	case SeverityCritical:
		al.logger.Error("auth_audit", fields...)
	case SeverityError:
		al.logger.Error("auth_audit", fields...)
	case SeverityWarning:
		al.logger.Warn("auth_audit", fields...)
	default:
		al.logger.Info("auth_audit", fields...)
	}
}

// determineSeverity determines the severity level based on event type and success
func (al *AuditLogger) determineSeverity(eventType AuditEventType, success bool) AuditSeverity {
	// Critical events
	criticalEvents := []AuditEventType{
		AuditBruteForceDetected,
		AuditAccountLocked,
		AuditSuspiciousActivity,
	}
	for _, e := range criticalEvents {
		if eventType == e {
			return SeverityCritical
		}
	}

	// Error events (failed security operations)
	if !success {
		errorEvents := []AuditEventType{
			AuditLoginFailed,
			AuditPasswordChangeFailed,
			AuditOAuthAuthorizeFailed,
			AuditWebAuthnLoginFailed,
			AuditWalletLoginFailed,
			AuditTwoFactorFailed,
		}
		for _, e := range errorEvents {
			if eventType == e {
				return SeverityError
			}
		}
	}

	// Warning events
	warningEvents := []AuditEventType{
		AuditLoginRateLimited,
		AuditLoginSuspended,
		AuditLoginNotApproved,
		AuditAnomalousLocation,
		AuditDeviceNotRecognized,
		AuditIPBlocked,
	}
	for _, e := range warningEvents {
		if eventType == e {
			return SeverityWarning
		}
	}

	return SeverityInfo
}

// calculateRiskScore calculates a risk score for the event
func (al *AuditLogger) calculateRiskScore(ctx context.Context, event *AuditEvent) float64 {
	score := 0.0

	// Failed authentication attempts increase risk
	if !event.Success {
		score += 20.0
	}

	// Check for suspicious patterns
	if event.EventType == AuditBruteForceDetected {
		score += 50.0
	}

	if event.EventType == AuditAnomalousLocation {
		score += 30.0
	}

	if event.EventType == AuditDeviceNotRecognized {
		score += 15.0
	}

	// Check recent failure count for this IP/user
	if event.Username != "" {
		if count := al.getRecentFailureCount(ctx, event.Username); count > al.config.FailedLoginThreshold {
			score += float64(count) * 5.0
		}
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// checkAlerts checks if any alerts should be triggered
func (al *AuditLogger) checkAlerts(_ context.Context, event *AuditEvent) {
	// Check for high risk score
	if event.RiskScore > 70 {
		al.logger.Warn("high risk authentication event detected",
			zap.String("event_id", event.ID),
			zap.String("username", event.Username),
			zap.Float64("risk_score", event.RiskScore))
		// Trigger alert (implement your alert mechanism)
	}

	// Check for brute force
	if event.EventType == AuditBruteForceDetected {
		al.logger.Error("brute force attack detected",
			zap.String("username", event.Username),
			zap.String("ip_address", event.IPAddress))
		// Trigger security alert
	}

	// Check for anomalous location
	if al.config.AlertOnAnomalousLocation && event.EventType == AuditAnomalousLocation {
		al.logger.Warn("login from anomalous location",
			zap.String("username", event.Username),
			zap.String("country", event.Country),
			zap.String("city", event.City))
		// Send notification to user
	}
}

// Helper functions

func generateEventID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID on crypto error
		return fmt.Sprintf("event_%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// hashIPSecure provides cryptographically secure IP hashing with fallback to simple masking
func (al *AuditLogger) hashIPSecure(ip string) string {
	if al.privacyHasher != nil {
		// Use secure privacy hashing if available
		hashedIP, err := al.privacyHasher.HashIP(ip)
		if err != nil {
			al.logger.Warn("failed to hash IP with privacy hasher, falling back to simple masking",
				zap.String("ip", ip),
				zap.Error(err))
		} else {
			return hashedIP
		}
	}

	// Fallback to simple masking for backward compatibility
	return hashIPSimple(ip)
}

// hashIPSimple provides simple IP masking (legacy behavior)
func hashIPSimple(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		// Keep first two octets, mask last two
		return fmt.Sprintf("%s.%s.xxx.xxx", parts[0], parts[1])
	}
	return "xxx.xxx.xxx.xxx"
}

// applyPrivacyHashing applies privacy hashing to sensitive fields in audit events
func (al *AuditLogger) applyPrivacyHashing(event *AuditEvent) {
	if al.privacyHasher == nil {
		return
	}

	// Hash IP address if not already hashed
	if event.IPAddress != "" && !al.config.HashIPAddresses {
		if hashedIP, err := al.privacyHasher.HashIP(event.IPAddress); err == nil {
			event.IPAddress = hashedIP
		} else {
			al.logger.Warn("failed to hash IP address",
				zap.String("ip", event.IPAddress),
				zap.Error(err))
		}
	}

	// Hash username if present
	if event.Username != "" {
		if hashedUsername, err := al.privacyHasher.HashUsername(event.Username); err == nil {
			// Store original username length for analytics
			if event.Metadata == nil {
				event.Metadata = make(map[string]interface{})
			}
			event.Metadata["original_username_length"] = len(event.Username)
			event.Username = hashedUsername
		} else {
			al.logger.Warn("failed to hash username",
				zap.String("username", event.Username),
				zap.Error(err))
		}
	}

	// Hash device name as it may contain PII
	if event.DeviceName != "" {
		if hashedDevice, err := al.privacyHasher.HashPII(event.DeviceName); err == nil {
			event.DeviceName = hashedDevice
		} else {
			al.logger.Warn("failed to hash device name",
				zap.String("device", event.DeviceName),
				zap.Error(err))
		}
	}

	// Hash sensitive metadata fields
	if event.Metadata != nil {
		al.hashSensitiveMetadata(event.Metadata)
	}
}

// hashSensitiveMetadata applies privacy hashing to sensitive fields in metadata
func (al *AuditLogger) hashSensitiveMetadata(metadata map[string]interface{}) {
	sensitiveFields := []string{"email", "phone", "address", "name", "real_name", "full_name"}

	for key, value := range metadata {
		keyLower := strings.ToLower(key)

		// Check if this is a sensitive field
		isSensitive := false
		for _, sensitiveField := range sensitiveFields {
			if strings.Contains(keyLower, sensitiveField) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			if strValue, ok := value.(string); ok && strValue != "" {
				if hashedValue, err := al.privacyHasher.HashPII(strValue); err == nil {
					metadata[key] = hashedValue
				} else {
					al.logger.Warn("failed to hash sensitive metadata field",
						zap.String("field", key),
						zap.Error(err))
				}
			}
		}
	}
}

func (al *AuditLogger) redactSensitiveData(event *AuditEvent) {
	// Redact sensitive fields in metadata
	if event.Metadata != nil {
		sensitiveKeys := []string{"password", "token", "secret", "key", "credential"}
		for key := range event.Metadata {
			for _, sensitive := range sensitiveKeys {
				if strings.Contains(strings.ToLower(key), sensitive) {
					event.Metadata[key] = "[REDACTED]"
				}
			}
		}
	}
}

func (al *AuditLogger) getRecentFailureCount(_ context.Context, _ string) int {
	// Query recent failures from storage
	// This is a simplified version - implement actual query
	return 0
}

// Query methods for audit logs

// GetUserAuditLogs retrieves audit logs for a specific user
func (al *AuditLogger) GetUserAuditLogs(ctx context.Context, username string, limit int) ([]*AuditEvent, error) {
	if auditRepo := al.repos.Audit(); auditRepo != nil {
		logs, err := auditRepo.GetUserAuditLogs(ctx, username, limit, time.Time{}, time.Time{})
		if err != nil {
			return nil, err
		}
		return al.convertToAuditEvents(logs), nil
	}
	return nil, ErrAuditRepositoryUnavailable
}

// GetIPAuditLogs retrieves audit logs for a specific IP address
func (al *AuditLogger) GetIPAuditLogs(ctx context.Context, ipAddress string, limit int) ([]*AuditEvent, error) {
	if auditRepo := al.repos.Audit(); auditRepo != nil {
		logs, err := auditRepo.GetIPAuditLogs(ctx, ipAddress, limit, time.Time{}, time.Time{})
		if err != nil {
			return nil, err
		}
		return al.convertToAuditEvents(logs), nil
	}
	return nil, ErrAuditRepositoryUnavailable
}

// GetSessionAuditLogs retrieves audit logs for a specific session
func (al *AuditLogger) GetSessionAuditLogs(ctx context.Context, sessionID string) ([]*AuditEvent, error) {
	if auditRepo := al.repos.Audit(); auditRepo != nil {
		logs, err := auditRepo.GetSessionAuditLogs(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return al.convertToAuditEvents(logs), nil
	}
	return nil, ErrAuditRepositoryUnavailable
}

// GetSecurityEvents retrieves security events within a time range
func (al *AuditLogger) GetSecurityEvents(ctx context.Context, startTime, endTime time.Time, severityFilter []AuditSeverity) ([]*AuditEvent, error) {
	if auditRepo := al.repos.Audit(); auditRepo != nil {
		var events []*AuditEvent
		for _, severity := range severityFilter {
			logs, err := auditRepo.GetSecurityEvents(ctx, string(severity), startTime, endTime, 100)
			if err != nil {
				return nil, err
			}
			events = append(events, al.convertToAuditEvents(logs)...)
		}
		return events, nil
	}
	return nil, ErrAuditRepositoryUnavailable
}

// convertToAuditEvents converts model audit logs to audit events
func (al *AuditLogger) convertToAuditEvents(logs []*models.AuthAuditLog) []*AuditEvent {
	events := make([]*AuditEvent, len(logs))
	for i, log := range logs {
		var metadata map[string]interface{}
		if log.Metadata != "" {
			if err := json.Unmarshal([]byte(log.Metadata), &metadata); err != nil {
				al.logger.Warn("failed to unmarshal audit log metadata", zap.Error(err), zap.String("log_id", log.ID))
			}
		}

		events[i] = &AuditEvent{
			ID:                log.ID,
			Timestamp:         log.Timestamp,
			EventType:         AuditEventType(log.EventType),
			Severity:          AuditSeverity(log.Severity),
			Username:          log.Username,
			UserID:            log.UserID,
			IPAddress:         log.IPAddress,
			UserAgent:         log.UserAgent,
			DeviceName:        log.DeviceName,
			SessionID:         log.SessionID,
			RequestID:         log.RequestID,
			Success:           log.Success,
			FailureReason:     log.FailureReason,
			Country:           log.Country,
			City:              log.City,
			Region:            log.Region,
			Latitude:          log.Latitude,
			Longitude:         log.Longitude,
			RiskScore:         log.RiskScore,
			RiskFlags:         log.RiskFlags,
			Metadata:          metadata,
			DataRetentionDays: log.DataRetentionDays,
			ComplianceFlags:   log.ComplianceFlags,
		}
	}
	return events
}
