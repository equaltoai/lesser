package privacy

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

// ConfigLoader handles loading privacy configuration from environment variables
type ConfigLoader struct {
	envPrefix string
}

// NewConfigLoader creates a new configuration loader with optional environment prefix
func NewConfigLoader(envPrefix string) *ConfigLoader {
	if err := common.ValidateRequiredParam("envPrefix", envPrefix); err != nil {
		envPrefix = "PRIVACY"
	}
	return &ConfigLoader{
		envPrefix: envPrefix,
	}
}

// LoadFromConfig loads privacy hashing configuration from centralized config
// This is the modern approach - use this instead of LoadFromEnvironment
func LoadFromConfig() (*HashingConfig, error) {
	cfg := config.Get()
	privacyConfig := DefaultConfig()

	// Load master key (required)
	masterKeyStr := cfg.PrivacyMasterKey
	if err := common.ValidateRequiredParam("privacy_master_key", masterKeyStr); err != nil {
		return nil, fmt.Errorf("privacy master key is not configured")
	}

	masterKey, err := decodeMasterKey(masterKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode privacy master key: %w", err)
	}
	privacyConfig.MasterKey = masterKey

	// Load privacy levels from centralized config
	privacyConfig.IPLevel = parsePrivacyLevel(cfg.IPLevel, privacyConfig.IPLevel)
	privacyConfig.EmailLevel = parsePrivacyLevel(cfg.EmailLevel, privacyConfig.EmailLevel)
	privacyConfig.UsernameLevel = parsePrivacyLevel(cfg.UsernameLevel, privacyConfig.UsernameLevel)
	privacyConfig.PIILevel = parsePrivacyLevel(cfg.PIILevel, privacyConfig.PIILevel)
	privacyConfig.GenericLevel = parsePrivacyLevel(cfg.GenericLevel, privacyConfig.GenericLevel)

	// Load key rotation settings from centralized config
	privacyConfig.KeyRotationEnabled = cfg.KeyRotationEnabled
	privacyConfig.KeyRotationInterval = cfg.KeyRotationInterval

	// Load Argon2 parameters from centralized config
	privacyConfig.Argon2Memory = cfg.Argon2Memory
	privacyConfig.Argon2Time = cfg.Argon2Time
	privacyConfig.Argon2Threads = cfg.Argon2Threads
	privacyConfig.Argon2KeyLen = cfg.Argon2KeyLen

	return privacyConfig, nil
}

// decodeMasterKey decodes a master key from string (base64, hex, or raw)
func decodeMasterKey(keyStr string) ([]byte, error) {
	if err := common.ValidateRequiredParam("keyStr", keyStr); err != nil {
		return nil, fmt.Errorf("master key string is empty")
	}

	// Try base64 first
	if decoded, err := base64.StdEncoding.DecodeString(keyStr); err == nil && len(decoded) >= 32 {
		return decoded, nil
	}

	// Try hex
	if decoded, err := hex.DecodeString(keyStr); err == nil && len(decoded) >= 32 {
		return decoded, nil
	}

	// Use raw string if it's long enough
	keyBytes := []byte(keyStr)
	if len(keyBytes) >= 32 {
		return keyBytes, nil
	}

	return nil, fmt.Errorf("master key must be at least 32 bytes (got %d)", len(keyBytes))
}

// parsePrivacyLevel parses a privacy level string with fallback to default
func parsePrivacyLevel(levelStr string, defaultLevel Level) Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "none":
		return LevelNone
	case "partial":
		return LevelPartial
	case "full":
		return LevelFull
	default:
		return defaultLevel
	}
}

// LoadFromEnvironment loads privacy hashing configuration from environment variables
// Deprecated: Use LoadFromConfig() instead for centralized configuration
func (cl *ConfigLoader) LoadFromEnvironment() (*HashingConfig, error) {
	// Use centralized config instead of direct environment access
	return LoadFromConfig()
}

// LoadHasherFromEnvironment creates a Hasher from environment variables
func (cl *ConfigLoader) LoadHasherFromEnvironment() (*Hasher, error) {
	config, err := cl.LoadFromEnvironment()
	if err != nil {
		return nil, err
	}

	return NewHasher(config)
}

// getEnvKey returns the full environment variable key with prefix
func (cl *ConfigLoader) getEnvKey(key string) string {
	return fmt.Sprintf("%s_%s", cl.envPrefix, key)
}

// decodeMasterKey attempts to decode a master key from various formats
func (cl *ConfigLoader) decodeMasterKey(keyStr string) ([]byte, error) {
	// Try base64 first
	if decoded, err := base64.StdEncoding.DecodeString(keyStr); err == nil && len(decoded) >= 32 {
		return decoded, nil
	}

	// Try hex encoding
	if decoded, err := hex.DecodeString(keyStr); err == nil && len(decoded) >= 32 {
		return decoded, nil
	}

	// If it's a raw string, ensure it's long enough
	keyBytes := []byte(keyStr)
	if len(keyBytes) < 32 {
		return nil, fmt.Errorf("master key must be at least 32 bytes when provided as raw string, got %d", len(keyBytes))
	}

	// Pad or truncate to exactly 64 bytes for consistency
	masterKey := make([]byte, 64)
	if len(keyBytes) >= 64 {
		copy(masterKey, keyBytes[:64])
	} else {
		copy(masterKey, keyBytes)
		// Fill the rest with a derived value to ensure full entropy
		for i := len(keyBytes); i < 64; i++ {
			masterKey[i] = byte(i ^ len(keyBytes))
		}
	}

	return masterKey, nil
}

// Legacy functions below are deprecated - they now use centralized config

// loadLevel loads a privacy level from environment variable (deprecated - now uses centralized config)
func (cl *ConfigLoader) loadLevel(key string, defaultValue Level) Level {
	// All privacy level loading now happens through centralized config
	// This function is maintained for backward compatibility but delegates to LoadFromConfig
	cfg := config.Get()
	switch key {
	case "IP_PRIVACY_LEVEL":
		return parsePrivacyLevel(cfg.IPLevel, defaultValue)
	case "EMAIL_PRIVACY_LEVEL":
		return parsePrivacyLevel(cfg.EmailLevel, defaultValue)
	case "USERNAME_PRIVACY_LEVEL":
		return parsePrivacyLevel(cfg.UsernameLevel, defaultValue)
	case "PII_PRIVACY_LEVEL":
		return parsePrivacyLevel(cfg.PIILevel, defaultValue)
	case "GENERIC_PRIVACY_LEVEL":
		return parsePrivacyLevel(cfg.GenericLevel, defaultValue)
	default:
		return defaultValue
	}
}

// loadBool loads a boolean value from environment variable (deprecated - now uses centralized config)
func (cl *ConfigLoader) loadBool(key string, defaultValue bool) bool {
	cfg := config.Get()
	switch key {
	case "KEY_ROTATION_ENABLED":
		return cfg.KeyRotationEnabled
	default:
		return defaultValue
	}
}

// loadDuration loads a duration from environment variable (deprecated - now uses centralized config)
func (cl *ConfigLoader) loadDuration(key string, defaultValue time.Duration) time.Duration {
	cfg := config.Get()
	switch key {
	case "KEY_ROTATION_INTERVAL":
		return cfg.KeyRotationInterval
	default:
		return defaultValue
	}
}

// loadUint32 loads a uint32 value from environment variable (deprecated - now uses centralized config)
func (cl *ConfigLoader) loadUint32(key string, defaultValue uint32) uint32 {
	cfg := config.Get()
	switch key {
	case "ARGON2_MEMORY":
		return cfg.Argon2Memory
	case "ARGON2_TIME":
		return cfg.Argon2Time
	case "ARGON2_KEY_LENGTH":
		return cfg.Argon2KeyLen
	default:
		return defaultValue
	}
}

// loadUint8 loads a uint8 value from environment variable (deprecated - now uses centralized config)
func (cl *ConfigLoader) loadUint8(key string, defaultValue uint8) uint8 {
	cfg := config.Get()
	switch key {
	case "ARGON2_THREADS":
		return cfg.Argon2Threads
	default:
		return defaultValue
	}
}

// GetEnvironmentDocumentation returns documentation for all privacy environment variables
func (cl *ConfigLoader) GetEnvironmentDocumentation() string {
	prefix := cl.envPrefix
	
	return fmt.Sprintf(`Privacy Hashing Environment Variables (prefix: %s_):

REQUIRED:
  %s_MASTER_KEY                 Master key for HMAC (base64, hex, or raw string, min 32 bytes)

PRIVACY LEVELS (none, partial, full):
  %s_IP_PRIVACY_LEVEL          Privacy level for IP addresses (default: partial)
  %s_EMAIL_PRIVACY_LEVEL       Privacy level for email addresses (default: partial)  
  %s_USERNAME_PRIVACY_LEVEL    Privacy level for usernames (default: full)
  %s_PII_PRIVACY_LEVEL         Privacy level for PII data (default: full)
  %s_GENERIC_PRIVACY_LEVEL     Privacy level for generic data (default: full)

KEY ROTATION:
  %s_KEY_ROTATION_ENABLED      Enable key rotation (true/false, default: false)
  %s_KEY_ROTATION_INTERVAL     Key rotation interval (e.g., "24h", default: 24h)

ARGON2 PARAMETERS:
  %s_ARGON2_MEMORY             Memory usage in KB (default: 65536)
  %s_ARGON2_TIME               Number of iterations (default: 3)
  %s_ARGON2_THREADS            Number of threads (default: 4)
  %s_ARGON2_KEY_LENGTH         Length of derived key in bytes (default: 32)

EXAMPLES:
  %s_MASTER_KEY="AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyAhIiMkJSYnKCkqKywtLi8wMTIzNDU2Nzg5Ojs8PT4/QA=="
  %s_IP_PRIVACY_LEVEL="partial"
  %s_EMAIL_PRIVACY_LEVEL="partial"
  %s_USERNAME_PRIVACY_LEVEL="full"
  %s_ARGON2_MEMORY="65536"
  %s_ARGON2_TIME="3"
`,
		prefix,
		prefix,
		prefix, prefix, prefix, prefix, prefix,
		prefix, prefix,
		prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix)
}

// ValidateEnvironmentVariables validates that required environment variables are set
func (cl *ConfigLoader) ValidateEnvironmentVariables() error {
	cfg := config.Get()
	if err := common.ValidateRequiredParam("privacy_master_key", cfg.PrivacyMasterKey); err != nil {
		return fmt.Errorf("required privacy master key is not configured in centralized config")
	}
	
	return nil
}

// SetupFromEnvironmentOrGenerate sets up privacy hashing from environment or generates new keys
func (cl *ConfigLoader) SetupFromEnvironmentOrGenerate() (*Hasher, error) {
	// Try to load from centralized config first
	hasher, err := cl.LoadHasherFromEnvironment()
	if err == nil {
		return hasher, nil
	}
	
	// If master key is missing, generate a new one and provide instructions
	cfg := config.Get()
	if err := common.ValidateRequiredParam("privacy_master_key", cfg.PrivacyMasterKey); err != nil {
		masterKey, genErr := GenerateMasterKeyBase64()
		if genErr != nil {
			return nil, fmt.Errorf("failed to generate master key: %w", genErr)
		}
		
		return nil, fmt.Errorf(`master key not found in configuration. Generated a new one:

Set this environment variable:
export PRIVACY_MASTER_KEY="%s"

Then restart your application.

%s`, masterKey, cl.GetEnvironmentDocumentation())
	}
	
	return nil, err
}

// PresetConfigurations provides common preset configurations
type PresetConfigurations struct{}

// GetDevelopmentConfig returns a configuration suitable for development
func (pc *PresetConfigurations) GetDevelopmentConfig() (*HashingConfig, error) {
	// Generate a deterministic key for development (NOT for production)
	devKey := make([]byte, 64)
	for i := range devKey {
		devKey[i] = byte(i % 256)
	}
	
	config := DefaultConfig()
	config.MasterKey = devKey
	config.IPLevel = LevelPartial
	config.EmailLevel = LevelPartial
	config.UsernameLevel = LevelPartial // Less privacy for development
	config.PIILevel = LevelPartial
	config.GenericLevel = LevelPartial
	
	// Faster Argon2 for development
	config.Argon2Memory = 4 * 1024  // 4 MB
	config.Argon2Time = 1           // 1 iteration
	config.Argon2Threads = 1        // 1 thread
	
	return config, nil
}

// GetProductionConfig returns a configuration suitable for production with maximum security
func (pc *PresetConfigurations) GetProductionConfig(masterKey []byte) (*HashingConfig, error) {
	if len(masterKey) < 32 {
		return nil, fmt.Errorf("production configuration requires a master key of at least 32 bytes")
	}
	
	config := DefaultConfig()
	config.MasterKey = masterKey
	config.IPLevel = LevelPartial      // Preserve network info for security analysis
	config.EmailLevel = LevelPartial   // Preserve domain for analytics
	config.UsernameLevel = LevelFull   // Full privacy for usernames
	config.PIILevel = LevelFull        // Full privacy for PII
	config.GenericLevel = LevelFull    // Full privacy by default
	
	// Strong Argon2 parameters for production
	config.Argon2Memory = 128 * 1024 // 128 MB
	config.Argon2Time = 4            // 4 iterations
	config.Argon2Threads = 8         // 8 threads
	config.Argon2KeyLen = 64         // 64 bytes output
	
	return config, nil
}

// GetComplianceConfig returns a configuration suitable for strict compliance requirements
func (pc *PresetConfigurations) GetComplianceConfig(masterKey []byte) (*HashingConfig, error) {
	if len(masterKey) < 32 {
		return nil, fmt.Errorf("compliance configuration requires a master key of at least 32 bytes")
	}
	
	config := DefaultConfig()
	config.MasterKey = masterKey
	
	// Maximum privacy for compliance
	config.IPLevel = LevelFull
	config.EmailLevel = LevelFull
	config.UsernameLevel = LevelFull
	config.PIILevel = LevelFull
	config.GenericLevel = LevelFull
	
	// Very strong Argon2 parameters for compliance
	config.Argon2Memory = 256 * 1024 // 256 MB
	config.Argon2Time = 5            // 5 iterations
	config.Argon2Threads = 8         // 8 threads
	config.Argon2KeyLen = 64         // 64 bytes output
	
	return config, nil
}