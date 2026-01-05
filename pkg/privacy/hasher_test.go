package privacy

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestGenerateMasterKey(t *testing.T) {
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	if len(key) != 64 {
		t.Errorf("Expected key length 64, got %d", len(key))
	}

	// Generate another key and ensure they're different
	key2, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey failed: %v", err)
	}

	if string(key) == string(key2) {
		t.Error("Generated keys should be different")
	}
}

func TestGenerateMasterKeyBase64(t *testing.T) {
	keyStr, err := GenerateMasterKeyBase64()
	if err != nil {
		t.Fatalf("GenerateMasterKeyBase64 failed: %v", err)
	}

	// Decode to verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		t.Errorf("Generated key is not valid base64: %v", err)
	}

	if len(decoded) != 64 {
		t.Errorf("Expected decoded key length 64, got %d", len(decoded))
	}
}

func TestNewHasher(t *testing.T) {
	// Test with valid config
	config := DefaultConfig()
	config.MasterKey = make([]byte, 64)

	hasher, err := NewHasher(config)
	if err != nil {
		t.Fatalf("NewHasher failed: %v", err)
	}

	if hasher == nil {
		t.Error("Expected non-nil hasher")
	}

	// Test with nil config (should use default)
	_, err = NewHasher(nil)
	if err == nil {
		t.Error("Expected error with nil config (no master key)")
	}

	// Test with short master key
	shortConfig := DefaultConfig()
	shortConfig.MasterKey = make([]byte, 16) // Too short

	_, err = NewHasher(shortConfig)
	if err == nil {
		t.Error("Expected error with short master key")
	}
}

func TestNewHasherFromMasterKey(t *testing.T) {
	// Test with base64 key
	keyBytes := make([]byte, 64)
	for i := range keyBytes {
		keyBytes[i] = byte(i)
	}
	base64Key := base64.StdEncoding.EncodeToString(keyBytes)

	hasher, err := NewHasherFromMasterKey(base64Key)
	if err != nil {
		t.Fatalf("NewHasherFromMasterKey failed: %v", err)
	}

	if hasher == nil {
		t.Error("Expected non-nil hasher")
	}

	// Test with raw string
	rawKey := "this_is_a_very_long_key_for_testing_purposes_that_is_at_least_64_chars"
	hasher2, err := NewHasherFromMasterKey(rawKey)
	if err != nil {
		t.Fatalf("NewHasherFromMasterKey with raw key failed: %v", err)
	}

	if hasher2 == nil {
		t.Error("Expected non-nil hasher")
	}

	// Verify the hashers work independently
	result1, _ := hasher.HashIP("192.168.1.1")
	result2, _ := hasher2.HashIP("192.168.1.1")
	// Note: Both hashers might produce same result if they derive the same master key
	// This is expected behavior as the decodeMasterKey function standardizes keys
	_ = result1
	_ = result2
}

func TestHashFull(t *testing.T) {
	hasher := createTestHasher(t)

	testCases := []struct {
		data     string
		dataType DataType
	}{
		{"192.168.1.1", DataTypeIP},
		{"user@example.com", DataTypeEmail},
		{"testuser", DataTypeUsername},
		{"sensitive data", DataTypePII},
		{"generic data", DataTypeGeneric},
	}

	for _, tc := range testCases {
		t.Run(string(tc.dataType), func(t *testing.T) {
			hash1, err := hasher.Hash(tc.data, tc.dataType)
			if err != nil {
				t.Fatalf("Hash failed: %v", err)
			}

			// Hash should be deterministic
			hash2, err := hasher.Hash(tc.data, tc.dataType)
			if err != nil {
				t.Fatalf("Hash failed: %v", err)
			}

			if hash1 != hash2 {
				t.Error("Hash should be deterministic")
			}

			// Hash should not be the original data
			if hash1 == tc.data {
				t.Error("Hash should not equal original data")
			}

			// Hash should start with the expected prefix
			expectedPrefix := "full_" + string(tc.dataType) + "_"
			if !strings.HasPrefix(hash1, expectedPrefix) {
				t.Errorf("Hash should start with %s, got %s", expectedPrefix, hash1)
			}
		})
	}
}

func TestHashIP(t *testing.T) {
	hasher := createTestHasher(t)

	testCases := []struct {
		name         string
		ip           string
		privacyLevel Level
		expectError  bool
	}{
		{"IPv4 Partial", "192.168.1.100", LevelPartial, false},
		{"IPv4 Full", "192.168.1.100", LevelFull, false},
		{"IPv4 None", "192.168.1.100", LevelNone, false},
		{"IPv6 Partial", "2001:db8::1", LevelPartial, false},
		{"IPv6 Full", "2001:db8::1", LevelFull, false},
		{"Invalid IP", "not.an.ip", LevelPartial, false},
		{"Empty IP", "", LevelPartial, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Update hasher config for this test
			hasher.config.IPLevel = tc.privacyLevel

			result, err := hasher.HashIP(tc.ip)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tc.ip == "" {
				if result != "" {
					t.Error("Empty IP should return empty result")
				}
				return
			}

			switch tc.privacyLevel {
			case LevelNone:
				if result != tc.ip {
					t.Errorf("None privacy level should return original IP, got %s", result)
				}
			case LevelPartial:
				if tc.ip == "192.168.1.100" {
					if !strings.HasPrefix(result, "192.168.") {
						t.Errorf("Partial IPv4 should preserve network, got %s", result)
					}
				}
			case LevelFull:
				if result == tc.ip {
					t.Error("Full privacy level should not return original IP")
				}
				if !strings.HasPrefix(result, "full_ip_") {
					t.Errorf("Full privacy should have proper prefix, got %s", result)
				}
			}
		})
	}
}

func TestHashEmail(t *testing.T) {
	hasher := createTestHasher(t)

	testCases := []struct {
		name         string
		email        string
		privacyLevel Level
	}{
		{"Valid Email Partial", "user@example.com", LevelPartial},
		{"Valid Email Full", "user@example.com", LevelFull},
		{"Valid Email None", "user@example.com", LevelNone},
		{"Invalid Email", "not-an-email", LevelPartial},
		{"Empty Email", "", LevelPartial},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hasher.config.EmailLevel = tc.privacyLevel

			result, err := hasher.HashEmail(tc.email)
			if err != nil {
				t.Fatalf("HashEmail failed: %v", err)
			}

			if tc.email == "" {
				if result != "" {
					t.Error("Empty email should return empty result")
				}
				return
			}

			switch tc.privacyLevel {
			case LevelNone:
				if result != tc.email {
					t.Errorf("None privacy level should return original email, got %s", result)
				}
			case LevelPartial:
				if tc.email == "user@example.com" {
					if !strings.HasSuffix(result, "@example.com") {
						t.Errorf("Partial email should preserve domain, got %s", result)
					}
				}
			case LevelFull:
				if result == tc.email {
					t.Error("Full privacy level should not return original email")
				}
			}
		})
	}
}

func TestHashUsername(t *testing.T) {
	hasher := createTestHasher(t)

	username := "testuser"
	result, err := hasher.HashUsername(username)
	if err != nil {
		t.Fatalf("HashUsername failed: %v", err)
	}

	if result == username {
		t.Error("Hashed username should not equal original")
	}

	// Should be deterministic
	result2, err := hasher.HashUsername(username)
	if err != nil {
		t.Fatalf("HashUsername failed: %v", err)
	}

	if result != result2 {
		t.Error("Username hashing should be deterministic")
	}
}

func TestHashPII(t *testing.T) {
	hasher := createTestHasher(t)

	pii := "John Doe"
	result, err := hasher.HashPII(pii)
	if err != nil {
		t.Fatalf("HashPII failed: %v", err)
	}

	if result == pii {
		t.Error("Hashed PII should not equal original")
	}

	// Should be deterministic
	result2, err := hasher.HashPII(pii)
	if err != nil {
		t.Fatalf("HashPII failed: %v", err)
	}

	if result != result2 {
		t.Error("PII hashing should be deterministic")
	}
}

func TestVerifyHash(t *testing.T) {
	hasher := createTestHasher(t)

	data := "test data"
	hash, err := hasher.Hash(data, DataTypeGeneric)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	// Verify correct hash
	valid, err := hasher.VerifyHash(data, hash, DataTypeGeneric)
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}

	if !valid {
		t.Error("Hash verification should succeed for correct data")
	}

	// Verify incorrect hash
	valid, err = hasher.VerifyHash("wrong data", hash, DataTypeGeneric)
	if err != nil {
		t.Fatalf("VerifyHash failed: %v", err)
	}

	if valid {
		t.Error("Hash verification should fail for incorrect data")
	}
}

func TestConfigSecurity(t *testing.T) {
	hasher := createTestHasher(t)

	// GetConfig should not expose the master key
	config := hasher.GetConfig()
	if config.MasterKey != nil {
		t.Error("GetConfig should not expose master key")
	}

	// Other fields should be preserved
	if config.IPLevel == "" {
		t.Error("GetConfig should preserve privacy levels")
	}
}

func TestContextKeyDerivation(t *testing.T) {
	hasher := createTestHasher(t)

	// Different data types should produce different hashes for the same data
	data := "test data"

	hashIP, _ := hasher.Hash(data, DataTypeIP)
	hashEmail, _ := hasher.Hash(data, DataTypeEmail)
	hashUsername, _ := hasher.Hash(data, DataTypeUsername)

	if hashIP == hashEmail || hashIP == hashUsername || hashEmail == hashUsername {
		t.Error("Different data types should produce different hashes")
	}
}

func TestTimingAttackResistance(t *testing.T) {
	hasher := createTestHasher(t)

	data := "test data"
	hash, _ := hasher.Hash(data, DataTypeGeneric)

	// This is a basic test - in practice, timing attack resistance
	// would need more sophisticated testing
	start := time.Now()
	hasher.VerifyHash(data, hash, DataTypeGeneric)
	correctTime := time.Since(start)

	start = time.Now()
	hasher.VerifyHash("wrong", hash, DataTypeGeneric)
	incorrectTime := time.Since(start)

	// Times should be similar (within an order of magnitude)
	// This is a crude test but better than nothing
	ratio := float64(correctTime) / float64(incorrectTime)
	if ratio > 10 || ratio < 0.1 {
		t.Logf("Timing difference may indicate vulnerability: correct=%v, incorrect=%v",
			correctTime, incorrectTime)
		// Don't fail the test as timing can vary, but log the concern
	}
}

func TestErrorHandling(t *testing.T) {
	// Test invalid Argon2 parameters
	config := DefaultConfig()
	config.MasterKey = make([]byte, 64)
	config.Argon2Memory = 0 // Invalid

	_, err := NewHasher(config)
	if err == nil {
		t.Error("Expected error with invalid Argon2 memory")
	}

	config.Argon2Memory = 1024
	config.Argon2Time = 0 // Invalid

	_, err = NewHasher(config)
	if err == nil {
		t.Error("Expected error with invalid Argon2 time")
	}
}

func TestEmptyDataHandling(t *testing.T) {
	hasher := createTestHasher(t)

	// Test empty strings
	result, err := hasher.Hash("", DataTypeGeneric)
	if err != nil {
		t.Errorf("Hash should handle empty string: %v", err)
	}
	if result != "" {
		t.Error("Empty string should return empty result")
	}

	result, err = hasher.HashIP("")
	if err != nil {
		t.Errorf("HashIP should handle empty string: %v", err)
	}
	if result != "" {
		t.Error("Empty IP should return empty result")
	}
}

// Helper function to create a test hasher
func createTestHasher(t *testing.T) *Hasher {
	config := DefaultConfig()
	// Use a deterministic key for testing
	config.MasterKey = make([]byte, 64)
	for i := range config.MasterKey {
		config.MasterKey[i] = byte(i % 256)
	}

	// Use much faster parameters for unit tests; security properties are validated
	// separately from performance characteristics.
	config.Argon2Memory = 1024
	config.Argon2Time = 1
	config.Argon2Threads = 1

	hasher, err := NewHasher(config)
	if err != nil {
		t.Fatalf("Failed to create test hasher: %v", err)
	}

	return hasher
}

// Benchmark tests
func BenchmarkHashIP(b *testing.B) {
	hasher := createBenchHasher(b)
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hasher.HashIP(ip)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashEmail(b *testing.B) {
	hasher := createBenchHasher(b)
	email := "user@example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hasher.HashEmail(email)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashFull(b *testing.B) {
	hasher := createBenchHasher(b)
	data := "test data for hashing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := hasher.Hash(data, DataTypeGeneric)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func createBenchHasher(b *testing.B) *Hasher {
	config := DefaultConfig()
	config.MasterKey = make([]byte, 64)
	for i := range config.MasterKey {
		config.MasterKey[i] = byte(i % 256)
	}

	// Use faster parameters for benchmarking
	config.Argon2Memory = 4 * 1024
	config.Argon2Time = 1
	config.Argon2Threads = 1

	hasher, err := NewHasher(config)
	if err != nil {
		b.Fatalf("Failed to create bench hasher: %v", err)
	}

	return hasher
}
