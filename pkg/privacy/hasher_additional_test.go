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

func TestHasher_PartialHashesUseDigestMaterial(t *testing.T) {
	hasher := createFastTestHasher(t)

	t.Run("ip host hash does not include full-hash prefix", func(t *testing.T) {
		hasher.config.IPLevel = LevelPartial

		got, err := hasher.HashIP("192.168.1.100")
		if err != nil {
			t.Fatalf("HashIP() error = %v", err)
		}
		if strings.Contains(got, "full") {
			t.Fatalf("HashIP() = %q, partial host hash must derive from digest bytes", got)
		}
	})

	t.Run("email local hash does not include full-hash prefix", func(t *testing.T) {
		hasher.config.EmailLevel = LevelPartial

		got, err := hasher.HashEmail("alice@example.com")
		if err != nil {
			t.Fatalf("HashEmail() error = %v", err)
		}
		local, _, ok := strings.Cut(got, "@")
		if !ok {
			t.Fatalf("HashEmail() = %q, want local@domain", got)
		}
		if strings.Contains(local, "full") || strings.Contains(local, "email") {
			t.Fatalf("HashEmail() = %q, local hash must derive from digest bytes", got)
		}
	})

	t.Run("username middle hash does not include full-hash prefix", func(t *testing.T) {
		hasher.config.UsernameLevel = LevelPartial

		got, err := hasher.HashUsername("abcdef")
		if err != nil {
			t.Fatalf("HashUsername() error = %v", err)
		}
		if strings.Contains(got, "full") || strings.Contains(got, "username") {
			t.Fatalf("HashUsername() = %q, middle hash must derive from digest bytes", got)
		}
	})

	t.Run("pii partial hashes do not include full-hash prefix", func(t *testing.T) {
		hasher.config.PIILevel = LevelPartial

		got, err := hasher.HashPII("John Doe")
		if err != nil {
			t.Fatalf("HashPII() error = %v", err)
		}
		if strings.Contains(got, "full_pii") {
			t.Fatalf("HashPII() = %q, partial hash must derive from digest bytes", got)
		}

		phone, err := hasher.HashPII("(555) 123-4567")
		if err != nil {
			t.Fatalf("HashPII(phone) error = %v", err)
		}
		if strings.Contains(phone, "full") || strings.Contains(phone, "pii") {
			t.Fatalf("HashPII(phone) = %q, digit replacements must derive from digest bytes", phone)
		}
	})
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
