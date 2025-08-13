package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// AuditEventType represents the type of authentication event
type AuditEventType string

const (
	// Login events
	AuditLoginSuccess           AuditEventType = "auth.login.success"
	AuditLoginFailed            AuditEventType = "auth.login.failed"
	AuditLoginRateLimited       AuditEventType = "auth.login.rate_limited"
	AuditLoginSuspended         AuditEventType = "auth.login.suspended"
	AuditLoginNotApproved       AuditEventType = "auth.login.not_approved"
	AuditLoginTwoFactorRequired AuditEventType = "auth.login.2fa_required"
	AuditLoginTwoFactorFailed   AuditEventType = "auth.login.2fa_failed"
	
	// Logout events
	AuditLogout           AuditEventType = "auth.logout"
	AuditLogoutAllDevices AuditEventType = "auth.logout.all_devices"
	
	// Registration events
	AuditRegistrationStarted   AuditEventType = "auth.registration.started"
	AuditRegistrationCompleted AuditEventType = "auth.registration.completed"
	AuditRegistrationFailed    AuditEventType = "auth.registration.failed"
	AuditEmailVerification     AuditEventType = "auth.registration.email_verified"
	
	// Password events
	AuditPasswordResetRequested AuditEventType = "auth.password.reset_requested"
	AuditPasswordResetCompleted AuditEventType = "auth.password.reset_completed"
	AuditPasswordChanged        AuditEventType = "auth.password.changed"
	AuditPasswordChangeFailed   AuditEventType = "auth.password.change_failed"
	
	// OAuth events
	AuditOAuthAuthorizeStarted   AuditEventType = "auth.oauth.authorize_started"
	AuditOAuthAuthorizeCompleted AuditEventType = "auth.oauth.authorize_completed"
	AuditOAuthAuthorizeFailed    AuditEventType = "auth.oauth.authorize_failed"
	AuditOAuthTokenIssued        AuditEventType = "auth.oauth.token_issued"
	AuditOAuthTokenRefreshed     AuditEventType = "auth.oauth.token_refreshed"
	AuditOAuthTokenRevoked       AuditEventType = "auth.oauth.token_revoked"
	AuditOAuthTokenFailed        AuditEventType = "auth.oauth.token_failed"
	
	// WebAuthn events
	AuditWebAuthnRegistrationStarted   AuditEventType = "auth.webauthn.registration_started"
	AuditWebAuthnRegistrationCompleted AuditEventType = "auth.webauthn.registration_completed"
	AuditWebAuthnRegistrationFailed    AuditEventType = "auth.webauthn.registration_failed"
	AuditWebAuthnLoginStarted          AuditEventType = "auth.webauthn.login_started"
	AuditWebAuthnLoginCompleted        AuditEventType = "auth.webauthn.login_completed"
	AuditWebAuthnLoginFailed           AuditEventType = "auth.webauthn.login_failed"
	AuditWebAuthnCredentialRemoved     AuditEventType = "auth.webauthn.credential_removed"
	
	// Wallet events
	AuditWalletConnected     AuditEventType = "auth.wallet.connected"
	AuditWalletDisconnected  AuditEventType = "auth.wallet.disconnected"
	AuditWalletLoginStarted  AuditEventType = "auth.wallet.login_started"
	AuditWalletLoginCompleted AuditEventType = "auth.wallet.login_completed"
	AuditWalletLoginFailed    AuditEventType = "auth.wallet.login_failed"
	
	// Session events
	AuditSessionCreated     AuditEventType = "auth.session.created"
	AuditSessionRefreshed   AuditEventType = "auth.session.refreshed"
	AuditSessionRevoked     AuditEventType = "auth.session.revoked"
	AuditSessionExpired     AuditEventType = "auth.session.expired"
	AuditSessionInvalidated AuditEventType = "auth.session.invalidated"
	
	// Security events
	AuditSuspiciousActivity  AuditEventType = "auth.security.suspicious_activity"
	AuditAccountLocked       AuditEventType = "auth.security.account_locked"
	AuditAccountUnlocked     AuditEventType = "auth.security.account_unlocked"
	AuditIPBlocked           AuditEventType = "auth.security.ip_blocked"
	AuditIPUnblocked         AuditEventType = "auth.security.ip_unblocked"
	AuditBruteForceDetected  AuditEventType = "auth.security.brute_force_detected"
	AuditAnomalousLocation   AuditEventType = "auth.security.anomalous_location"
	AuditDeviceNotRecognized AuditEventType = "auth.security.device_not_recognized"
	
	// API Key events
	AuditAPIKeyCreated AuditEventType = "auth.api_key.created"
	AuditAPIKeyRevoked AuditEventType = "auth.api_key.revoked"
	AuditAPIKeyUsed    AuditEventType = "auth.api_key.used"
	AuditAPIKeyFailed  AuditEventType = "auth.api_key.failed"
	
	// 2FA events
	AuditTwoFactorEnabled  AuditEventType = "auth.2fa.enabled"
	AuditTwoFactorDisabled AuditEventType = "auth.2fa.disabled"
	AuditTwoFactorVerified AuditEventType = "auth.2fa.verified"
	AuditTwoFactorFailed   AuditEventType = "auth.2fa.failed"
	AuditRecoveryCodeUsed  AuditEventType = "auth.2fa.recovery_code_used"
)

// AuditSeverity represents the severity level of an audit event
type AuditSeverity string

const (
	SeverityInfo     AuditSeverity = "info"
	SeverityWarning  AuditSeverity = "warning"
	SeverityError    AuditSeverity = "error"
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
	RiskScore  float64 `json:"risk_score,omitempty"`
	RiskFlags  []string `json:"risk_flags,omitempty"`
	
	// Compliance fields
	DataRetentionDays int    `json:"data_retention_days,omitempty"`
	ComplianceFlags   []string `json:"compliance_flags,omitempty"`
}

// AuditLogger handles authentication audit logging
type AuditLogger struct {
	repos  StorageProvider
	logger *zap.Logger
	config *AuditConfig
}

// AuditConfig defines audit logging configuration
type AuditConfig struct {
	// Enable/disable audit logging
	Enabled bool
	
	// Storage configuration
	StoreToDB     bool
	StoreToFile   bool
	StoreToSIEM   bool // Security Information and Event Management
	
	// Log levels
	LogSuccessfulAuth bool
	LogFailedAuth     bool
	LogSecurityEvents bool
	LogAPIAccess      bool
	
	// Data retention
	RetentionDays int
	
	// Privacy settings
	HashIPAddresses   bool
	RedactSensitive   bool
	AnonymizeAfterDays int
	
	// Alert thresholds
	FailedLoginThreshold int
	SuspiciousIPThreshold int
	AlertOnAnomalousLocation bool
	AlertOnNewDevice bool
	
	// SIEM integration
	SIEMEndpoint string
	SIEMAPIKey   string
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(repos StorageProvider, logger *zap.Logger, config *AuditConfig) *AuditLogger {
	if config == nil {
		config = DefaultAuditConfig()
	}
	
	return &AuditLogger{
		repos:  repos,
		logger: logger,
		config: config,
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
	if event.ID == "" {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Severity == "" {
		event.Severity = al.determineSeverity(event.EventType, event.Success)
	}
	
	// Apply privacy settings
	if al.config.HashIPAddresses && event.IPAddress != "" {
		event.IPAddress = hashIP(event.IPAddress)
	}
	if al.config.RedactSensitive {
		al.redactSensitiveData(event)
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
			"credential_id": credentialID,
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
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}
	
	// Append to audit log file (implementation depends on your file storage strategy)
	// This is a placeholder - you might want to use a rotating file logger
	al.logger.Info("audit_event", zap.ByteString("event", data))
	
	return nil
}

// sendToSIEM sends the event to a SIEM system
func (al *AuditLogger) sendToSIEM(event *AuditEvent) error {
	if al.config.SIEMEndpoint == "" {
		return nil
	}
	
	// Convert to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}
	
	// Send to SIEM endpoint
	req, err := http.NewRequest("POST", al.config.SIEMEndpoint, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to create SIEM request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	if al.config.SIEMAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+al.config.SIEMAPIKey)
	}
	
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send to SIEM: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM returned error status: %d", resp.StatusCode)
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
func (al *AuditLogger) checkAlerts(ctx context.Context, event *AuditEvent) {
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
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func hashIP(ip string) string {
	// Simple hash for privacy (in production, use proper hashing)
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		// Keep first two octets, hash last two
		return fmt.Sprintf("%s.%s.xxx.xxx", parts[0], parts[1])
	}
	return "xxx.xxx.xxx.xxx"
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

func (al *AuditLogger) getRecentFailureCount(ctx context.Context, username string) int {
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
	return nil, fmt.Errorf("audit repository not available")
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
	return nil, fmt.Errorf("audit repository not available")
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
	return nil, fmt.Errorf("audit repository not available")
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
	return nil, fmt.Errorf("audit repository not available")
}

// convertToAuditEvents converts model audit logs to audit events
func (al *AuditLogger) convertToAuditEvents(logs []*models.AuthAuditLog) []*AuditEvent {
	events := make([]*AuditEvent, len(logs))
	for i, log := range logs {
		var metadata map[string]interface{}
		if log.Metadata != "" {
			_ = json.Unmarshal([]byte(log.Metadata), &metadata)
		}
		
		events[i] = &AuditEvent{
			ID:            log.ID,
			Timestamp:     log.Timestamp,
			EventType:     AuditEventType(log.EventType),
			Severity:      AuditSeverity(log.Severity),
			Username:      log.Username,
			UserID:        log.UserID,
			IPAddress:     log.IPAddress,
			UserAgent:     log.UserAgent,
			DeviceName:    log.DeviceName,
			SessionID:     log.SessionID,
			RequestID:     log.RequestID,
			Success:       log.Success,
			FailureReason: log.FailureReason,
			Country:       log.Country,
			City:          log.City,
			Region:        log.Region,
			Latitude:      log.Latitude,
			Longitude:     log.Longitude,
			RiskScore:     log.RiskScore,
			RiskFlags:     log.RiskFlags,
			Metadata:      metadata,
			DataRetentionDays: log.DataRetentionDays,
			ComplianceFlags:   log.ComplianceFlags,
		}
	}
	return events
}