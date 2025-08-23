//go:build example

package auth

import (
	"context"
	"log"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Example showing how to wire existing security infrastructure for enhanced device fingerprinting

// SetupEnhancedDeviceFingerprinting demonstrates how to initialize the enhanced device fingerprinting manager
func SetupEnhancedDeviceFingerprinting(
	repos StorageProvider,
	logger *zap.Logger,
	aiService *ai.AIService,
	db core.DB,
) *DeviceFingerprintManager {
	// Configure enhanced security settings
	config := &DeviceFingerprintConfig{
		EnableFingerprinting:  true,
		StrictFingerprinting:  false,
		FingerprintTTL:        90 * 24 * time.Hour,
		TrustNewDevices:       false,
		RequireDeviceApproval: false,
		MaxDevicesPerUser:     20,
		DeviceTrustThreshold:  7 * 24 * time.Hour,

		// Enhanced analysis features
		EnableBehaviorialAnalysis: true,
		EnableAdvancedRiskScoring: true,
		EnableMLAnomalyDetection:  true,
		EnableNetworkAnalysis:     true,
		VPNDetectionThreshold:     0.7,
		BehavioralRiskThreshold:   0.6,
		AnomalyScoreThreshold:     0.75,
	}

	// Create session security manager for integration
	secManager := NewSessionSecurityManager(logger, DefaultAdvancedSessionSecurityConfig())

	// Create enhanced device fingerprint manager
	return NewEnhancedDeviceFingerprintManager(
		repos,
		logger,
		config,
		aiService,
		db,
		secManager,
	)
}

// ExampleEnhancedDeviceValidation shows how to use the enhanced device validation
func ExampleEnhancedDeviceValidation(
	ctx context.Context,
	dfm *DeviceFingerprintManager,
	username string,
	userAgent string,
	ipAddress string,
	acceptLang string,
	acceptEncoding string,
	additionalData map[string]string,
) (*DeviceValidationResult, error) {
	// Generate comprehensive device fingerprint with AI analysis
	fingerprint := dfm.GenerateEnhancedFingerprintWithContext(
		ctx,
		userAgent,
		ipAddress,
		acceptLang,
		acceptEncoding,
		additionalData,
	)

	// Log the enhanced analysis results
	log.Printf("Device Analysis Results:")
	log.Printf("  VPN Probability: %.2f", fingerprint.VPNProbability)
	log.Printf("  Bot Probability: %.2f", fingerprint.BotProbability)
	log.Printf("  Anomaly Score: %.2f", fingerprint.AnomalyScore)
	log.Printf("  Behavioral Risk: %.2f", fingerprint.BehavioralRisk)
	log.Printf("  Network Risk: %.2f", fingerprint.NetworkRisk)
	log.Printf("  Threat Intel Score: %.2f", fingerprint.ThreatIntelScore)

	// Validate device with comprehensive security analysis
	result, err := dfm.ValidateDevice(ctx, username, fingerprint)
	if err != nil {
		return nil, err
	}

	// Log validation results
	log.Printf("Device Validation Results:")
	log.Printf("  Known Device: %v", result.IsKnownDevice)
	log.Printf("  Risk Score: %.2f", result.RiskScore)
	log.Printf("  Requires Challenge: %v", result.RequiresChallenge)
	log.Printf("  Requires Approval: %v", result.RequiresApproval)
	log.Printf("  Risk Factors: %d", len(result.RiskFactors))

	// Log risk factors and mitigation suggestions
	if len(result.RiskFactors) > 0 {
		log.Printf("  Risk Factors:")
		for _, factor := range result.RiskFactors {
			log.Printf("    - %s (%s): %s", factor.Type, factor.Severity, factor.Description)
		}
	}

	if len(result.MitigationSuggestions) > 0 {
		log.Printf("  Mitigation Suggestions:")
		for _, suggestion := range result.MitigationSuggestions {
			log.Printf("    - %s", suggestion)
		}
	}

	// Log security analysis details
	if result.SecurityAnalysis != nil {
		log.Printf("  Security Analysis:")
		log.Printf("    VPN Detected: %v", result.SecurityAnalysis.VPNDetected)
		log.Printf("    Proxy Detected: %v", result.SecurityAnalysis.ProxyDetected)
		log.Printf("    Bot Detected: %v", result.SecurityAnalysis.BotDetected)
		log.Printf("    Malware Risk: %.2f", result.SecurityAnalysis.MalwareRisk)
	}

	return result, nil
}

// ExampleDynamORMIntegration shows how the enhanced device fingerprinting integrates with DynamORM
func ExampleDynamORMIntegration() {
	// Initialize DynamORM client (as done in lambda handlers)
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), "us-east-1")
	if err != nil {
		log.Fatal("Failed to initialize DynamORM client:", err)
	}

	// This would typically be injected from your dependency injection system
	var repos StorageProvider   // Replace with actual StorageProvider implementation
	var aiService *ai.AIService // Replace with actual AI service instance
	logger := zap.L()

	// Setup enhanced device fingerprinting with all security infrastructure wired
	dfm := SetupEnhancedDeviceFingerprinting(repos, logger, aiService, db)

	// Example usage in authentication flow
	ctx := context.Background()
	username := "user@example.com"
	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	ipAddress := "192.168.1.100"
	acceptLang := "en-US,en;q=0.9"
	acceptEncoding := "gzip, deflate, br"
	additionalData := map[string]string{
		"timezone":          "America/New_York",
		"screen_resolution": "1920x1080",
		"color_depth":       "24",
		"platform":          "Win32",
	}

	result, err := ExampleEnhancedDeviceValidation(
		ctx, dfm, username, userAgent, ipAddress,
		acceptLang, acceptEncoding, additionalData,
	)

	if err != nil {
		log.Printf("Device validation failed: %v", err)
		return
	}

	// Make authentication decisions based on comprehensive analysis
	if result.RequiresApproval {
		log.Println("Device requires manual approval - blocking access")
		// Implement approval workflow
	} else if result.RequiresChallenge {
		log.Println("Device requires additional challenges - requesting 2FA/CAPTCHA")
		// Implement challenge workflow
	} else {
		log.Println("Device validation passed - allowing access")
		// Continue with normal authentication flow
	}
}

// Key Features Demonstrated:
//
// 1. **AI-Powered Analysis**: Integrates with existing AI service for:
//    - Advanced VPN/proxy detection beyond basic IP checks
//    - ML-based anomaly detection using behavioral patterns
//    - Spam analysis patterns applied to device characteristics
//
// 2. **Comprehensive Risk Assessment**: Multiple risk factors analyzed:
//    - Network-based risks (VPN, proxy, malicious IPs)
//    - Behavioral anomalies (timing, interaction patterns)
//    - Threat intelligence integration
//    - Device fingerprint consistency
//
// 3. **Enhanced Security Integration**: Wires existing security infrastructure:
//    - SessionSecurityManager for security policy enforcement
//    - DynamORM for efficient device storage and retrieval
//    - AI service for advanced pattern recognition
//
// 4. **Production-Ready Features**:
//    - Detailed risk factor breakdown with evidence
//    - Actionable mitigation suggestions
//    - Comprehensive logging and monitoring
//    - Backward compatibility with existing code
//
// 5. **Privacy-Preserving Design**:
//    - No storage of sensitive personal information
//    - Risk assessment without privacy violations
//    - Configurable privacy vs security trade-offs
