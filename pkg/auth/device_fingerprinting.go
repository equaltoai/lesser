package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// DeviceFingerprintManager handles device identification and tracking
type DeviceFingerprintManager struct {
	repos  StorageProvider
	logger *zap.Logger
	config *DeviceFingerprintConfig
}

// DeviceFingerprintConfig holds device fingerprinting configuration
type DeviceFingerprintConfig struct {
	EnableFingerprinting    bool          // Enable device fingerprinting
	StrictFingerprinting    bool          // Strict fingerprint matching
	FingerprintTTL         time.Duration // How long to keep fingerprints
	TrustNewDevices        bool          // Automatically trust new devices
	RequireDeviceApproval  bool          // Require manual device approval
	MaxDevicesPerUser      int           // Maximum devices per user
	DeviceTrustThreshold   time.Duration // Time before device becomes trusted
}

// DefaultDeviceFingerprintConfig provides secure defaults
func DefaultDeviceFingerprintConfig() *DeviceFingerprintConfig {
	return &DeviceFingerprintConfig{
		EnableFingerprinting:   true,
		StrictFingerprinting:   false,
		FingerprintTTL:        90 * 24 * time.Hour, // 90 days
		TrustNewDevices:       false,
		RequireDeviceApproval: false,
		MaxDevicesPerUser:     20,
		DeviceTrustThreshold:  7 * 24 * time.Hour, // 7 days
	}
}

// EnhancedDeviceFingerprint represents a comprehensive device fingerprint
type EnhancedDeviceFingerprint struct {
	// Basic identifiers
	UserAgent     string `json:"user_agent"`
	IPAddress     string `json:"ip_address"`
	AcceptLang    string `json:"accept_language,omitempty"`
	AcceptEncoding string `json:"accept_encoding,omitempty"`
	
	// Browser fingerprinting
	Timezone      string `json:"timezone,omitempty"`
	ScreenRes     string `json:"screen_resolution,omitempty"`
	ColorDepth    string `json:"color_depth,omitempty"`
	Platform      string `json:"platform,omitempty"`
	CookieEnabled string `json:"cookie_enabled,omitempty"`
	
	// Network fingerprinting
	IPVersion     string `json:"ip_version"`      // IPv4 or IPv6
	ASN           string `json:"asn,omitempty"`   // Autonomous System Number
	ISP           string `json:"isp,omitempty"`   // Internet Service Provider
	Country       string `json:"country,omitempty"`
	Region        string `json:"region,omitempty"`
	
	// Behavioral fingerprinting
	RequestTiming time.Duration `json:"request_timing,omitempty"`
	RequestOrder  []string      `json:"request_order,omitempty"`
	
	// Computed values
	BasicFingerprint    string `json:"basic_fingerprint"`    // Hash of basic attributes
	ExtendedFingerprint string `json:"extended_fingerprint"` // Hash of all attributes
	FingerprintEntropy  float64 `json:"fingerprint_entropy"`  // Uniqueness score
}

// DeviceInfo represents information about a user's device
type DeviceInfo struct {
	DeviceID         string                     `json:"device_id"`
	Username         string                     `json:"username"`
	DeviceName       string                     `json:"device_name"`
	DeviceType       string                     `json:"device_type"`
	Fingerprint      *EnhancedDeviceFingerprint `json:"fingerprint"`
	TrustLevel       string                     `json:"trust_level"`
	CreatedAt        time.Time                  `json:"created_at"`
	LastSeenAt       time.Time                  `json:"last_seen_at"`
	LastIPAddress    string                     `json:"last_ip_address"`
	SessionCount     int                        `json:"session_count"`
	IsApproved       bool                       `json:"is_approved"`
	RequiresApproval bool                       `json:"requires_approval"`
}

// DeviceValidationResult represents device validation results
type DeviceValidationResult struct {
	IsKnownDevice       bool     `json:"is_known_device"`
	DeviceID           string   `json:"device_id"`
	TrustLevel         string   `json:"trust_level"`
	MatchConfidence    float64  `json:"match_confidence"`
	RequiresChallenge  bool     `json:"requires_challenge"`
	RequiresApproval   bool     `json:"requires_approval"`
	ChangedAttributes  []string `json:"changed_attributes"`
	RiskScore          float64  `json:"risk_score"`
}

// NewDeviceFingerprintManager creates a new device fingerprint manager
func NewDeviceFingerprintManager(repos StorageProvider, logger *zap.Logger, config *DeviceFingerprintConfig) *DeviceFingerprintManager {
	if config == nil {
		config = DefaultDeviceFingerprintConfig()
	}
	
	return &DeviceFingerprintManager{
		repos:  repos,
		logger: logger,
		config: config,
	}
}

// GenerateEnhancedFingerprint creates a comprehensive device fingerprint
func (dfm *DeviceFingerprintManager) GenerateEnhancedFingerprint(userAgent, ipAddress, acceptLang, acceptEncoding string, additionalData map[string]string) *EnhancedDeviceFingerprint {
	fingerprint := &EnhancedDeviceFingerprint{
		UserAgent:      userAgent,
		IPAddress:      ipAddress,
		AcceptLang:     acceptLang,
		AcceptEncoding: acceptEncoding,
		IPVersion:      dfm.detectIPVersion(ipAddress),
	}

	// Extract additional data
	if timezone, exists := additionalData["timezone"]; exists {
		fingerprint.Timezone = timezone
	}
	if screenRes, exists := additionalData["screen_resolution"]; exists {
		fingerprint.ScreenRes = screenRes
	}
	if colorDepth, exists := additionalData["color_depth"]; exists {
		fingerprint.ColorDepth = colorDepth
	}
	if platform, exists := additionalData["platform"]; exists {
		fingerprint.Platform = platform
	}

	// Parse additional info from User-Agent
	fingerprint.Platform = dfm.extractPlatformFromUserAgent(userAgent)

	// Generate fingerprint hashes
	fingerprint.BasicFingerprint = dfm.generateBasicFingerprint(fingerprint)
	fingerprint.ExtendedFingerprint = dfm.generateExtendedFingerprint(fingerprint)
	fingerprint.FingerprintEntropy = dfm.calculateFingerprintEntropy(fingerprint)

	return fingerprint
}

// ValidateDevice validates a device fingerprint against known devices
func (dfm *DeviceFingerprintManager) ValidateDevice(ctx context.Context, username string, fingerprint *EnhancedDeviceFingerprint) (*DeviceValidationResult, error) {
	result := &DeviceValidationResult{
		IsKnownDevice:     false,
		MatchConfidence:   0.0,
		RequiresChallenge: false,
		RequiresApproval:  false,
		ChangedAttributes: []string{},
		RiskScore:         0.0,
	}

	// Get user's known devices
	devices, err := dfm.repos.Account().GetUserDevices(ctx, username)
	if err != nil {
		return result, fmt.Errorf("failed to get user devices: %w", err)
	}

	// Find best matching device
	var bestMatch *storage.Device
	var bestConfidence float64

	for _, device := range devices {
		confidence := dfm.calculateDeviceMatchConfidence(device, fingerprint)
		if confidence > bestConfidence {
			bestConfidence = confidence
			bestMatch = device
		}
	}

	// Determine if device is known based on confidence threshold
	confidenceThreshold := 0.8
	if dfm.config.StrictFingerprinting {
		confidenceThreshold = 0.95
	}

	if bestMatch != nil && bestConfidence >= confidenceThreshold {
		result.IsKnownDevice = true
		result.DeviceID = bestMatch.DeviceID
		result.TrustLevel = bestMatch.TrustLevel
		result.MatchConfidence = bestConfidence
		
		// Check for changes in device attributes
		result.ChangedAttributes = dfm.detectDeviceChanges(bestMatch, fingerprint)
		
		// Calculate risk score based on changes
		result.RiskScore = dfm.calculateDeviceRiskScore(bestMatch, fingerprint, result.ChangedAttributes)
		
		// Determine if challenges/approvals are needed
		result.RequiresChallenge = result.RiskScore > 0.5 || len(result.ChangedAttributes) > 2
		result.RequiresApproval = result.RiskScore > 0.8 && dfm.config.RequireDeviceApproval
	} else {
		// New device
		result.IsKnownDevice = false
		result.RequiresChallenge = !dfm.config.TrustNewDevices
		result.RequiresApproval = dfm.config.RequireDeviceApproval
		result.RiskScore = dfm.calculateNewDeviceRiskScore(fingerprint)
	}

	dfm.logger.Debug("device validation completed",
		zap.String("username", username),
		zap.Bool("isKnownDevice", result.IsKnownDevice),
		zap.Float64("matchConfidence", result.MatchConfidence),
		zap.Float64("riskScore", result.RiskScore))

	return result, nil
}

// RegisterNewDevice registers a new device for a user
func (dfm *DeviceFingerprintManager) RegisterNewDevice(ctx context.Context, username string, fingerprint *EnhancedDeviceFingerprint, deviceName string) (*DeviceInfo, error) {
	// Check device limits
	if err := dfm.enforceDeviceLimits(ctx, username); err != nil {
		return nil, err
	}

	// Generate device ID
	deviceID := dfm.generateDeviceID(fingerprint)

	// Determine trust level
	trustLevel := "untrusted"
	if dfm.config.TrustNewDevices {
		trustLevel = "trusted"
	}

	// Create device record
	device := &storage.Device{
		DeviceID:       deviceID,
		Username:       username,
		DeviceName:     deviceName,
		DeviceType:     dfm.detectDeviceType(fingerprint.UserAgent),
		LastIPAddress:  fingerprint.IPAddress,
		LastUserAgent:  fingerprint.UserAgent,
		CreatedAt:      time.Now(),
		LastSeenAt:     time.Now(),
		TrustLevel:     trustLevel,
	}

	err := dfm.repos.Account().CreateDevice(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	deviceInfo := &DeviceInfo{
		DeviceID:         device.DeviceID,
		Username:         username,
		DeviceName:       deviceName,
		DeviceType:       device.DeviceType,
		Fingerprint:      fingerprint,
		TrustLevel:       trustLevel,
		CreatedAt:        device.CreatedAt,
		LastSeenAt:       device.LastSeenAt,
		LastIPAddress:    device.LastIPAddress,
		SessionCount:     0,
		IsApproved:       !dfm.config.RequireDeviceApproval,
		RequiresApproval: dfm.config.RequireDeviceApproval,
	}

	dfm.logger.Info("new device registered",
		zap.String("username", username),
		zap.String("deviceID", deviceID),
		zap.String("deviceType", device.DeviceType),
		zap.String("trustLevel", trustLevel))

	return deviceInfo, nil
}

// UpdateDeviceFingerprint updates a device's fingerprint after validation
func (dfm *DeviceFingerprintManager) UpdateDeviceFingerprint(ctx context.Context, deviceID string, fingerprint *EnhancedDeviceFingerprint) error {
	device, err := dfm.repos.Account().GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}

	// Update device with new fingerprint data
	device.LastIPAddress = fingerprint.IPAddress
	device.LastUserAgent = fingerprint.UserAgent
	device.LastSeenAt = time.Now()

	// Promote trust level if device has been used long enough
	if device.TrustLevel == "untrusted" && time.Since(device.CreatedAt) > dfm.config.DeviceTrustThreshold {
		device.TrustLevel = "trusted"
		dfm.logger.Info("device trust level promoted",
			zap.String("deviceID", deviceID),
			zap.String("newTrustLevel", "trusted"))
	}

	return dfm.repos.Account().UpdateDevice(ctx, device)
}

// Helper methods

// generateBasicFingerprint creates a hash from basic device attributes
func (dfm *DeviceFingerprintManager) generateBasicFingerprint(fp *EnhancedDeviceFingerprint) string {
	combined := fmt.Sprintf("%s|%s|%s", fp.UserAgent, fp.IPAddress, fp.AcceptLang)
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// generateExtendedFingerprint creates a hash from all device attributes
func (dfm *DeviceFingerprintManager) generateExtendedFingerprint(fp *EnhancedDeviceFingerprint) string {
	combined := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		fp.UserAgent, fp.IPAddress, fp.AcceptLang, fp.AcceptEncoding,
		fp.Timezone, fp.ScreenRes, fp.ColorDepth, fp.Platform)
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// calculateFingerprintEntropy calculates the uniqueness score of a fingerprint
func (dfm *DeviceFingerprintManager) calculateFingerprintEntropy(fp *EnhancedDeviceFingerprint) float64 {
	// Simple entropy calculation based on available attributes
	entropy := 0.0
	
	if fp.UserAgent != "" {
		entropy += 2.0 // User agents are fairly unique
	}
	if fp.ScreenRes != "" {
		entropy += 1.5 // Screen resolution adds uniqueness
	}
	if fp.Timezone != "" {
		entropy += 1.0 // Timezone adds some uniqueness
	}
	if fp.AcceptLang != "" {
		entropy += 0.5 // Language preferences add a bit
	}
	if fp.ColorDepth != "" {
		entropy += 0.3 // Color depth adds minimal uniqueness
	}
	
	// Normalize to 0-1 scale
	maxEntropy := 5.3
	return entropy / maxEntropy
}

// calculateDeviceMatchConfidence calculates how well a device matches a fingerprint
func (dfm *DeviceFingerprintManager) calculateDeviceMatchConfidence(device *storage.Device, fingerprint *EnhancedDeviceFingerprint) float64 {
	confidence := 0.0

	// User agent matching (most important)
	if device.LastUserAgent == fingerprint.UserAgent {
		confidence += 0.4
	} else if dfm.isUserAgentSimilar(device.LastUserAgent, fingerprint.UserAgent) {
		confidence += 0.2
	}

	// IP address matching
	if device.LastIPAddress == fingerprint.IPAddress {
		confidence += 0.3
	} else if dfm.isIPInSameNetwork(device.LastIPAddress, fingerprint.IPAddress) {
		confidence += 0.1
	}

	// Additional factors would be compared here if stored

	return confidence
}

// detectDeviceChanges identifies what attributes have changed
func (dfm *DeviceFingerprintManager) detectDeviceChanges(device *storage.Device, fingerprint *EnhancedDeviceFingerprint) []string {
	var changes []string

	if device.LastUserAgent != fingerprint.UserAgent {
		changes = append(changes, "user_agent")
	}
	if device.LastIPAddress != fingerprint.IPAddress {
		changes = append(changes, "ip_address")
	}

	return changes
}

// calculateDeviceRiskScore calculates the risk score for a device validation
func (dfm *DeviceFingerprintManager) calculateDeviceRiskScore(device *storage.Device, fingerprint *EnhancedDeviceFingerprint, changes []string) float64 {
	risk := 0.0

	// Risk factors
	if len(changes) > 0 {
		risk += float64(len(changes)) * 0.1
	}

	// High-risk user agent changes
	if dfm.containsChange(changes, "user_agent") && dfm.isHighRiskUserAgentChange(device.LastUserAgent, fingerprint.UserAgent) {
		risk += 0.3
	}

	// IP address changes from different countries/regions
	if dfm.containsChange(changes, "ip_address") {
		risk += 0.2
	}

	// Cap at 1.0
	if risk > 1.0 {
		risk = 1.0
	}

	return risk
}

// calculateNewDeviceRiskScore calculates risk score for new devices
func (dfm *DeviceFingerprintManager) calculateNewDeviceRiskScore(fingerprint *EnhancedDeviceFingerprint) float64 {
	risk := 0.3 // Base risk for new devices

	// Check for suspicious user agents
	if dfm.isSuspiciousUserAgent(fingerprint.UserAgent) {
		risk += 0.3
	}

	// Check for VPN/proxy indicators
	if dfm.isLikelyVPN(fingerprint.IPAddress) {
		risk += 0.2
	}

	// Cap at 1.0
	if risk > 1.0 {
		risk = 1.0
	}

	return risk
}

// Utility methods

func (dfm *DeviceFingerprintManager) detectIPVersion(ipAddress string) string {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return "unknown"
	}
	if ip.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
}

func (dfm *DeviceFingerprintManager) extractPlatformFromUserAgent(userAgent string) string {
	// Simple platform detection
	userAgentLower := strings.ToLower(userAgent)
	switch {
	case strings.Contains(userAgentLower, "windows"):
		return "Windows"
	case strings.Contains(userAgentLower, "macintosh") || strings.Contains(userAgentLower, "mac os"):
		return "macOS"
	case strings.Contains(userAgentLower, "linux"):
		return "Linux"
	case strings.Contains(userAgentLower, "android"):
		return "Android"
	case strings.Contains(userAgentLower, "iphone") || strings.Contains(userAgentLower, "ipad"):
		return "iOS"
	default:
		return "Unknown"
	}
}

func (dfm *DeviceFingerprintManager) detectDeviceType(userAgent string) string {
	userAgentLower := strings.ToLower(userAgent)
	switch {
	case strings.Contains(userAgentLower, "mobile") || strings.Contains(userAgentLower, "android") || strings.Contains(userAgentLower, "iphone"):
		return "mobile"
	case strings.Contains(userAgentLower, "tablet") || strings.Contains(userAgentLower, "ipad"):
		return "tablet"
	default:
		return "desktop"
	}
}

func (dfm *DeviceFingerprintManager) generateDeviceID(fingerprint *EnhancedDeviceFingerprint) string {
	// Generate device ID from fingerprint
	combined := fmt.Sprintf("%s_%d", fingerprint.ExtendedFingerprint, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])[:16] // Use first 16 chars
}

func (dfm *DeviceFingerprintManager) isUserAgentSimilar(ua1, ua2 string) bool {
	// Simple similarity check - could be enhanced with more sophisticated algorithms
	if len(ua1) == 0 || len(ua2) == 0 {
		return false
	}
	
	// Extract browser and major version
	browser1 := dfm.extractBrowserFromUA(ua1)
	browser2 := dfm.extractBrowserFromUA(ua2)
	
	return browser1 == browser2
}

func (dfm *DeviceFingerprintManager) extractBrowserFromUA(userAgent string) string {
	// Simple browser extraction
	switch {
	case strings.Contains(userAgent, "Chrome/"):
		return "Chrome"
	case strings.Contains(userAgent, "Firefox/"):
		return "Firefox"
	case strings.Contains(userAgent, "Safari/") && !strings.Contains(userAgent, "Chrome/"):
		return "Safari"
	case strings.Contains(userAgent, "Edge/"):
		return "Edge"
	default:
		return "Unknown"
	}
}

func (dfm *DeviceFingerprintManager) isIPInSameNetwork(ip1, ip2 string) bool {
	// Check if IPs are in same /24 network for IPv4
	parsedIP1 := net.ParseIP(ip1)
	parsedIP2 := net.ParseIP(ip2)
	
	if parsedIP1 == nil || parsedIP2 == nil {
		return false
	}
	
	if parsedIP1.To4() != nil && parsedIP2.To4() != nil {
		// IPv4 - check /24 subnet
		return parsedIP1.To4()[0] == parsedIP2.To4()[0] &&
			   parsedIP1.To4()[1] == parsedIP2.To4()[1] &&
			   parsedIP1.To4()[2] == parsedIP2.To4()[2]
	}
	
	return false
}

func (dfm *DeviceFingerprintManager) containsChange(changes []string, change string) bool {
	for _, c := range changes {
		if c == change {
			return true
		}
	}
	return false
}

func (dfm *DeviceFingerprintManager) isHighRiskUserAgentChange(oldUA, newUA string) bool {
	// Check for suspicious user agent changes
	oldBrowser := dfm.extractBrowserFromUA(oldUA)
	newBrowser := dfm.extractBrowserFromUA(newUA)
	
	// Different browsers is suspicious
	return oldBrowser != newBrowser
}

func (dfm *DeviceFingerprintManager) isSuspiciousUserAgent(userAgent string) bool {
	suspiciousPatterns := []string{
		"bot", "crawler", "spider", "scraper",
		"curl", "wget", "python", "postman",
	}
	
	userAgentLower := strings.ToLower(userAgent)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(userAgentLower, pattern) {
			return true
		}
	}
	
	return false
}

func (dfm *DeviceFingerprintManager) isLikelyVPN(_ string) bool {
	// Basic VPN detection - in production, use a proper VPN detection service
	// This is a placeholder implementation
	return false
}

func (dfm *DeviceFingerprintManager) enforceDeviceLimits(ctx context.Context, username string) error {
	devices, err := dfm.repos.Account().GetUserDevices(ctx, username)
	if err != nil {
		return err
	}
	
	if len(devices) >= dfm.config.MaxDevicesPerUser {
		return fmt.Errorf("maximum number of devices (%d) exceeded", dfm.config.MaxDevicesPerUser)
	}
	
	return nil
}