package privacy

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestConfigLoader_LoadFromEnvironment(t *testing.T) {
	// Clean up environment
	cleanEnv(t)

	// Set up test environment
	key, _ := GenerateMasterKeyBase64()
	os.Setenv("PRIVACY_MASTER_KEY", key)
	os.Setenv("PRIVACY_IP_PRIVACY_LEVEL", "full")
	os.Setenv("PRIVACY_EMAIL_PRIVACY_LEVEL", "none")
	os.Setenv("PRIVACY_ARGON2_MEMORY", "8192")
	os.Setenv("PRIVACY_ARGON2_TIME", "2")

	defer cleanEnv(t)

	loader := NewConfigLoader("PRIVACY")
	config, err := loader.LoadFromEnvironment()
	if err != nil {
		t.Fatalf("LoadFromEnvironment failed: %v", err)
	}

	if config.IPLevel != LevelFull {
		t.Errorf("Expected IP privacy level full, got %v", config.IPLevel)
	}

	if config.EmailLevel != LevelNone {
		t.Errorf("Expected email privacy level none, got %v", config.EmailLevel)
	}

	if config.Argon2Memory != 8192 {
		t.Errorf("Expected Argon2 memory 8192, got %d", config.Argon2Memory)
	}

	if config.Argon2Time != 2 {
		t.Errorf("Expected Argon2 time 2, got %d", config.Argon2Time)
	}
}

func TestConfigLoader_LoadFromEnvironment_MissingMasterKey(t *testing.T) {
	cleanEnv(t)
	defer cleanEnv(t)

	loader := NewConfigLoader("PRIVACY")
	_, err := loader.LoadFromEnvironment()
	if err == nil {
		t.Error("Expected error when master key is missing")
	}

	if !strings.Contains(err.Error(), "PRIVACY_MASTER_KEY") {
		t.Errorf("Error should mention master key variable: %v", err)
	}
}

func TestConfigLoader_LoadHasherFromEnvironment(t *testing.T) {
	cleanEnv(t)

	key, _ := GenerateMasterKeyBase64()
	os.Setenv("PRIVACY_MASTER_KEY", key)
	defer cleanEnv(t)

	loader := NewConfigLoader("PRIVACY")
	hasher, err := loader.LoadHasherFromEnvironment()
	if err != nil {
		t.Fatalf("LoadHasherFromEnvironment failed: %v", err)
	}

	if hasher == nil {
		t.Error("Expected non-nil hasher")
	}

	// Test hasher functionality
	result, err := hasher.HashIP("192.168.1.1")
	if err != nil {
		t.Errorf("Hasher should work: %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestDecodeMasterKey(t *testing.T) {
	// Test base64 key
	keyBytes := make([]byte, 64)
	for i := range keyBytes {
		keyBytes[i] = byte(i)
	}
	base64Key := base64.StdEncoding.EncodeToString(keyBytes)

	decoded, err := decodeMasterKey(base64Key)
	if err != nil {
		t.Fatalf("Failed to decode base64 key: %v", err)
	}

	if len(decoded) != 64 {
		t.Errorf("Expected 64 bytes, got %d", len(decoded))
	}

	// Test raw string key (long enough)
	rawKey := "this_is_a_very_long_raw_key_that_is_at_least_32_bytes_long_for_testing"
	decoded, err = decodeMasterKey(rawKey)
	if err != nil {
		t.Fatalf("Failed to decode raw key: %v", err)
	}

	if len(decoded) != len(rawKey) {
		t.Errorf("Expected %d bytes, got %d", len(rawKey), len(decoded))
	}

	// Test short raw string key (should fail)
	shortKey := "short"
	_, err = decodeMasterKey(shortKey)
	if err == nil {
		t.Error("Expected error for short key")
	}
}

func TestParsePrivacyLevel(t *testing.T) {
	testCases := []struct {
		envValue string
		expected Level
	}{
		{"none", LevelNone},
		{"NONE", LevelNone},
		{"partial", LevelPartial},
		{"PARTIAL", LevelPartial},
		{"full", LevelFull},
		{"FULL", LevelFull},
		{"invalid", LevelFull}, // default fallback
		{"", LevelFull},        // default fallback
	}

	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			result := parsePrivacyLevel(tc.envValue, LevelFull)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}


func TestConfigLoader_GetEnvironmentDocumentation(t *testing.T) {
	loader := NewConfigLoader("PRIVACY")
	docs := loader.GetEnvironmentDocumentation()

	// Check that documentation contains expected elements
	expectedElements := []string{
		"PRIVACY_MASTER_KEY",
		"PRIVACY_IP_PRIVACY_LEVEL",
		"PRIVACY_EMAIL_PRIVACY_LEVEL",
		"PRIVACY_ARGON2_MEMORY",
		"none, partial, full",
		"EXAMPLES:",
	}

	for _, element := range expectedElements {
		if !strings.Contains(docs, element) {
			t.Errorf("Documentation should contain %s", element)
		}
	}
}

func TestConfigLoader_ValidateEnvironmentVariables(t *testing.T) {
	loader := NewConfigLoader("PRIVACY")

	// Test without master key
	cleanEnv(t)
	err := loader.ValidateEnvironmentVariables()
	if err == nil {
		t.Error("Expected error when master key is missing")
	}

	// Test with master key
	key, _ := GenerateMasterKeyBase64()
	os.Setenv("PRIVACY_MASTER_KEY", key)
	defer cleanEnv(t)

	err = loader.ValidateEnvironmentVariables()
	if err != nil {
		t.Errorf("Validation should pass when master key is present: %v", err)
	}
}

func TestConfigLoader_SetupFromEnvironmentOrGenerate(t *testing.T) {
	loader := NewConfigLoader("PRIVACY")

	// Test with missing master key (should provide generation instructions)
	cleanEnv(t)
	_, err := loader.SetupFromEnvironmentOrGenerate()
	if err == nil {
		t.Error("Expected error with generation instructions")
	}

	if !strings.Contains(err.Error(), "export PRIVACY_MASTER_KEY") {
		t.Errorf("Error should contain export instruction: %v", err)
	}

	// Test with valid master key
	key, _ := GenerateMasterKeyBase64()
	os.Setenv("PRIVACY_MASTER_KEY", key)
	defer cleanEnv(t)

	hasher, err := loader.SetupFromEnvironmentOrGenerate()
	if err != nil {
		t.Errorf("Should succeed with valid master key: %v", err)
	}

	if hasher == nil {
		t.Error("Expected non-nil hasher")
	}
}

func TestPresetConfigurations_Development(t *testing.T) {
	pc := &PresetConfigurations{}

	config, err := pc.GetDevelopmentConfig()
	if err != nil {
		t.Fatalf("GetDevelopmentConfig failed: %v", err)
	}

	// Development config should have lower security for speed
	if config.Argon2Memory >= 64*1024 {
		t.Error("Development config should use less memory for speed")
	}

	if config.Argon2Time > 1 {
		t.Error("Development config should use fewer iterations for speed")
	}

	// Should have partial privacy levels for debugging
	if config.UsernameLevel != LevelPartial {
		t.Error("Development config should use partial privacy for usernames")
	}
}

func TestPresetConfigurations_Production(t *testing.T) {
	pc := &PresetConfigurations{}

	// Test without master key (should fail)
	_, err := pc.GetProductionConfig(nil)
	if err == nil {
		t.Error("Expected error without master key")
	}

	// Test with valid master key
	masterKey := make([]byte, 64)
	config, err := pc.GetProductionConfig(masterKey)
	if err != nil {
		t.Fatalf("GetProductionConfig failed: %v", err)
	}

	// Production config should have higher security
	if config.Argon2Memory < 64*1024 {
		t.Error("Production config should use more memory for security")
	}

	if config.Argon2Time < 3 {
		t.Error("Production config should use more iterations for security")
	}

	// Should have appropriate privacy levels
	if config.UsernameLevel != LevelFull {
		t.Error("Production config should use full privacy for usernames")
	}
}

func TestPresetConfigurations_Compliance(t *testing.T) {
	pc := &PresetConfigurations{}

	masterKey := make([]byte, 64)
	config, err := pc.GetComplianceConfig(masterKey)
	if err != nil {
		t.Fatalf("GetComplianceConfig failed: %v", err)
	}

	// Compliance config should have maximum security
	if config.Argon2Memory < 128*1024 {
		t.Error("Compliance config should use maximum memory")
	}

	if config.Argon2Time < 4 {
		t.Error("Compliance config should use maximum iterations")
	}

	// Should have full privacy for everything
	privacyLevels := []Level{
		config.IPLevel,
		config.EmailLevel,
		config.UsernameLevel,
		config.PIILevel,
		config.GenericLevel,
	}

	for _, level := range privacyLevels {
		if level != LevelFull {
			t.Error("Compliance config should use full privacy for all data types")
		}
	}
}

func TestConfigLoader_CustomPrefix(t *testing.T) {
	// Test with custom prefix
	loader := NewConfigLoader("CUSTOM")

	key, _ := GenerateMasterKeyBase64()
	os.Setenv("CUSTOM_MASTER_KEY", key)
	defer os.Unsetenv("CUSTOM_MASTER_KEY")

	hasher, err := loader.LoadHasherFromEnvironment()
	if err != nil {
		t.Fatalf("Custom prefix should work: %v", err)
	}

	if hasher == nil {
		t.Error("Expected non-nil hasher")
	}
}

func TestConfigLoader_EmptyPrefix(t *testing.T) {
	// Test with empty prefix (should default to "PRIVACY")
	loader := NewConfigLoader("")

	if loader.envPrefix != "PRIVACY" {
		t.Errorf("Empty prefix should default to PRIVACY, got %s", loader.envPrefix)
	}
}

// Helper function to clean environment variables
func cleanEnv(t *testing.T) {
	envVars := []string{
		"PRIVACY_MASTER_KEY",
		"PRIVACY_IP_PRIVACY_LEVEL",
		"PRIVACY_EMAIL_PRIVACY_LEVEL",
		"PRIVACY_USERNAME_PRIVACY_LEVEL",
		"PRIVACY_PII_PRIVACY_LEVEL",
		"PRIVACY_GENERIC_PRIVACY_LEVEL",
		"PRIVACY_KEY_ROTATION_ENABLED",
		"PRIVACY_KEY_ROTATION_INTERVAL",
		"PRIVACY_ARGON2_MEMORY",
		"PRIVACY_ARGON2_TIME",
		"PRIVACY_ARGON2_THREADS",
		"PRIVACY_ARGON2_KEY_LENGTH",
		"TEST_LEVEL",
		"TEST_BOOL",
		"TEST_DURATION",
		"TEST_UINT32",
		"CUSTOM_MASTER_KEY",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}
