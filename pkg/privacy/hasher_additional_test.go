package privacy

import (
	"strings"
	"testing"
)

func TestHasher_HashUsername_PrivacyLevels(t *testing.T) {
	hasher := createFastTestHasher(t)

	t.Run("none returns original", func(t *testing.T) {
		hasher.config.UsernameLevel = LevelNone

		got, err := hasher.HashUsername("testuser")
		if err != nil {
			t.Fatalf("HashUsername() error = %v", err)
		}
		if got != "testuser" {
			t.Fatalf("HashUsername() = %q, want original", got)
		}
	})

	t.Run("partial short username falls back to full hash", func(t *testing.T) {
		hasher.config.UsernameLevel = LevelPartial

		got, err := hasher.HashUsername("ab")
		if err != nil {
			t.Fatalf("HashUsername() error = %v", err)
		}
		if got == "ab" {
			t.Fatalf("HashUsername() = %q, want hashed value", got)
		}
		if !strings.HasPrefix(got, "full_username_") {
			t.Fatalf("HashUsername() = %q, want full hash prefix", got)
		}
	})

	t.Run("partial preserves first and last character", func(t *testing.T) {
		hasher.config.UsernameLevel = LevelPartial

		got, err := hasher.HashUsername("abcdef")
		if err != nil {
			t.Fatalf("HashUsername() error = %v", err)
		}
		if len(got) != len("abcdef") {
			t.Fatalf("HashUsername() length = %d, want %d", len(got), len("abcdef"))
		}
		if got[0] != 'a' || got[len(got)-1] != 'f' {
			t.Fatalf("HashUsername() = %q, want to preserve first/last chars", got)
		}
	})

	t.Run("partial repeats hash when middle is longer than hash", func(t *testing.T) {
		hasher.config.UsernameLevel = LevelPartial

		username := "a" + strings.Repeat("b", 200) + "z"
		got, err := hasher.HashUsername(username)
		if err != nil {
			t.Fatalf("HashUsername() error = %v", err)
		}
		if len(got) != len(username) {
			t.Fatalf("HashUsername() length = %d, want %d", len(got), len(username))
		}
		if got[0] != 'a' || got[len(got)-1] != 'z' {
			t.Fatalf("HashUsername() = %q, want to preserve first/last chars", got)
		}
	})
}

func TestHasher_HashPII_PrivacyLevelsAndDetection(t *testing.T) {
	hasher := createFastTestHasher(t)

	t.Run("none returns original", func(t *testing.T) {
		hasher.config.PIILevel = LevelNone

		got, err := hasher.HashPII("John Doe")
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if got != "John Doe" {
			t.Fatalf("HashPII() = %q, want original", got)
		}
	})

	t.Run("partial phone number preserves formatting", func(t *testing.T) {
		hasher.config.PIILevel = LevelPartial

		in := "(555) 123-4567"
		got, err := hasher.HashPII(in)
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if got == in {
			t.Fatalf("HashPII() = %q, want hashed value", got)
		}
		if !strings.Contains(got, "(") || !strings.Contains(got, ")") || !strings.Contains(got, "-") {
			t.Fatalf("HashPII() = %q, want to preserve phone formatting characters", got)
		}
		if len(got) != len(in) {
			t.Fatalf("HashPII() length = %d, want %d", len(got), len(in))
		}
	})

	t.Run("partial SSN-like data uses full hash", func(t *testing.T) {
		hasher.config.PIILevel = LevelPartial

		got, err := hasher.HashPII("123-45-6789")
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if !strings.HasPrefix(got, "full_pii_") {
			t.Fatalf("HashPII() = %q, want full hash prefix", got)
		}
	})

	t.Run("partial other data includes length prefix", func(t *testing.T) {
		hasher.config.PIILevel = LevelPartial

		in := "John Doe"
		got, err := hasher.HashPII(in)
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if !strings.HasPrefix(got, "pii_len") {
			t.Fatalf("HashPII() = %q, want length-prefixed hash", got)
		}
	})

	t.Run("phone number replacement cycles hash when more digits than hash length", func(t *testing.T) {
		hasher.config.PIILevel = LevelPartial

		in := strings.Repeat("1", 120)
		got, err := hasher.HashPII(in)
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if len(got) != len(in) {
			t.Fatalf("HashPII() length = %d, want %d", len(got), len(in))
		}
	})
}

func TestHasher_PrivateDetectors(t *testing.T) {
	hasher := createFastTestHasher(t)

	if hasher.looksLikePhoneNumber("abc1234567") {
		t.Fatalf("looksLikePhoneNumber() unexpectedly true for non-phone data")
	}
	if !hasher.looksLikePhoneNumber("555-123-4567") {
		t.Fatalf("looksLikePhoneNumber() unexpectedly false for phone-like data")
	}

	if !hasher.looksLikeSSN("123 45 6789") {
		t.Fatalf("looksLikeSSN() unexpectedly false for SSN-like data")
	}
	if hasher.looksLikeSSN("123-45-678x") {
		t.Fatalf("looksLikeSSN() unexpectedly true for non-digit data")
	}
	if hasher.looksLikeSSN("1234") {
		t.Fatalf("looksLikeSSN() unexpectedly true for short data")
	}
}

func createFastTestHasher(t *testing.T) *Hasher {
	t.Helper()

	cfg := DefaultConfig()
	cfg.MasterKey = make([]byte, 64)
	for i := range cfg.MasterKey {
		cfg.MasterKey[i] = byte(i % 256)
	}

	cfg.Argon2Memory = 1024
	cfg.Argon2Time = 1
	cfg.Argon2Threads = 1

	hasher, err := NewHasher(cfg)
	if err != nil {
		t.Fatalf("NewHasher() error = %v", err)
	}
	return hasher
}
