package privacy

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// TestSecurityProperties verifies important security properties of the implementation
func TestSecurityProperties(t *testing.T) {
	hasher := createTestHasher(t)

	t.Run("NoReversibleInformation", func(t *testing.T) {
		testNoReversibleInformation(t, hasher)
	})

	t.Run("UniformDistribution", func(t *testing.T) {
		testUniformDistribution(t, hasher)
	})

	t.Run("DeterministicHashing", func(t *testing.T) {
		testDeterministicHashing(t, hasher)
	})

	t.Run("ContextSeparation", func(t *testing.T) {
		testContextSeparation(t, hasher)
	})

	t.Run("KeySensitivity", func(t *testing.T) {
		testKeySensitivity(t)
	})

	t.Run("OutputLength", func(t *testing.T) {
		testOutputLength(t, hasher)
	})
}

// testNoReversibleInformation verifies that hashed data doesn't contain reversible information
func testNoReversibleInformation(t *testing.T, hasher *Hasher) {
	sensitiveData := []string{
		"192.168.1.100",
		"user@example.com",
		"john_doe",
		"John Doe",
		"sensitive personal information",
	}

	for _, data := range sensitiveData {
		hash, err := hasher.Hash(data, DataTypePII)
		if err != nil {
			t.Fatalf("Hash failed: %v", err)
		}

		validateHashDoesNotContainOriginalData(t, hash, data)
		validateHashDoesNotContainDerivatives(t, hash, data)
	}
}

// validateHashDoesNotContainOriginalData checks that hash doesn't contain the original data
func validateHashDoesNotContainOriginalData(t *testing.T, hash, data string) {
	if strings.Contains(hash, data) {
		t.Errorf("Hash contains original data: %s -> %s", data, hash)
	}
}

// validateHashDoesNotContainDerivatives checks that hash doesn't contain obvious derivatives
func validateHashDoesNotContainDerivatives(t *testing.T, hash, data string) {
	dataLower := strings.ToLower(data)
	if strings.Contains(strings.ToLower(hash), dataLower) && len(dataLower) > 3 {
		t.Errorf("Hash contains derivative of original data: %s -> %s", data, hash)
	}
}

// testUniformDistribution tests that hashes have good distribution
func testUniformDistribution(t *testing.T, hasher *Hasher) {
	hashes := make(map[string]int)

	for i := 0; i < 1000; i++ {
		data := fmt.Sprintf("test_data_%d", i)
		hash, err := hasher.Hash(data, DataTypeGeneric)
		if err != nil {
			t.Fatalf("Hash failed: %v", err)
		}

		countHashPrefix(hashes, hash)
	}

	validateDistribution(t, hashes)
}

// countHashPrefix extracts and counts hash prefixes for distribution analysis
func countHashPrefix(hashes map[string]int, hash string) {
	hashWithoutPrefix := strings.TrimPrefix(hash, "full_generic_")
	if len(hashWithoutPrefix) >= 8 {
		prefix := hashWithoutPrefix[:8]
		hashes[prefix]++
	}
}

// validateDistribution checks that we don't have too many collisions in prefixes
func validateDistribution(t *testing.T, hashes map[string]int) {
	maxCollisions := 50 // Allow some collisions but not too many
	for prefix, count := range hashes {
		if count > maxCollisions {
			t.Errorf("Too many prefix collisions for %s: %d", prefix, count)
		}
	}
}

// testDeterministicHashing verifies that the same input always produces the same output
func testDeterministicHashing(t *testing.T, hasher *Hasher) {
	testData := "deterministic_test_data"

	hash1, err := hasher.Hash(testData, DataTypeGeneric)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		hash2, err := hasher.Hash(testData, DataTypeGeneric)
		if err != nil {
			t.Fatalf("Hash failed: %v", err)
		}

		if hash1 != hash2 {
			t.Error("Hashing is not deterministic")
			break
		}
	}
}

// testContextSeparation verifies that different contexts produce different hashes
func testContextSeparation(t *testing.T, hasher *Hasher) {
	testData := "context_test_data"

	hashes := generateHashesForAllContexts(t, hasher, testData)
	validateAllHashesAreDifferent(t, hashes)
}

// generateHashesForAllContexts creates hashes for all data types
func generateHashesForAllContexts(t *testing.T, hasher *Hasher, testData string) []string {
	hashIP, _ := hasher.Hash(testData, DataTypeIP)
	hashEmail, _ := hasher.Hash(testData, DataTypeEmail)
	hashUsername, _ := hasher.Hash(testData, DataTypeUsername)
	hashPII, _ := hasher.Hash(testData, DataTypePII)
	hashGeneric, _ := hasher.Hash(testData, DataTypeGeneric)

	return []string{hashIP, hashEmail, hashUsername, hashPII, hashGeneric}
}

// validateAllHashesAreDifferent checks that all hashes in the slice are unique
func validateAllHashesAreDifferent(t *testing.T, hashes []string) {
	for i := 0; i < len(hashes); i++ {
		for j := i + 1; j < len(hashes); j++ {
			if hashes[i] == hashes[j] {
				t.Errorf("Context separation failed: same hash for different contexts")
			}
		}
	}
}

// testKeySensitivity verifies that different keys produce different hashes
func testKeySensitivity(t *testing.T) {
	hasher1 := createTestHasher(t)
	hasher2 := createTestHasherWithDifferentKey(t)

	testData := "key_sensitivity_test"

	hash1, _ := hasher1.Hash(testData, DataTypeGeneric)
	hash2, _ := hasher2.Hash(testData, DataTypeGeneric)

	if hash1 == hash2 {
		t.Error("Different keys should produce different hashes")
	}
}

// createTestHasherWithDifferentKey creates a hasher with a different master key
func createTestHasherWithDifferentKey(t *testing.T) *Hasher {
	config2 := DefaultConfig()
	config2.MasterKey = make([]byte, 64)
	for i := range config2.MasterKey {
		config2.MasterKey[i] = byte((i + 1) % 256) // Different from first hasher
	}

	hasher2, err := NewHasher(config2)
	if err != nil {
		t.Fatalf("Failed to create second hasher: %v", err)
	}

	return hasher2
}

// testOutputLength verifies consistent output lengths
func testOutputLength(t *testing.T, hasher *Hasher) {
	testInputs := []string{
		"a",
		"short",
		"medium_length_input",
		"very_long_input_string_that_is_much_longer_than_others_to_test_consistency",
	}

	lengths := collectHashLengths(t, hasher, testInputs)
	validateHashLengthConsistency(t, lengths)
}

// collectHashLengths generates hashes for test inputs and collects their lengths
func collectHashLengths(t *testing.T, hasher *Hasher, testInputs []string) []int {
	var lengths []int
	for _, input := range testInputs {
		hash, err := hasher.Hash(input, DataTypeGeneric)
		if err != nil {
			t.Fatalf("Hash failed: %v", err)
		}

		lengths = append(lengths, len(hash))
	}
	return lengths
}

// validateHashLengthConsistency checks that hash lengths don't vary too much
func validateHashLengthConsistency(t *testing.T, lengths []int) {
	minLen, maxLen := lengths[0], lengths[0]
	for _, length := range lengths {
		if length < minLen {
			minLen = length
		}
		if length > maxLen {
			maxLen = length
		}
	}

	if maxLen-minLen > 10 {
		t.Errorf("Hash lengths vary too much: min=%d, max=%d", minLen, maxLen)
	}
}

// TestIPPartialHashing tests the security of partial IP hashing
func TestIPPartialHashing(t *testing.T) {
	hasher := createTestHasher(t)
	hasher.config.IPLevel = LevelPartial

	t.Run("IPv4PartialPreservation", func(t *testing.T) {
		testIPs := []string{
			"192.168.1.100",
			"10.0.0.1",
			"172.16.5.4",
		}

		for _, ip := range testIPs {
			hash, err := hasher.HashIP(ip)
			if err != nil {
				t.Fatalf("HashIP failed: %v", err)
			}

			parts := strings.Split(ip, ".")
			if len(parts) != 4 {
				continue
			}

			expectedPrefix := fmt.Sprintf("%s.%s.", parts[0], parts[1])
			if !strings.HasPrefix(hash, expectedPrefix) {
				t.Errorf("IPv4 partial hashing should preserve network: %s -> %s", ip, hash)
			}

			// Verify that host portion is not preserved
			hostPortion := fmt.Sprintf("%s.%s", parts[2], parts[3])
			if strings.Contains(hash, hostPortion) {
				t.Errorf("IPv4 partial hashing should not preserve host portion: %s", hash)
			}
		}
	})

	t.Run("IPv6PartialPreservation", func(t *testing.T) {
		testIPs := []string{
			"2001:db8::1",
			"fe80::1234:5678:abcd:ef01",
		}

		for _, ip := range testIPs {
			hash, err := hasher.HashIP(ip)
			if err != nil {
				t.Fatalf("HashIP failed: %v", err)
			}

			// Should contain some part of the original but not all
			if hash == ip {
				t.Errorf("IPv6 partial hashing should not return original IP: %s", ip)
			}

			// Should not be a full hash either
			if strings.HasPrefix(hash, "full_ip_") {
				t.Errorf("IPv6 partial hashing should not use full hash format: %s", hash)
			}
		}
	})
}

// TestEmailPartialHashing tests the security of partial email hashing
func TestEmailPartialHashing(t *testing.T) {
	hasher := createTestHasher(t)
	hasher.config.EmailLevel = LevelPartial

	testEmails := []string{
		"user@example.com",
		"john.doe@company.org",
		"test123@domain.net",
	}

	for _, email := range testEmails {
		hash, err := hasher.HashEmail(email)
		if err != nil {
			t.Fatalf("HashEmail failed: %v", err)
		}

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			continue
		}

		domain := parts[1]
		localPart := parts[0]

		// Should preserve domain
		if !strings.HasSuffix(hash, "@"+domain) {
			t.Errorf("Email partial hashing should preserve domain: %s -> %s", email, hash)
		}

		// Should not preserve local part
		if strings.Contains(hash, localPart) {
			t.Errorf("Email partial hashing should not preserve local part: %s", hash)
		}
	}
}

// TestCryptographicStrength tests the cryptographic properties
func TestCryptographicStrength(t *testing.T) {
	hasher := createTestHasher(t)

	t.Run("AvalancheEffect", func(t *testing.T) {
		// Small changes in input should cause large changes in output
		baseData := "avalanche_test_data"
		baseHash, _ := hasher.Hash(baseData, DataTypeGeneric)

		// Change one character
		modifiedData := "avalanche_test_datb" // Changed 'a' to 'b'
		modifiedHash, _ := hasher.Hash(modifiedData, DataTypeGeneric)

		// Count different characters (after removing prefixes)
		baseClean := strings.TrimPrefix(baseHash, "full_generic_")
		modifiedClean := strings.TrimPrefix(modifiedHash, "full_generic_")

		minLen := min(len(baseClean), len(modifiedClean))
		if minLen == 0 {
			t.Fatal("Hashes too short for avalanche test")
		}

		differentChars := 0
		for i := 0; i < minLen; i++ {
			if baseClean[i] != modifiedClean[i] {
				differentChars++
			}
		}

		// At least 30% of characters should be different for good avalanche effect
		diffPercentage := float64(differentChars) / float64(minLen)
		if diffPercentage < 0.3 {
			t.Errorf("Poor avalanche effect: only %.2f%% of characters changed", diffPercentage*100)
		}
	})

	t.Run("RandomnessTest", func(t *testing.T) {
		// Generate many hashes and check for basic randomness properties
		hashes := make([]string, 100)

		for i := 0; i < 100; i++ {
			data := fmt.Sprintf("randomness_test_%d", i)
			hash, err := hasher.Hash(data, DataTypeGeneric)
			if err != nil {
				t.Fatalf("Hash failed: %v", err)
			}
			hashes[i] = hash
		}

		// Check for duplicates
		hashSet := make(map[string]bool)
		for _, hash := range hashes {
			if hashSet[hash] {
				t.Error("Found duplicate hash in randomness test")
				break
			}
			hashSet[hash] = true
		}

		// Check character distribution in hex portion
		charCounts := make(map[rune]int)
		totalChars := 0

		for _, hash := range hashes {
			// Extract hex portion (remove prefix)
			hexPart := strings.TrimPrefix(hash, "full_generic_")
			for _, char := range hexPart {
				if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
					charCounts[char]++
					totalChars++
				}
			}
		}

		// Check that character distribution is reasonably uniform
		expectedFreq := float64(totalChars) / 16.0 // 16 hex characters
		for char, count := range charCounts {
			freq := float64(count)
			deviation := (freq - expectedFreq) / expectedFreq
			if deviation > 0.5 || deviation < -0.5 {
				t.Logf("Character '%c' has unusual frequency: %.2f (expected ~%.2f)",
					char, freq, expectedFreq)
				// Don't fail the test as some deviation is normal with small samples
			}
		}
	})
}

// TestResourceUsage tests that the implementation doesn't use excessive resources
func TestResourceUsage(t *testing.T) {
	hasher := createTestHasher(t)

	t.Run("MemoryUsage", func(t *testing.T) {
		// Test with large inputs
		largeInput := strings.Repeat("a", 10000)

		_, err := hasher.Hash(largeInput, DataTypeGeneric)
		if err != nil {
			t.Fatalf("Hash of large input failed: %v", err)
		}

		// If we get here without running out of memory, the test passes
	})

	t.Run("ConfigValidation", func(t *testing.T) {
		// Test that invalid configurations are rejected
		invalidConfigs := []*HashingConfig{
			{MasterKey: make([]byte, 16)},                   // Too short
			{MasterKey: make([]byte, 64), Argon2Memory: 0},  // Invalid memory
			{MasterKey: make([]byte, 64), Argon2Time: 0},    // Invalid time
			{MasterKey: make([]byte, 64), Argon2Threads: 0}, // Invalid threads
			{MasterKey: make([]byte, 64), Argon2KeyLen: 0},  // Invalid key length
		}

		for _, config := range invalidConfigs {
			if config.Argon2Memory == 0 {
				config.Argon2Memory = 0 // Explicitly set invalid value
				config.Argon2Time = 1
				config.Argon2Threads = 1
				config.Argon2KeyLen = 32
			} else if config.Argon2Time == 0 {
				config.Argon2Memory = 1024
				config.Argon2Time = 0 // Explicitly set invalid value
				config.Argon2Threads = 1
				config.Argon2KeyLen = 32
			} else if config.Argon2Threads == 0 {
				config.Argon2Memory = 1024
				config.Argon2Time = 1
				config.Argon2Threads = 0 // Explicitly set invalid value
				config.Argon2KeyLen = 32
			} else if config.Argon2KeyLen == 0 {
				config.Argon2Memory = 1024
				config.Argon2Time = 1
				config.Argon2Threads = 1
				config.Argon2KeyLen = 0 // Explicitly set invalid value
			} else {
				// Set valid defaults for unspecified values
				config.Argon2Memory = 1024
				config.Argon2Time = 1
				config.Argon2Threads = 1
				config.Argon2KeyLen = 32
			}

			_, err := NewHasher(config)
			if err == nil {
				t.Error("Expected error for invalid configuration")
			}
		}
	})
}

// TestLevels verifies that privacy levels work correctly
func TestLevels(t *testing.T) {
	config := DefaultConfig()
	config.MasterKey = make([]byte, 64)

	testData := "192.168.1.100"

	t.Run("LevelNone", func(t *testing.T) {
		config.IPLevel = LevelNone
		hasher, _ := NewHasher(config)

		result, err := hasher.HashIP(testData)
		if err != nil {
			t.Fatalf("HashIP failed: %v", err)
		}

		if result != testData {
			t.Error("Privacy level none should return original data")
		}
	})

	t.Run("LevelPartial", func(t *testing.T) {
		config.IPLevel = LevelPartial
		hasher, _ := NewHasher(config)

		result, err := hasher.HashIP(testData)
		if err != nil {
			t.Fatalf("HashIP failed: %v", err)
		}

		if result == testData {
			t.Error("Privacy level partial should not return original data")
		}

		if strings.HasPrefix(result, "full_") {
			t.Error("Privacy level partial should not use full hash format")
		}
	})

	t.Run("LevelFull", func(t *testing.T) {
		config.IPLevel = LevelFull
		hasher, _ := NewHasher(config)

		result, err := hasher.HashIP(testData)
		if err != nil {
			t.Fatalf("HashIP failed: %v", err)
		}

		if result == testData {
			t.Error("Privacy level full should not return original data")
		}

		if !strings.HasPrefix(result, "full_ip_") {
			t.Error("Privacy level full should use full hash format")
		}
	})
}

// Helper function (Go doesn't have built-in min for int)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestMasterKeyGeneration verifies master key generation security
func TestMasterKeyGeneration(t *testing.T) {
	t.Run("KeyUniqueness", func(t *testing.T) {
		keys := make([][]byte, 10)

		for i := 0; i < 10; i++ {
			key, err := GenerateMasterKey()
			if err != nil {
				t.Fatalf("GenerateMasterKey failed: %v", err)
			}
			keys[i] = key
		}

		// Check that all keys are different
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if string(keys[i]) == string(keys[j]) {
					t.Error("Generated keys should be unique")
				}
			}
		}
	})

	t.Run("KeyEntropy", func(t *testing.T) {
		key, err := GenerateMasterKey()
		if err != nil {
			t.Fatalf("GenerateMasterKey failed: %v", err)
		}

		// Basic entropy check - count unique bytes
		byteCount := make(map[byte]int)
		for _, b := range key {
			byteCount[b]++
		}

		// Should have good byte distribution (not all same byte)
		if len(byteCount) < 16 {
			t.Error("Generated key has poor entropy (too few unique bytes)")
		}
	})

	t.Run("Base64Encoding", func(t *testing.T) {
		keyStr, err := GenerateMasterKeyBase64()
		if err != nil {
			t.Fatalf("GenerateMasterKeyBase64 failed: %v", err)
		}

		// Should be valid base64
		decoded, err := base64.StdEncoding.DecodeString(keyStr)
		if err != nil {
			t.Errorf("Generated key is not valid base64: %v", err)
		}

		if len(decoded) != 64 {
			t.Errorf("Decoded key should be 64 bytes, got %d", len(decoded))
		}
	})
}
