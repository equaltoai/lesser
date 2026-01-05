package privacy

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
)

func TestLoadFromConfig_SuccessAndErrors(t *testing.T) {
	t.Run("success loads levels and decodes master key", func(t *testing.T) {
		rawKey := "raw_key_for_tests_that_is_long_enough_to_be_valid_32bytes"
		t.Setenv("PRIVACY_MASTER_KEY", rawKey)
		t.Setenv("IP_PRIVACY_LEVEL", "none")
		t.Setenv("EMAIL_PRIVACY_LEVEL", "partial")
		t.Setenv("USERNAME_PRIVACY_LEVEL", "full")
		t.Setenv("PII_PRIVACY_LEVEL", "partial")
		t.Setenv("GENERIC_PRIVACY_LEVEL", "full")
		t.Setenv("KEY_ROTATION_ENABLED", "true")
		t.Setenv("KEY_ROTATION_INTERVAL", "2h")
		t.Setenv("ARGON2_MEMORY", "2048")
		t.Setenv("ARGON2_TIME", "2")
		t.Setenv("ARGON2_THREADS", "2")
		t.Setenv("ARGON2_KEY_LENGTH", "32")
		config.ResetForTests()

		cfg, err := LoadFromConfig()
		if err != nil {
			t.Fatalf("LoadFromConfig() error = %v", err)
		}
		if cfg == nil {
			t.Fatalf("LoadFromConfig() returned nil config")
		}
		if len(cfg.MasterKey) < 32 {
			t.Fatalf("LoadFromConfig() master key length = %d, want >= 32", len(cfg.MasterKey))
		}
		if cfg.IPLevel != LevelNone || cfg.EmailLevel != LevelPartial || cfg.PIILevel != LevelPartial {
			t.Fatalf("LoadFromConfig() levels = IP:%s email:%s pii:%s", cfg.IPLevel, cfg.EmailLevel, cfg.PIILevel)
		}
	})

	t.Run("missing master key returns error", func(t *testing.T) {
		t.Setenv("PRIVACY_MASTER_KEY", "")
		config.ResetForTests()

		_, err := LoadFromConfig()
		if err == nil {
			t.Fatalf("LoadFromConfig() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "privacy master key is not configured") {
			t.Fatalf("LoadFromConfig() error = %v, want missing key error", err)
		}
	})

	t.Run("decode error is wrapped", func(t *testing.T) {
		t.Setenv("PRIVACY_MASTER_KEY", "short")
		config.ResetForTests()

		_, err := LoadFromConfig()
		if err == nil {
			t.Fatalf("LoadFromConfig() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to decode privacy master key") {
			t.Fatalf("LoadFromConfig() error = %v, want wrapped decode error", err)
		}
	})
}

func TestConfigLoader_EnvironmentIntegration(t *testing.T) {
	t.Run("load hasher succeeds when configured", func(t *testing.T) {
		rawKey := "raw_key_for_tests_that_is_long_enough_to_be_valid_32bytes"
		t.Setenv("PRIVACY_MASTER_KEY", rawKey)
		config.ResetForTests()

		loader := NewConfigLoader("PRIVACY")
		hasher, err := loader.LoadHasherFromEnvironment()
		if err != nil {
			t.Fatalf("LoadHasherFromEnvironment() error = %v", err)
		}
		if hasher == nil {
			t.Fatalf("LoadHasherFromEnvironment() returned nil hasher")
		}
	})

	t.Run("validate environment variables returns error when missing", func(t *testing.T) {
		t.Setenv("PRIVACY_MASTER_KEY", "")
		config.ResetForTests()

		loader := NewConfigLoader("PRIVACY")
		if err := loader.ValidateEnvironmentVariables(); err == nil {
			t.Fatalf("ValidateEnvironmentVariables() expected error, got nil")
		}
	})

	t.Run("setup generates instructions when missing", func(t *testing.T) {
		t.Setenv("PRIVACY_MASTER_KEY", "")
		config.ResetForTests()

		loader := NewConfigLoader("PRIVACY")
		hasher, err := loader.SetupFromEnvironmentOrGenerate()
		if hasher != nil {
			t.Fatalf("SetupFromEnvironmentOrGenerate() hasher = %v, want nil", hasher)
		}
		if err == nil {
			t.Fatalf("SetupFromEnvironmentOrGenerate() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Generated a new one:") || !strings.Contains(err.Error(), "export PRIVACY_MASTER_KEY=") {
			t.Fatalf("SetupFromEnvironmentOrGenerate() error = %v, want instructions", err)
		}
	})

	t.Run("setup returns original error when configured but invalid", func(t *testing.T) {
		t.Setenv("PRIVACY_MASTER_KEY", "short")
		config.ResetForTests()

		loader := NewConfigLoader("PRIVACY")
		hasher, err := loader.SetupFromEnvironmentOrGenerate()
		if hasher != nil {
			t.Fatalf("SetupFromEnvironmentOrGenerate() hasher = %v, want nil", hasher)
		}
		if err == nil {
			t.Fatalf("SetupFromEnvironmentOrGenerate() expected error, got nil")
		}
		if strings.Contains(err.Error(), "Generated a new one:") {
			t.Fatalf("SetupFromEnvironmentOrGenerate() error = %v, want original decode error", err)
		}
	})

	t.Run("load from config uses hex decode path when base64 fails", func(t *testing.T) {
		keyBytes := make([]byte, 33) // 66 hex chars -> invalid base64 length
		for i := range keyBytes {
			keyBytes[i] = byte(i + 1)
		}
		hexKey := hex.EncodeToString(keyBytes)

		t.Setenv("PRIVACY_MASTER_KEY", hexKey)
		config.ResetForTests()

		cfg, err := LoadFromConfig()
		if err != nil {
			t.Fatalf("LoadFromConfig() error = %v", err)
		}
		if len(cfg.MasterKey) != len(keyBytes) {
			t.Fatalf("LoadFromConfig() master key length = %d, want %d", len(cfg.MasterKey), len(keyBytes))
		}
	})
}
