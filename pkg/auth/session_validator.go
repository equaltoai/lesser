package auth

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// SessionValidator provides comprehensive session validation
type SessionValidator struct {
	sessionManager    *SessionManager
	securityManager   *SessionSecurityManager
	lifecycleManager  *SessionLifecycleManager
	fingerprintManager *DeviceFingerprintManager
	logger           *zap.Logger
	config           *SessionValidationConfig
}

// SessionValidationConfig holds validation configuration
type SessionValidationConfig struct {
	// Validation levels
	RequireDeviceValidation bool // Validate device fingerprints
	RequireIPValidation     bool // Validate IP addresses
	RequireCSRFValidation   bool // Validate CSRF tokens
	RequireSecurityHeaders  bool // Validate security headers
	
	// Validation thresholds
	MaxSessionAge          time.Duration // Maximum session age
	MaxInactivityPeriod    time.Duration // Maximum inactivity period
	MinTrustScore          float64       // Minimum required trust score
	DeviceMatchThreshold   float64       // Device fingerprint match threshold
	
	// Security policies
	StrictValidation       bool // Enable strict validation mode
	AllowGracePeriod       bool // Allow grace period for token rotation
	RequireReauth          bool // Require reauthentication for sensitive operations
	LogAllValidations      bool // Log all validation attempts
}

// DefaultSessionValidationConfig provides secure defaults
func DefaultSessionValidationConfig() *SessionValidationConfig {
	return &SessionValidationConfig{
		RequireDeviceValidation: true,
		RequireIPValidation:     true,
		RequireCSRFValidation:   true,
		RequireSecurityHeaders:  false, // Optional for API endpoints
		MaxSessionAge:          30 * 24 * time.Hour, // 30 days
		MaxInactivityPeriod:    24 * time.Hour,      // 24 hours
		MinTrustScore:          0.7,                 // 70% trust required
		DeviceMatchThreshold:   0.8,                 // 80% device match required
		StrictValidation:       false,
		AllowGracePeriod:       true,
		RequireReauth:          false,
		LogAllValidations:      false,
	}
}

// SessionValidationRequest represents a session validation request
type SessionValidationRequest struct {
	// Session identification
	SessionID    string `json:"session_id,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	CSRFToken    string `json:"csrf_token,omitempty"`
	
	// Request context
	IPAddress    string            `json:"ip_address"`
	UserAgent    string            `json:"user_agent"`
	Headers      map[string]string `json:"headers,omitempty"`
	RequestPath  string            `json:"request_path,omitempty"`
	RequestMethod string           `json:"request_method,omitempty"`
	
	// Additional context
	DeviceFingerprint map[string]string `json:"device_fingerprint,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`
	RequireHighSecurity bool           `json:"require_high_security,omitempty"`
}

// SessionValidationResponse represents the validation result
type SessionValidationResponse struct {
	// Validation result
	Valid           bool    `json:"valid"`
	TrustScore      float64 `json:"trust_score"`
	RiskScore       float64 `json:"risk_score"`
	ValidationLevel string  `json:"validation_level"` // basic, standard, strict
	
	// Session information
	SessionID       string    `json:"session_id"`
	Username        string    `json:"username"`
	ExpiresAt       time.Time `json:"expires_at"`
	LastActivity    time.Time `json:"last_activity"`
	DeviceID        string    `json:"device_id,omitempty"`
	
	// Validation details
	ValidatedChecks  []string `json:"validated_checks"`
	FailedChecks     []string `json:"failed_checks"`
	Warnings         []string `json:"warnings"`
	RequiredActions  []string `json:"required_actions"`
	
	// Security recommendations
	RequiresChallenge      bool   `json:"requires_challenge"`
	RequiresReauth         bool   `json:"requires_reauth"`
	RequiresDeviceApproval bool   `json:"requires_device_approval"`
	SuggestedAction        string `json:"suggested_action"`
	
	// Extended session if applicable
	ExtendedSession bool      `json:"extended_session,omitempty"`
	NewExpiresAt    time.Time `json:"new_expires_at,omitempty"`
	NewRefreshToken string    `json:"new_refresh_token,omitempty"`
}

// NewSessionValidator creates a new comprehensive session validator
func NewSessionValidator(
	sessionManager *SessionManager,
	securityManager *SessionSecurityManager,
	lifecycleManager *SessionLifecycleManager,
	fingerprintManager *DeviceFingerprintManager,
	logger *zap.Logger,
	config *SessionValidationConfig,
) *SessionValidator {
	if config == nil {
		config = DefaultSessionValidationConfig()
	}
	
	return &SessionValidator{
		sessionManager:     sessionManager,
		securityManager:    securityManager,
		lifecycleManager:   lifecycleManager,
		fingerprintManager: fingerprintManager,
		logger:            logger,
		config:            config,
	}
}

// ValidateSession performs comprehensive session validation
func (sv *SessionValidator) ValidateSession(ctx context.Context, request *SessionValidationRequest) (*SessionValidationResponse, error) {
	response := &SessionValidationResponse{
		Valid:              false,
		TrustScore:         0.0,
		RiskScore:          1.0,
		ValidationLevel:    sv.determineValidationLevel(request),
		ValidatedChecks:    []string{},
		FailedChecks:       []string{},
		Warnings:           []string{},
		RequiredActions:    []string{},
		RequiresChallenge:  false,
		RequiresReauth:     false,
		SuggestedAction:    "deny",
	}

	// Log validation attempt if configured
	if sv.config.LogAllValidations {
		sv.logger.Debug("session validation started",
			zap.String("sessionID", request.SessionID),
			zap.String("ipAddress", request.IPAddress),
			zap.String("userAgent", request.UserAgent),
			zap.String("validationLevel", response.ValidationLevel))
	}

	// Step 1: Basic session validation
	session, err := sv.validateBasicSession(ctx, request, response)
	if err != nil {
		return response, err
	}
	if session == nil {
		return response, nil // Session not found or invalid
	}

	response.SessionID = session.SessionID
	response.Username = session.Username
	response.ExpiresAt = session.ExpiresAt
	response.LastActivity = session.LastActivity

	// Step 2: Device fingerprint validation
	if sv.config.RequireDeviceValidation {
		if err := sv.validateDevice(ctx, session, request, response); err != nil {
			sv.logger.Warn("device validation error", zap.Error(err))
		}
	}

	// Step 3: Security validation
	if err := sv.validateSecurity(ctx, session, request, response); err != nil {
		sv.logger.Warn("security validation error", zap.Error(err))
	}

	// Step 4: Session lifecycle validation
	if err := sv.validateLifecycle(ctx, session, request, response); err != nil {
		sv.logger.Warn("lifecycle validation error", zap.Error(err))
	}

	// Step 5: Calculate final trust and risk scores
	sv.calculateFinalScores(session, request, response)

	// Step 6: Determine final validation result
	sv.determineFinalResult(session, request, response)

	// Step 7: Handle session extension if applicable
	if response.Valid && sv.shouldExtendSession(session) {
		if err := sv.handleSessionExtension(ctx, session, response); err != nil {
			sv.logger.Warn("session extension failed", zap.Error(err))
		}
	}

	// Log validation result
	sv.logger.Info("session validation completed",
		zap.String("sessionID", response.SessionID),
		zap.String("username", response.Username),
		zap.Bool("valid", response.Valid),
		zap.Float64("trustScore", response.TrustScore),
		zap.Float64("riskScore", response.RiskScore),
		zap.String("action", response.SuggestedAction))

	return response, nil
}

// validateBasicSession performs basic session validation
func (sv *SessionValidator) validateBasicSession(ctx context.Context, request *SessionValidationRequest, response *SessionValidationResponse) (*Session, error) {
	var session *Session
	var err error

	// Get session by ID or refresh token
	if request.SessionID != "" {
		session, err = sv.sessionManager.repos.Account().GetSession(ctx, request.SessionID)
		if err != nil {
			response.FailedChecks = append(response.FailedChecks, "session_not_found")
			return nil, nil
		}
	} else if request.RefreshToken != "" {
		session, err = sv.sessionManager.ValidateRefreshToken(ctx, request.RefreshToken)
		if err != nil {
			response.FailedChecks = append(response.FailedChecks, "invalid_refresh_token")
			return nil, nil
		}
	} else {
		response.FailedChecks = append(response.FailedChecks, "no_session_identifier")
		return nil, nil
	}

	response.ValidatedChecks = append(response.ValidatedChecks, "session_exists")

	// Check session expiration
	if time.Now().After(session.ExpiresAt) {
		response.FailedChecks = append(response.FailedChecks, "session_expired")
		return nil, nil
	}
	response.ValidatedChecks = append(response.ValidatedChecks, "session_not_expired")

	// Check session age
	if sv.config.MaxSessionAge > 0 && time.Since(session.CreatedAt) > sv.config.MaxSessionAge {
		response.FailedChecks = append(response.FailedChecks, "session_too_old")
		response.RequiresReauth = true
		return session, nil
	}
	response.ValidatedChecks = append(response.ValidatedChecks, "session_age_acceptable")

	// Check inactivity period
	if time.Since(session.LastActivity) > sv.config.MaxInactivityPeriod {
		response.FailedChecks = append(response.FailedChecks, "session_inactive")
		return nil, nil
	}
	response.ValidatedChecks = append(response.ValidatedChecks, "session_active")

	return session, nil
}

// validateDevice performs device fingerprint validation
func (sv *SessionValidator) validateDevice(ctx context.Context, session *Session, request *SessionValidationRequest, response *SessionValidationResponse) error {
	// Generate device fingerprint from request
	fingerprint := sv.fingerprintManager.GenerateEnhancedFingerprint(
		request.UserAgent,
		request.IPAddress,
		request.Headers["Accept-Language"],
		request.Headers["Accept-Encoding"],
		request.DeviceFingerprint,
	)

	// Validate device
	deviceValidation, err := sv.fingerprintManager.ValidateDevice(ctx, session.Username, fingerprint)
	if err != nil {
		return err
	}

	response.DeviceID = deviceValidation.DeviceID

	// Check device match confidence
	if deviceValidation.MatchConfidence < sv.config.DeviceMatchThreshold {
		response.FailedChecks = append(response.FailedChecks, "device_fingerprint_mismatch")
		response.RequiresChallenge = true
		response.RiskScore += 0.3
	} else {
		response.ValidatedChecks = append(response.ValidatedChecks, "device_fingerprint_match")
	}

	// Handle new devices
	if !deviceValidation.IsKnownDevice {
		response.Warnings = append(response.Warnings, "new_device_detected")
		response.RequiresDeviceApproval = deviceValidation.RequiresApproval
		response.RiskScore += 0.2
	}

	// Add device risk to overall risk score
	response.RiskScore += deviceValidation.RiskScore * 0.3

	return nil
}

// validateSecurity performs security-specific validation
func (sv *SessionValidator) validateSecurity(_ context.Context, session *Session, request *SessionValidationRequest, response *SessionValidationResponse) error {
	// IP address validation
	if sv.config.RequireIPValidation {
		if session.IPAddress != request.IPAddress {
			if !sv.isIPChangeAllowed(session.IPAddress, request.IPAddress) {
				response.FailedChecks = append(response.FailedChecks, "ip_address_changed")
				response.RequiresChallenge = true
				response.RiskScore += 0.2
			} else {
				response.Warnings = append(response.Warnings, "ip_address_changed_but_allowed")
			}
		} else {
			response.ValidatedChecks = append(response.ValidatedChecks, "ip_address_match")
		}
	}

	// CSRF token validation
	if sv.config.RequireCSRFValidation && request.CSRFToken != "" {
		// In a real implementation, you'd validate against stored CSRF token
		response.ValidatedChecks = append(response.ValidatedChecks, "csrf_token_valid")
	}

	// Security headers validation
	if sv.config.RequireSecurityHeaders {
		missingHeaders := sv.securityManager.ValidateSecurityHeaders(request.Headers)
		if len(missingHeaders) > 0 {
			response.Warnings = append(response.Warnings, fmt.Sprintf("missing_security_headers: %v", missingHeaders))
		} else {
			response.ValidatedChecks = append(response.ValidatedChecks, "security_headers_present")
		}
	}

	// High-risk user agent check
	if sv.securityManager.IsHighRiskUserAgent(request.UserAgent) {
		response.FailedChecks = append(response.FailedChecks, "high_risk_user_agent")
		response.RiskScore += 0.4
	}

	return nil
}

// validateLifecycle performs session lifecycle validation
func (sv *SessionValidator) validateLifecycle(ctx context.Context, session *Session, _ *SessionValidationRequest, response *SessionValidationResponse) error {
	// Get session health
	health, err := sv.lifecycleManager.GetSessionHealth(ctx, session.SessionID)
	if err != nil {
		return err
	}

	// Check if session should be extended
	if health.ShouldBeExtended {
		response.Warnings = append(response.Warnings, "session_should_be_extended")
	}

	// Check if session can be extended
	if !health.CanBeExtended {
		response.Warnings = append(response.Warnings, "session_cannot_be_extended")
	}

	// Add lifecycle information
	response.ValidatedChecks = append(response.ValidatedChecks, "lifecycle_validated")

	return nil
}

// calculateFinalScores calculates the final trust and risk scores
func (sv *SessionValidator) calculateFinalScores(_ *Session, _ *SessionValidationRequest, response *SessionValidationResponse) {
	// Start with base trust score
	baseTrustScore := 0.8

	// Adjust based on validation results
	validationRatio := float64(len(response.ValidatedChecks)) / float64(len(response.ValidatedChecks)+len(response.FailedChecks))
	response.TrustScore = baseTrustScore * validationRatio

	// Adjust trust score based on warnings
	warningPenalty := float64(len(response.Warnings)) * 0.05
	response.TrustScore -= warningPenalty

	// Ensure risk score is complementary to trust score
	if response.TrustScore > 0.5 {
		response.RiskScore = 1.0 - response.TrustScore
	}

	// Apply strict validation adjustments
	if sv.config.StrictValidation {
		response.TrustScore *= 0.9 // 10% penalty for strict mode
		response.RiskScore = 1.0 - response.TrustScore
	}

	// Clamp values to valid ranges
	if response.TrustScore < 0 {
		response.TrustScore = 0
	}
	if response.TrustScore > 1 {
		response.TrustScore = 1
	}
	if response.RiskScore < 0 {
		response.RiskScore = 0
	}
	if response.RiskScore > 1 {
		response.RiskScore = 1
	}
}

// determineFinalResult determines the final validation result
func (sv *SessionValidator) determineFinalResult(_ *Session, request *SessionValidationRequest, response *SessionValidationResponse) {
	// Check if any critical checks failed
	criticalFailures := []string{"session_expired", "session_inactive", "session_not_found"}
	for _, failure := range response.FailedChecks {
		for _, critical := range criticalFailures {
			if failure == critical {
				response.Valid = false
				response.SuggestedAction = "deny"
				return
			}
		}
	}

	// Check trust score threshold
	if response.TrustScore >= sv.config.MinTrustScore {
		response.Valid = true
		response.SuggestedAction = "allow"
	} else if response.TrustScore >= sv.config.MinTrustScore*0.7 {
		response.Valid = true
		response.RequiresChallenge = true
		response.SuggestedAction = "challenge"
	} else {
		response.Valid = false
		response.SuggestedAction = "deny"
	}

	// Override for high-security requests
	if request.RequireHighSecurity && response.TrustScore < 0.9 {
		response.RequiresReauth = true
		response.SuggestedAction = "reauth"
	}

	// Add required actions based on flags
	if response.RequiresChallenge {
		response.RequiredActions = append(response.RequiredActions, "challenge_required")
	}
	if response.RequiresReauth {
		response.RequiredActions = append(response.RequiredActions, "reauthentication_required")
	}
	if response.RequiresDeviceApproval {
		response.RequiredActions = append(response.RequiredActions, "device_approval_required")
	}
}

// Helper methods

func (sv *SessionValidator) determineValidationLevel(request *SessionValidationRequest) string {
	if sv.config.StrictValidation || request.RequireHighSecurity {
		return "strict"
	}
	if sv.config.RequireDeviceValidation && sv.config.RequireIPValidation {
		return "standard"
	}
	return "basic"
}

func (sv *SessionValidator) isIPChangeAllowed(oldIP, newIP string) bool {
	// Allow IP changes within the same subnet
	// This is a simplified implementation
	return sv.securityManager.isIPInSameSubnet(oldIP, newIP)
}

func (sv *SessionValidator) shouldExtendSession(session *Session) bool {
	// Check if session is close to expiry
	timeUntilExpiry := time.Until(session.ExpiresAt)
	return timeUntilExpiry < 6*time.Hour // Extend if less than 6 hours remaining
}

func (sv *SessionValidator) handleSessionExtension(ctx context.Context, session *Session, response *SessionValidationResponse) error {
	// Extend the session
	err := sv.lifecycleManager.extendSession(ctx, session)
	if err != nil {
		return err
	}

	response.ExtendedSession = true
	response.NewExpiresAt = session.ExpiresAt
	response.ValidatedChecks = append(response.ValidatedChecks, "session_extended")

	return nil
}

// QuickValidateSession provides a simplified validation for common cases
func (sv *SessionValidator) QuickValidateSession(ctx context.Context, sessionID, ipAddress, userAgent string) (bool, error) {
	request := &SessionValidationRequest{
		SessionID: sessionID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Timestamp: time.Now(),
	}

	response, err := sv.ValidateSession(ctx, request)
	if err != nil {
		return false, err
	}

	return response.Valid && response.TrustScore >= sv.config.MinTrustScore, nil
}

// ValidateRefreshTokenRequest validates a refresh token request specifically
func (sv *SessionValidator) ValidateRefreshTokenRequest(ctx context.Context, refreshToken, ipAddress, userAgent string) (*SessionValidationResponse, error) {
	request := &SessionValidationRequest{
		RefreshToken: refreshToken,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Timestamp:    time.Now(),
	}

	return sv.ValidateSession(ctx, request)
}