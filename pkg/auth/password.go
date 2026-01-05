package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBcryptCost is the default cost factor for bcrypt hashing
const DefaultBcryptCost = 12

// PasswordPolicy defines password requirements
type PasswordPolicy struct {
	MinLength              int
	RequireUppercase       bool
	RequireLowercase       bool
	RequireNumbers         bool
	RequireSpecialChars    bool
	PreventCommonPasswords bool
}

// DefaultPolicy defines the default password requirements
var DefaultPolicy = PasswordPolicy{
	MinLength:              12,
	RequireUppercase:       true,
	RequireLowercase:       true,
	RequireNumbers:         true,
	RequireSpecialChars:    true,
	PreventCommonPasswords: true,
}

// PasswordStrengthConfig controls how PasswordStrength scores passwords.
type PasswordStrengthConfig struct {
	// MinLength is the minimum length to score above 0.
	MinLength int

	// LongLength is the length threshold for the long password bonus.
	LongLength int
	// LongBonus is the bonus applied when the long password condition is met.
	LongBonus int
	// RequireAllCharacterTypesForLongBonus requires upper/lower/number/special for the long bonus.
	RequireAllCharacterTypesForLongBonus bool

	// SequentialPenalty is subtracted when sequential patterns are detected.
	SequentialPenalty int
	// SequentialPatternMinRun is the minimum length of an ascending/descending run
	// (for letters or digits) before it is considered sequential.
	SequentialPatternMinRun int
	// RepeatedPenalty is subtracted when repeated characters are detected.
	RepeatedPenalty int
}

// DefaultPasswordStrengthConfig matches the built-in strength scoring used by PasswordStrength.
var DefaultPasswordStrengthConfig = PasswordStrengthConfig{
	MinLength:  8,
	LongLength: 16,
	LongBonus:  1,

	RequireAllCharacterTypesForLongBonus: true,
	SequentialPenalty:                    2,
	SequentialPatternMinRun:              4,
	RepeatedPenalty:                      1,
}

// Note: Common passwords are now defined in common_passwords.go

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	// Validate password length
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}
	if len(password) > 72 {
		// Bcrypt has a maximum password length of 72 bytes
		return "", ErrPasswordTooLong
	}

	// Generate hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", errors.Join(ErrPasswordHashFailed, err)
	}

	return string(hash), nil
}

// VerifyPassword checks if a password matches the hash
func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// ValidatePassword checks if a password meets security requirements
func ValidatePassword(password string, username string) error {
	// Check minimum length
	if len(password) < DefaultPolicy.MinLength {
		return fmt.Errorf("password must be at least %d characters", DefaultPolicy.MinLength)
	}

	// Check character requirements
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if DefaultPolicy.RequireUppercase && !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if DefaultPolicy.RequireLowercase && !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if DefaultPolicy.RequireNumbers && !hasNumber {
		return errors.New("password must contain at least one number")
	}
	if DefaultPolicy.RequireSpecialChars && !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	// Check against username
	if strings.Contains(strings.ToLower(password), strings.ToLower(username)) {
		return errors.New("password cannot contain username")
	}

	// Check common passwords - do this check before other pattern checks
	// This is important for the test case with "password@123"
	if DefaultPolicy.PreventCommonPasswords && IsCommonPassword(password) {
		return errors.New("password is too common")
	}

	// Check for sequential patterns
	if hasSequentialPattern(password) {
		return errors.New("password cannot contain sequential characters")
	}

	// Check for repeated characters
	if hasRepeatedPattern(password) {
		return errors.New("password cannot contain repeated characters")
	}

	return nil
}

// hasSequentialPattern checks for sequential number or letter patterns
func hasSequentialPattern(password string) bool {
	sequentialNumbers := []string{"012", "123", "234", "345", "456", "567", "678", "789", "890", "987", "876", "765", "654", "543", "432", "321", "210"}
	sequentialLetters := []string{"abc", "bcd", "cde", "def", "efg", "fgh", "ghi", "hij", "ijk", "jkl", "klm", "lmn", "mno", "nop", "opq", "pqr", "qrs", "rst", "stu", "tuv", "uvw", "vwx", "wxy", "xyz"}

	lowerPass := strings.ToLower(password)

	for _, pattern := range sequentialNumbers {
		if strings.Contains(password, pattern) {
			return true
		}
	}

	for _, pattern := range sequentialLetters {
		if strings.Contains(lowerPass, pattern) {
			return true
		}
	}

	return false
}

// hasRepeatedPattern checks for repeated character patterns
func hasRepeatedPattern(password string) bool {
	// Go's regexp doesn't support backreferences, so we need to check manually
	for i := 0; i < len(password)-2; i++ {
		if password[i] == password[i+1] && password[i] == password[i+2] {
			return true
		}
	}
	return false
}

// PasswordStrength calculates password strength score (0-5).
//
// The score is based on how many character classes are present (lower/upper/number/special),
// plus a long-password bonus (default: length >= 16 and all classes), with penalties for
// sequential or repeated-character patterns.
func PasswordStrength(password string) int {
	return PasswordStrengthWithConfig(password, DefaultPasswordStrengthConfig)
}

// PasswordStrengthWithConfig calculates password strength score (0-5) using the provided config.
func PasswordStrengthWithConfig(password string, cfg PasswordStrengthConfig) int {
	if cfg.MinLength <= 0 {
		cfg.MinLength = DefaultPasswordStrengthConfig.MinLength
	}
	if cfg.LongLength <= 0 {
		cfg.LongLength = DefaultPasswordStrengthConfig.LongLength
	}
	if cfg.SequentialPatternMinRun <= 0 {
		cfg.SequentialPatternMinRun = DefaultPasswordStrengthConfig.SequentialPatternMinRun
	}

	if len(password) < cfg.MinLength {
		return 0
	}

	hasLower, hasUpper, hasNumber, hasSpecial := passwordCharacterClasses(password)

	score := 0
	if hasLower {
		score++
	}
	if hasUpper {
		score++
	}
	if hasNumber {
		score++
	}
	if hasSpecial {
		score++
	}

	if cfg.LongBonus > 0 && len(password) >= cfg.LongLength {
		longBonusAllowed := true
		if cfg.RequireAllCharacterTypesForLongBonus {
			longBonusAllowed = hasLower && hasUpper && hasNumber && hasSpecial
		}
		if longBonusAllowed {
			score += cfg.LongBonus
		}
	}

	// Penalty for patterns
	if cfg.SequentialPenalty > 0 && hasSequentialRun(password, cfg.SequentialPatternMinRun) {
		score -= cfg.SequentialPenalty
	}
	if cfg.RepeatedPenalty > 0 && hasRepeatedPattern(password) {
		score -= cfg.RepeatedPenalty
	}

	// Normalize score
	if score < 0 {
		score = 0
	}
	if score > 5 {
		score = 5
	}

	return score
}

func passwordCharacterClasses(password string) (hasLower, hasUpper, hasNumber, hasSpecial bool) {
	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	return hasLower, hasUpper, hasNumber, hasSpecial
}

func hasSequentialRun(password string, minRun int) bool {
	if minRun <= 1 {
		return true
	}

	lower := strings.ToLower(password)
	b := []byte(lower)

	runLength := 1
	runDir := 0 // 1 ascending, -1 descending, 0 none

	for i := 1; i < len(b); i++ {
		prev, cur := b[i-1], b[i]

		if !isASCIIAlphaNum(prev) || !isASCIIAlphaNum(cur) || !sameASCIIGroup(prev, cur) {
			runLength = 1
			runDir = 0
			continue
		}

		diff := int(cur) - int(prev)
		if diff != 1 && diff != -1 {
			runLength = 1
			runDir = 0
			continue
		}

		if runDir == 0 || runDir == diff {
			runDir = diff
			if runLength == 1 {
				runLength = 2
			} else {
				runLength++
			}
		} else {
			runDir = diff
			runLength = 2
		}

		if runLength >= minRun {
			return true
		}
	}

	return false
}

func isASCIIAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func sameASCIIGroup(a, b byte) bool {
	return (a >= 'a' && a <= 'z' && b >= 'a' && b <= 'z') ||
		(a >= '0' && a <= '9' && b >= '0' && b <= '9')
}

// PasswordStrengthLabel returns a human-readable strength label
func PasswordStrengthLabel(strength int) string {
	labels := []string{
		"Very Weak",
		"Weak",
		"Fair",
		"Good",
		"Strong",
		"Very Strong",
	}

	if strength < 0 {
		strength = 0
	}
	if strength >= len(labels) {
		strength = len(labels) - 1
	}

	return labels[strength]
}

// GeneratePasswordHint provides helpful hints for password improvement
func GeneratePasswordHint(password string) []string {
	hints := []string{}

	if len(password) < 12 {
		hints = append(hints, fmt.Sprintf("Add %d more characters", 12-len(password)))
	}

	if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		hints = append(hints, "Add uppercase letters")
	}

	if !regexp.MustCompile(`[a-z]`).MatchString(password) {
		hints = append(hints, "Add lowercase letters")
	}

	if !regexp.MustCompile(`[0-9]`).MatchString(password) {
		hints = append(hints, "Add numbers")
	}

	if !regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) {
		hints = append(hints, "Add special characters (!@#$%^&*)")
	}

	if hasSequentialPattern(password) {
		hints = append(hints, "Avoid sequential patterns (123, abc)")
	}

	if hasRepeatedPattern(password) {
		hints = append(hints, "Avoid repeated characters")
	}

	return hints
}
