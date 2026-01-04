package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"math/big"
	"time"
)

// ConstantTimeCompare performs a constant-time comparison of two strings
func ConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ConstantTimeComparePadded performs a constant-time comparison without early exit on length mismatch.
// Length still influences runtime due to required padding work; for secrets of fixed length this is acceptable.
func ConstantTimeComparePadded(a, b string) bool {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	paddedA := padToLength(a, maxLen)
	paddedB := padToLength(b, maxLen)

	return subtle.ConstantTimeCompare([]byte(paddedA), []byte(paddedB)) == 1 && len(a) == len(b)
}

// ConstantTimeDelay adds a small random delay to prevent timing analysis
func ConstantTimeDelay() {
	// Random delay between 0-10ms
	delay := time.Duration(secureRandInt(10)) * time.Millisecond
	time.Sleep(delay)
}

// TimingSafeTokenValidation validates tokens with timing attack protection
func TimingSafeTokenValidation(providedToken, storedToken string) bool {
	// Always perform the comparison even if lengths differ
	maxLen := len(providedToken)
	if len(storedToken) > maxLen {
		maxLen = len(storedToken)
	}

	// Pad both to same length
	paddedProvided := padToLength(providedToken, maxLen)
	paddedStored := padToLength(storedToken, maxLen)

	// Constant time comparison
	result := subtle.ConstantTimeCompare([]byte(paddedProvided), []byte(paddedStored)) == 1

	// Add small random delay
	ConstantTimeDelay()

	// Return result only after all operations
	return result && len(providedToken) == len(storedToken)
}

// ValidateAPIKey validates an API key with timing attack protection
func ValidateAPIKey(provided string, getStoredKey func() (string, error)) error {
	stored, err := getStoredKey()
	if err != nil {
		ConstantTimeDelay() // Delay even on error
		return err
	}

	if !TimingSafeTokenValidation(provided, stored) {
		ConstantTimeDelay()
		return ErrInvalidAPIKey
	}

	return nil
}

// padToLength pads a string to the specified length with null bytes
func padToLength(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}

	padded := make([]byte, length)
	copy(padded, s)
	return string(padded)
}

// secureRandInt generates a cryptographically secure random integer up to max
func secureRandInt(maxVal int) int {
	if maxVal <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal)))
	if err != nil {
		// Fallback to 0 on error
		return 0
	}

	return int(n.Int64())
}

// TimingSafeStringSliceContains checks if a string is in a slice with timing protection
func TimingSafeStringSliceContains(slice []string, target string) bool {
	found := false

	// Check all elements to avoid early exit timing leaks
	for _, s := range slice {
		if ConstantTimeCompare(s, target) {
			found = true
			// Don't break - continue checking all elements
		}
	}

	// Add random delay
	ConstantTimeDelay()

	return found
}

// ValidateSessionToken validates a session token with timing protection
func ValidateSessionToken(token string, validateFunc func(string) (bool, error)) error {
	// Add initial delay
	ConstantTimeDelay()

	valid, err := validateFunc(token)

	// Always add delay regardless of result
	ConstantTimeDelay()

	if err != nil {
		return err
	}

	if !valid {
		return ErrInvalidToken
	}

	return nil
}
