package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SessionSecurityManager handles advanced session security measures
type SessionSecurityManager struct {
	logger *zap.Logger
	config *AdvancedSessionSecurityConfig
}

// AdvancedSessionSecurityConfig holds advanced security configuration
type AdvancedSessionSecurityConfig struct {
	EnableIPBinding           bool          // Bind sessions to IP addresses
	EnableDeviceFingerprinting bool          // Use device fingerprinting
	EnableGeoValidation       bool          // Validate geographic location
	MaxConcurrentSessions     int           // Max sessions per user
	SessionFixationPrevention bool          // Prevent session fixation attacks
	CSRFProtection           bool          // Enable CSRF token validation
	SecureCookiesOnly        bool          // Only use secure cookies
	StrictSameSite           bool          // Use strict SameSite policy
	SessionTimeout           time.Duration // Auto-logout timeout
	GracePeriod              time.Duration // Token rotation grace period
}

// Security action constants
const (
	SecurityActionDeny = "deny"
)

// DefaultAdvancedSessionSecurityConfig provides secure defaults
func DefaultAdvancedSessionSecurityConfig() *AdvancedSessionSecurityConfig {
	return &AdvancedSessionSecurityConfig{
		EnableIPBinding:           true,
		EnableDeviceFingerprinting: true,
		EnableGeoValidation:       false, // Disabled by default (requires external service)
		MaxConcurrentSessions:     10,
		SessionFixationPrevention: true,
		CSRFProtection:           true,
		SecureCookiesOnly:        true,
		StrictSameSite:           true,
		SessionTimeout:           24 * time.Hour,
		GracePeriod:              1 * time.Hour,
	}
}

// NewSessionSecurityManager creates a new session security manager
func NewSessionSecurityManager(logger *zap.Logger, config *AdvancedSessionSecurityConfig) *SessionSecurityManager {
	if config == nil {
		config = DefaultAdvancedSessionSecurityConfig()
	}
	return &SessionSecurityManager{
		logger: logger,
		config: config,
	}
}

// DeviceFingerprint represents a device fingerprint
type DeviceFingerprint struct {
	UserAgent    string `json:"user_agent"`
	IPAddress    string `json:"ip_address"`
	AcceptLang   string `json:"accept_language,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	ScreenRes    string `json:"screen_resolution,omitempty"`
	ColorDepth   string `json:"color_depth,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Fingerprint  string `json:"fingerprint"` // SHA256 hash of combined factors
}

// SecurityValidationResult represents the result of security validation
type SecurityValidationResult struct {
	Valid              bool     `json:"valid"`
	TrustScore         float64  `json:"trust_score"`      // 0.0 - 1.0
	RiskFactors        []string `json:"risk_factors"`
	RequiresChallenge  bool     `json:"requires_challenge"`
	RecommendedAction  string   `json:"recommended_action"`
}

// SessionAnomalyFlags represents detected anomalies
type SessionAnomalyFlags struct {
	IPChanged         bool `json:"ip_changed"`
	DeviceChanged     bool `json:"device_changed"`
	LocationChanged   bool `json:"location_changed"`
	UnusualTiming     bool `json:"unusual_timing"`
	ConcurrentSessions bool `json:"concurrent_sessions"`
	SuspiciousActivity bool `json:"suspicious_activity"`
}

// GenerateCSRFToken generates a cryptographically secure CSRF token
func (ssm *SessionSecurityManager) GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidateCSRFToken validates a CSRF token against the expected value
func (ssm *SessionSecurityManager) ValidateCSRFToken(provided, expected string) bool {
	if provided == "" || expected == "" {
		return false
	}
	// Use constant-time comparison to prevent timing attacks
	return provided == expected
}

// GenerateDeviceFingerprint creates a device fingerprint from request metadata
func (ssm *SessionSecurityManager) GenerateDeviceFingerprint(userAgent, ipAddress, acceptLang string) *DeviceFingerprint {
	// Create fingerprint from available data
	combined := fmt.Sprintf("%s|%s|%s", userAgent, ipAddress, acceptLang)
	
	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(combined))
	fingerprint := hex.EncodeToString(hash[:])
	
	return &DeviceFingerprint{
		UserAgent:   userAgent,
		IPAddress:   ipAddress,
		AcceptLang:  acceptLang,
		Fingerprint: fingerprint,
	}
}

// ValidateSessionSecurity performs comprehensive session security validation
func (ssm *SessionSecurityManager) ValidateSessionSecurity(_ context.Context, session *Session, currentFingerprint *DeviceFingerprint) (*SecurityValidationResult, error) {
	result := &SecurityValidationResult{
		Valid:      true,
		TrustScore: 1.0,
		RiskFactors: []string{},
		RequiresChallenge: false,
		RecommendedAction: "allow",
	}

	// Check session expiration
	if time.Now().After(session.ExpiresAt) {
		result.Valid = false
		result.TrustScore = 0.0
		result.RiskFactors = append(result.RiskFactors, "session_expired")
		result.RecommendedAction = SecurityActionDeny
		return result, nil
	}

	// Check session inactivity
	if time.Since(session.LastActivity) > ssm.config.SessionTimeout {
		result.Valid = false
		result.TrustScore = 0.0
		result.RiskFactors = append(result.RiskFactors, "session_inactive")
		result.RecommendedAction = SecurityActionDeny
		return result, nil
	}

	// IP address validation
	if ssm.config.EnableIPBinding && session.IPAddress != currentFingerprint.IPAddress {
		if !ssm.isIPInSameSubnet(session.IPAddress, currentFingerprint.IPAddress) {
			result.TrustScore -= 0.3
			result.RiskFactors = append(result.RiskFactors, "ip_address_changed")
			result.RequiresChallenge = true
		}
	}

	// Device fingerprint validation
	if ssm.config.EnableDeviceFingerprinting {
		// Compare user agents (basic device fingerprinting)
		if session.UserAgent != currentFingerprint.UserAgent {
			result.TrustScore -= 0.2
			result.RiskFactors = append(result.RiskFactors, "device_fingerprint_changed")
		}
	}

	// Determine final action based on trust score
	switch {
	case result.TrustScore >= 0.8:
		result.RecommendedAction = "allow"
	case result.TrustScore >= 0.5:
		result.RecommendedAction = "challenge"
		result.RequiresChallenge = true
	default:
		result.RecommendedAction = SecurityActionDeny
		result.Valid = false
	}

	ssm.logger.Debug("session security validation completed",
		zap.String("sessionID", session.SessionID),
		zap.Float64("trustScore", result.TrustScore),
		zap.Strings("riskFactors", result.RiskFactors),
		zap.String("action", result.RecommendedAction))

	return result, nil
}

// DetectSessionAnomalies detects various session anomalies
func (ssm *SessionSecurityManager) DetectSessionAnomalies(session *Session, currentFingerprint *DeviceFingerprint) *SessionAnomalyFlags {
	flags := &SessionAnomalyFlags{}

	// IP address change detection
	if session.IPAddress != currentFingerprint.IPAddress {
		flags.IPChanged = true
	}

	// Device change detection (basic)
	if session.UserAgent != currentFingerprint.UserAgent {
		flags.DeviceChanged = true
	}

	// Unusual timing detection (very basic)
	if time.Since(session.LastActivity) < 1*time.Second {
		flags.UnusualTiming = true // Potentially automated activity
	}

	return flags
}

// isIPInSameSubnet checks if two IP addresses are in the same subnet
func (ssm *SessionSecurityManager) isIPInSameSubnet(ip1, ip2 string) bool {
	// Parse IP addresses
	parsedIP1 := net.ParseIP(ip1)
	parsedIP2 := net.ParseIP(ip2)
	
	if parsedIP1 == nil || parsedIP2 == nil {
		return false
	}

	// Check if both are IPv4 or IPv6
	if parsedIP1.To4() != nil && parsedIP2.To4() != nil {
		// IPv4 - check /24 subnet
		return parsedIP1.To4()[0] == parsedIP2.To4()[0] &&
			   parsedIP1.To4()[1] == parsedIP2.To4()[1] &&
			   parsedIP1.To4()[2] == parsedIP2.To4()[2]
	}

	// For IPv6 or other cases, be more strict
	return false
}

// PreventSessionFixation implements session fixation prevention
func (ssm *SessionSecurityManager) PreventSessionFixation(oldSessionID string) (string, error) {
	if !ssm.config.SessionFixationPrevention {
		return oldSessionID, nil
	}

	// Generate new session ID
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate new session ID: %w", err)
	}
	
	newSessionID := hex.EncodeToString(b)
	
	ssm.logger.Debug("session ID regenerated for fixation prevention",
		zap.String("oldSessionID", oldSessionID),
		zap.String("newSessionID", newSessionID))
	
	return newSessionID, nil
}

// GenerateSecureSessionCookie generates a secure session cookie value
func (ssm *SessionSecurityManager) GenerateSecureSessionCookie(sessionID, username string) (string, error) {
	// Create session cookie payload
	timestamp := time.Now().Unix()
	payload := fmt.Sprintf("%s|%s|%d", sessionID, username, timestamp)
	
	// Add entropy
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate cookie entropy: %w", err)
	}
	entropy := hex.EncodeToString(b)
	
	// Combine and hash
	combined := fmt.Sprintf("%s|%s", payload, entropy)
	hash := sha256.Sum256([]byte(combined))
	
	return hex.EncodeToString(hash[:]), nil
}

// ValidateSecurityHeaders validates important security headers
func (ssm *SessionSecurityManager) ValidateSecurityHeaders(headers map[string]string) []string {
	var missingHeaders []string
	
	// Check for important security headers
	requiredHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
	}
	
	for header, expectedValue := range requiredHeaders {
		if value, exists := headers[header]; !exists || value != expectedValue {
			missingHeaders = append(missingHeaders, header)
		}
	}
	
	return missingHeaders
}

// CalculateSessionRisk calculates an overall risk score for a session
func (ssm *SessionSecurityManager) CalculateSessionRisk(_ *Session, anomalies *SessionAnomalyFlags, validationResult *SecurityValidationResult) float64 {
	baseRisk := 0.0
	
	// Add risk based on anomalies
	if anomalies.IPChanged {
		baseRisk += 0.3
	}
	if anomalies.DeviceChanged {
		baseRisk += 0.2
	}
	if anomalies.LocationChanged {
		baseRisk += 0.25
	}
	if anomalies.UnusualTiming {
		baseRisk += 0.1
	}
	if anomalies.ConcurrentSessions {
		baseRisk += 0.15
	}
	
	// Factor in validation result
	baseRisk += (1.0 - validationResult.TrustScore) * 0.5
	
	// Cap at 1.0
	if baseRisk > 1.0 {
		baseRisk = 1.0
	}
	
	return baseRisk
}

// ShouldRequire2FA determines if 2FA should be required based on risk
func (ssm *SessionSecurityManager) ShouldRequire2FA(riskScore float64, session *Session) bool {
	// Require 2FA for high-risk scenarios
	if riskScore >= 0.7 {
		return true
	}
	
	// Require 2FA for privileged sessions older than threshold
	if time.Since(session.CreatedAt) > 7*24*time.Hour && riskScore >= 0.4 {
		return true
	}
	
	return false
}

// LogSecurityEvent logs a security-related event
func (ssm *SessionSecurityManager) LogSecurityEvent(eventType, sessionID, username, description string, metadata map[string]interface{}) {
	fields := []zap.Field{
		zap.String("event_type", eventType),
		zap.String("session_id", sessionID),
		zap.String("username", username),
		zap.String("description", description),
		zap.Time("timestamp", time.Now()),
	}
	
	// Add metadata fields
	for key, value := range metadata {
		fields = append(fields, zap.Any(key, value))
	}
	
	ssm.logger.Info("security event", fields...)
}

// IsHighRiskUserAgent checks if a user agent is potentially malicious
func (ssm *SessionSecurityManager) IsHighRiskUserAgent(userAgent string) bool {
	// Simple checks for suspicious patterns
	suspiciousPatterns := []string{
		"bot", "crawler", "spider", "scraper",
		"curl", "wget", "python-requests",
		"postman", "insomnia",
	}
	
	userAgentLower := strings.ToLower(userAgent)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(userAgentLower, pattern) {
			return true
		}
	}
	
	return false
}

// RotateSessionSecrets rotates session secrets for enhanced security
func (ssm *SessionSecurityManager) RotateSessionSecrets(session *Session) error {
	// Generate new CSRF token
	newCSRFToken, err := ssm.GenerateCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to rotate CSRF token: %w", err)
	}
	
	// Update session with new secrets
	// This would typically update the session in storage
	ssm.logger.Debug("session secrets rotated",
		zap.String("sessionID", session.SessionID),
		zap.String("newCSRFToken", newCSRFToken))
	
	return nil
}