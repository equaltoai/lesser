package privacy

import (
	"encoding/base64"
	"strings"
	"testing"
)

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

func TestConfigLoader_EmptyPrefix(t *testing.T) {
	// Test with empty prefix (should default to "PRIVACY")
	loader := NewConfigLoader("")

	if loader.envPrefix != "PRIVACY" {
		t.Errorf("Empty prefix should default to PRIVACY, got %s", loader.envPrefix)
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
