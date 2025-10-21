package features

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appconfig"
)

// FeatureFlags manages feature toggles for the application
type FeatureFlags struct {
	flags       map[string]bool
	client      *appconfig.Client
	stopRefresh chan struct{}
	environment string
	application string
	clientId    string
	mu          sync.RWMutex
}

// Feature flag keys
const (
	// Core features
	RateLimitingEnabled    = "rate_limiting_enabled"
	CircuitBreakerEnabled  = "circuit_breaker_enabled"
	EnhancedMonitoring     = "enhanced_monitoring"
	ServiceMeshIntegration = "service_mesh_integration"

	// Development features
	MockServicesEnabled = "mock_services_enabled"
	DebugLoggingEnabled = "debug_logging_enabled"
	DevDashboardEnabled = "dev_dashboard_enabled"

	// UI features
	NewDashboardUI = "new_dashboard_ui"

	// Security features
	AdvancedSecurityEnabled = "advanced_security_enabled"
	IPAllowlistEnabled      = "ip_allowlist_enabled"
)

// FeatureFlagConfig configures the feature flag system
type FeatureFlagConfig struct {
	Environment string
	Application string
	Region      string
	LocalOnly   bool // For testing without AWS
}

// NewFeatureFlags creates a new feature flag manager
func NewFeatureFlags(config FeatureFlagConfig) (*FeatureFlags, error) {
	ff := &FeatureFlags{
		flags:       make(map[string]bool),
		environment: config.Environment,
		application: config.Application,
		clientId:    generateClientId(),
		stopRefresh: make(chan struct{}),
	}

	// Load defaults first
	ff.loadDefaults()

	// If local only mode, don't connect to AWS
	if config.LocalOnly {
		ff.loadLocalOverrides()
		return ff, nil
	}

	// Initialize AWS client
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
	)
	if err != nil {
		// Continue with defaults if AWS config fails
		return ff, nil
	}

	ff.client = appconfig.NewFromConfig(cfg)

	// Load initial flags
	if err := ff.refresh(); err != nil {
		// Log error but continue with defaults
		log.Printf("Failed to load initial feature flags: %v", err)
	}

	// Start refresh goroutine if client is available
	if ff.client != nil {
		go ff.refreshLoop()
	}

	return ff, nil
}

// IsEnabled checks if a feature flag is enabled
func (ff *FeatureFlags) IsEnabled(flag string) bool {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	// Check environment variable override first
	envKey := "LIFT_FEATURE_" + flag
	if envVal := os.Getenv(envKey); envVal != "" {
		return envVal == "true" || envVal == "1"
	}

	enabled, exists := ff.flags[flag]
	if !exists {
		// Return safe default
		return ff.getDefault(flag)
	}

	return enabled
}

// SetFlag manually sets a flag value (useful for testing)
func (ff *FeatureFlags) SetFlag(flag string, enabled bool) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	ff.flags[flag] = enabled
}

// GetAllFlags returns all current flag values
func (ff *FeatureFlags) GetAllFlags() map[string]bool {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	result := make(map[string]bool)
	for k, v := range ff.flags {
		result[k] = v
	}
	return result
}

// refresh fetches the latest configuration from AWS AppConfig
func (ff *FeatureFlags) refresh() error {
	if ff.client == nil {
		return fmt.Errorf("no AWS client available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get configuration from AWS AppConfig
	// NOTE: GetConfiguration is deprecated in favor of StartConfigurationSession/GetLatestConfiguration
	// but the new APIs require AWS SDK v2 appconfig 1.15.0+.
	// Will update to new API in next major release to maintain compatibility
	resp, err := ff.client.GetConfiguration(ctx, &appconfig.GetConfigurationInput{ //nolint:staticcheck // Using deprecated API until SDK upgrade
		Application:   aws.String(ff.application),
		Environment:   aws.String(ff.environment),
		Configuration: aws.String("feature-flags"),
		ClientId:      aws.String(ff.clientId),
	})
	if err != nil {
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	// Parse configuration
	var flags map[string]bool
	if err := json.Unmarshal(resp.Content, &flags); err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	ff.mu.Lock()
	ff.flags = flags
	ff.mu.Unlock()

	return nil
}

// refreshLoop periodically refreshes feature flags
func (ff *FeatureFlags) refreshLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ff.refresh(); err != nil {
				log.Printf("Failed to refresh feature flags: %v", err)
			}
		case <-ff.stopRefresh:
			return
		}
	}
}

// Stop stops the refresh loop
func (ff *FeatureFlags) Stop() {
	close(ff.stopRefresh)
}

// getDefault returns the default value for a feature flag
func (ff *FeatureFlags) getDefault(flag string) bool {
	// Safe defaults for production
	defaults := map[string]bool{
		// Core features - enabled by default
		RateLimitingEnabled:   true,
		CircuitBreakerEnabled: true,
		EnhancedMonitoring:    true,

		// Optional features - disabled by default
		ServiceMeshIntegration: false,
		MockServicesEnabled:    false,
		DebugLoggingEnabled:    false,
		DevDashboardEnabled:    false,
		NewDashboardUI:         false,

		// Security features - enabled by default
		AdvancedSecurityEnabled: true,
		IPAllowlistEnabled:      false,
	}

	// Check if we're in development mode
	if isDevelopment() {
		// Different defaults for development
		defaults[MockServicesEnabled] = true
		defaults[DebugLoggingEnabled] = true
		defaults[DevDashboardEnabled] = true
	}

	if val, exists := defaults[flag]; exists {
		return val
	}

	// Unknown flags default to false
	return false
}

// loadDefaults loads the default flag values
func (ff *FeatureFlags) loadDefaults() {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	// Load all defaults
	for flag := range getAllKnownFlags() {
		ff.flags[flag] = ff.getDefault(flag)
	}
}

// loadLocalOverrides loads flags from local configuration or environment
func (ff *FeatureFlags) loadLocalOverrides() {
	// Check for local config file
	configFile := os.Getenv("LIFT_FEATURE_FLAGS_FILE")
	if configFile == "" {
		configFile = ".lift-features.json"
	}

	data, err := os.ReadFile(configFile) // #nosec G304 - configFile is either default or from controlled env var
	if err == nil {
		var overrides map[string]bool
		if err := json.Unmarshal(data, &overrides); err == nil {
			ff.mu.Lock()
			for k, v := range overrides {
				ff.flags[k] = v
			}
			ff.mu.Unlock()
		}
	}
}

// getAllKnownFlags returns all known feature flag keys
func getAllKnownFlags() map[string]bool {
	return map[string]bool{
		RateLimitingEnabled:     true,
		CircuitBreakerEnabled:   true,
		EnhancedMonitoring:      true,
		ServiceMeshIntegration:  true,
		MockServicesEnabled:     true,
		DebugLoggingEnabled:     true,
		DevDashboardEnabled:     true,
		NewDashboardUI:          true,
		AdvancedSecurityEnabled: true,
		IPAllowlistEnabled:      true,
	}
}

// isDevelopment checks if we're running in development mode
func isDevelopment() bool {
	env := os.Getenv("LIFT_ENV")
	return env == "development" || env == "dev" || env == ""
}

// generateClientId generates a unique client ID for AppConfig
func generateClientId() string {
	hostname, err := os.Hostname()
	if err != nil {
		// Fallback to a timestamp-based ID if hostname is unavailable
		hostname = "unknown-host"
	}
	return fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
}

// Global instance (optional, can be initialized in main)
var defaultFlags *FeatureFlags

// InitializeFeatureFlags initializes the global feature flags instance
func InitializeFeatureFlags(config FeatureFlagConfig) error {
	ff, err := NewFeatureFlags(config)
	if err != nil {
		return err
	}
	defaultFlags = ff
	return nil
}

// IsEnabled checks if a feature is enabled using the global instance
func IsEnabled(flag string) bool {
	if defaultFlags == nil {
		// Return safe default if not initialized
		ff := &FeatureFlags{flags: make(map[string]bool)}
		return ff.getDefault(flag)
	}
	return defaultFlags.IsEnabled(flag)
}
