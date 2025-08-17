package privacy

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
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

// LoadFromEnvironment loads privacy hashing configuration from environment variables
func (cl *ConfigLoader) LoadFromEnvironment() (*HashingConfig, error) {
	config := DefaultConfig()

	// Load master key (required)
	masterKeyEnv := cl.getEnvKey("MASTER_KEY")
	masterKeyStr := os.Getenv(masterKeyEnv)
	if err := common.ValidateRequiredParam(masterKeyEnv, masterKeyStr); err != nil {
		return nil, fmt.Errorf("required environment variable %s is not set", masterKeyEnv)
	}

	masterKey, err := cl.decodeMasterKey(masterKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key from %s: %w", masterKeyEnv, err)
	}
	config.MasterKey = masterKey

	// Load privacy levels
	config.IPLevel = cl.loadLevel("IP_PRIVACY_LEVEL", config.IPLevel)
	config.EmailLevel = cl.loadLevel("EMAIL_PRIVACY_LEVEL", config.EmailLevel)
	config.UsernameLevel = cl.loadLevel("USERNAME_PRIVACY_LEVEL", config.UsernameLevel)
	config.PIILevel = cl.loadLevel("PII_PRIVACY_LEVEL", config.PIILevel)
	config.GenericLevel = cl.loadLevel("GENERIC_PRIVACY_LEVEL", config.GenericLevel)

	// Load key rotation settings
	config.KeyRotationEnabled = cl.loadBool("KEY_ROTATION_ENABLED", config.KeyRotationEnabled)
	if rotationInterval := cl.loadDuration("KEY_ROTATION_INTERVAL", config.KeyRotationInterval); rotationInterval > 0 {
		config.KeyRotationInterval = rotationInterval
	}

	// Load Argon2 parameters
	config.Argon2Memory = cl.loadUint32("ARGON2_MEMORY", config.Argon2Memory)
	config.Argon2Time = cl.loadUint32("ARGON2_TIME", config.Argon2Time)
	config.Argon2Threads = cl.loadUint8("ARGON2_THREADS", config.Argon2Threads)
	config.Argon2KeyLen = cl.loadUint32("ARGON2_KEY_LENGTH", config.Argon2KeyLen)

	return config, nil
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

// loadLevel loads a privacy level from environment variable
func (cl *ConfigLoader) loadLevel(key string, defaultValue Level) Level {
	envKey := cl.getEnvKey(key)
	value := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
	
	switch value {
	case "none":
		return LevelNone
	case "partial":
		return LevelPartial
	case "full":
		return LevelFull
	default:
		return defaultValue
	}
}

// loadBool loads a boolean value from environment variable
func (cl *ConfigLoader) loadBool(key string, defaultValue bool) bool {
	envKey := cl.getEnvKey(key)
	value := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
	
	switch value {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultValue
	}
}

// loadDuration loads a duration from environment variable
func (cl *ConfigLoader) loadDuration(key string, defaultValue time.Duration) time.Duration {
	envKey := cl.getEnvKey(key)
	value := strings.TrimSpace(os.Getenv(envKey))
	
	if err := common.ValidateRequiredParam("value", value); err != nil {
		return defaultValue
	}
	
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	
	return duration
}

// loadUint32 loads a uint32 value from environment variable
func (cl *ConfigLoader) loadUint32(key string, defaultValue uint32) uint32 {
	envKey := cl.getEnvKey(key)
	value := strings.TrimSpace(os.Getenv(envKey))
	
	if err := common.ValidateRequiredParam("value", value); err != nil {
		return defaultValue
	}
	
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return defaultValue
	}
	
	return uint32(parsed)
}

// loadUint8 loads a uint8 value from environment variable
func (cl *ConfigLoader) loadUint8(key string, defaultValue uint8) uint8 {
	envKey := cl.getEnvKey(key)
	value := strings.TrimSpace(os.Getenv(envKey))
	
	if err := common.ValidateRequiredParam("value", value); err != nil {
		return defaultValue
	}
	
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return defaultValue
	}
	
	return uint8(parsed)
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
	masterKeyEnv := cl.getEnvKey("MASTER_KEY")
	if err := common.ValidateRequiredParam(masterKeyEnv, os.Getenv(masterKeyEnv)); err != nil {
		return fmt.Errorf("required environment variable %s is not set", masterKeyEnv)
	}
	
	return nil
}

// SetupFromEnvironmentOrGenerate sets up privacy hashing from environment or generates new keys
func (cl *ConfigLoader) SetupFromEnvironmentOrGenerate() (*Hasher, error) {
	// Try to load from environment first
	hasher, err := cl.LoadHasherFromEnvironment()
	if err == nil {
		return hasher, nil
	}
	
	// If master key is missing, generate a new one and provide instructions
	masterKeyEnv := cl.getEnvKey("MASTER_KEY")
	if err := common.ValidateRequiredParam(masterKeyEnv, os.Getenv(masterKeyEnv)); err != nil {
		masterKey, genErr := GenerateMasterKeyBase64()
		if genErr != nil {
			return nil, fmt.Errorf("failed to generate master key: %w", genErr)
		}
		
		return nil, fmt.Errorf(`master key not found in environment. Generated a new one:

Set this environment variable:
export %s="%s"

Then restart your application.

%s`, masterKeyEnv, masterKey, cl.GetEnvironmentDocumentation())
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