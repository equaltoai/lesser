// Package privacy provides cryptographically secure privacy-preserving hashing utilities
// for protecting sensitive user data while maintaining analytical capabilities.
package privacy

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"golang.org/x/crypto/argon2"
)

// DataType represents the type of data being hashed for different privacy strategies
type DataType string

const (
	// DataTypeIP represents IP address data
	DataTypeIP DataType = "ip"
	// DataTypeEmail represents email address data
	DataTypeEmail DataType = "email"
	// DataTypeUsername represents username data
	DataTypeUsername DataType = "username"
	// DataTypePII represents personally identifiable information
	DataTypePII DataType = "pii"
	// DataTypeGeneric represents generic sensitive data
	DataTypeGeneric DataType = "generic"
)

// Level defines the level of privacy protection
type Level string

const (
	// LevelNone provides no privacy protection (original data)
	LevelNone Level = "none"
	// LevelPartial provides partial privacy (preserves some analytical value)
	LevelPartial Level = "partial"
	// LevelFull provides maximum privacy protection (full hash)
	LevelFull Level = "full"
)

// HashingConfig defines configuration for privacy hashing
type HashingConfig struct {
	// Master key for HMAC (must be kept secret)
	MasterKey []byte
	// Privacy levels for different data types
	IPLevel       Level
	EmailLevel    Level
	UsernameLevel Level
	PIILevel      Level
	GenericLevel  Level
	// Key rotation settings
	KeyRotationEnabled  bool
	KeyRotationInterval time.Duration
	// Performance settings
	Argon2Memory  uint32 // Memory usage in KB
	Argon2Time    uint32 // Number of iterations
	Argon2Threads uint8  // Number of threads
	Argon2KeyLen  uint32 // Length of derived key
}

// DefaultConfig returns a secure default configuration
func DefaultConfig() *HashingConfig {
	return &HashingConfig{
		IPLevel:             LevelPartial,
		EmailLevel:          LevelPartial,
		UsernameLevel:       LevelFull,
		PIILevel:            LevelFull,
		GenericLevel:        LevelFull,
		KeyRotationEnabled:  false, // Disabled by default for consistency
		KeyRotationInterval: 24 * time.Hour,
		// Argon2id parameters (moderate security/performance balance)
		Argon2Memory:  64 * 1024, // 64 MB
		Argon2Time:    3,         // 3 iterations
		Argon2Threads: 4,         // 4 threads
		Argon2KeyLen:  32,        // 32 bytes output
	}
}

// Hasher provides cryptographically secure privacy hashing
type Hasher struct {
	config *HashingConfig
}

// NewHasher creates a new privacy hasher with the given configuration
func NewHasher(config *HashingConfig) (*Hasher, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate master key
	if len(config.MasterKey) < 32 {
		return nil, fmt.Errorf("master key must be at least 32 bytes, got %d", len(config.MasterKey))
	}

	// Validate Argon2 parameters
	if config.Argon2Memory < 1024 {
		return nil, fmt.Errorf("argon2 memory must be at least 1024 KB")
	}
	if config.Argon2Time < 1 {
		return nil, fmt.Errorf("argon2 time must be at least 1")
	}
	if config.Argon2Threads < 1 {
		return nil, fmt.Errorf("argon2 threads must be at least 1")
	}
	if config.Argon2KeyLen < 16 {
		return nil, fmt.Errorf("argon2 key length must be at least 16 bytes")
	}

	return &Hasher{
		config: config,
	}, nil
}

// NewHasherFromMasterKey creates a hasher with default config and the given master key
func NewHasherFromMasterKey(masterKey string) (*Hasher, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(masterKey)
	if err != nil {
		// Try hex decoding if base64 fails
		keyBytes, err = hex.DecodeString(masterKey)
		if err != nil {
			// Use the string directly as bytes (less secure but functional)
			keyBytes = []byte(masterKey)
		}
	}

	config := DefaultConfig()
	config.MasterKey = keyBytes

	return NewHasher(config)
}

// GenerateMasterKey generates a cryptographically secure master key
func GenerateMasterKey() ([]byte, error) {
	key := make([]byte, 64) // 512-bit key
	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// GenerateMasterKeyBase64 generates a master key and returns it as base64
func GenerateMasterKeyBase64() (string, error) {
	key, err := GenerateMasterKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Hash provides privacy-preserving hashing for different data types
func (ph *Hasher) Hash(data string, dataType DataType) (string, error) {
	if err := common.ValidateRequiredParam("data", data); err != nil {
		return "", nil
	}

	// For Hash() method, always use full hashing regardless of privacy level
	// Use specific methods like HashIP() and HashEmail() for privacy-level-aware hashing
	return ph.hashFull(data, dataType)
}

// HashIP provides IP address privacy hashing with optional subnet preservation
func (ph *Hasher) HashIP(ipAddress string) (string, error) {
	if err := common.ValidateRequiredParam("ip_address", ipAddress); err != nil {
		return "", nil
	}

	privacyLevel := ph.config.IPLevel

	switch privacyLevel {
	case LevelNone:
		return ipAddress, nil
	case LevelPartial:
		return ph.hashIPPartial(ipAddress)
	case LevelFull:
		return ph.hashFull(ipAddress, DataTypeIP)
	default:
		return ph.hashFull(ipAddress, DataTypeIP)
	}
}

// HashEmail provides email privacy hashing with optional domain preservation
func (ph *Hasher) HashEmail(email string) (string, error) {
	if err := common.ValidateRequiredParam("email", email); err != nil {
		return "", nil
	}

	privacyLevel := ph.config.EmailLevel

	switch privacyLevel {
	case LevelNone:
		return email, nil
	case LevelPartial:
		return ph.hashEmailPartial(email)
	case LevelFull:
		return ph.hashFull(email, DataTypeEmail)
	default:
		return ph.hashFull(email, DataTypeEmail)
	}
}

// HashUsername provides username privacy hashing
func (ph *Hasher) HashUsername(username string) (string, error) {
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return "", nil
	}

	privacyLevel := ph.getLevel(DataTypeUsername)

	switch privacyLevel {
	case LevelNone:
		return username, nil
	case LevelPartial:
		return ph.hashPartial(username, DataTypeUsername)
	case LevelFull:
		return ph.hashFull(username, DataTypeUsername)
	default:
		return ph.hashFull(username, DataTypeUsername)
	}
}

// HashPII provides PII privacy hashing
func (ph *Hasher) HashPII(pii string) (string, error) {
	if err := common.ValidateRequiredParam("pii", pii); err != nil {
		return "", nil
	}

	privacyLevel := ph.getLevel(DataTypePII)

	switch privacyLevel {
	case LevelNone:
		return pii, nil
	case LevelPartial:
		return ph.hashPartial(pii, DataTypePII)
	case LevelFull:
		return ph.hashFull(pii, DataTypePII)
	default:
		return ph.hashFull(pii, DataTypePII)
	}
}

// getLevel returns the privacy level for a given data type
func (ph *Hasher) getLevel(dataType DataType) Level {
	switch dataType {
	case DataTypeIP:
		return ph.config.IPLevel
	case DataTypeEmail:
		return ph.config.EmailLevel
	case DataTypeUsername:
		return ph.config.UsernameLevel
	case DataTypePII:
		return ph.config.PIILevel
	case DataTypeGeneric:
		return ph.config.GenericLevel
	default:
		return ph.config.GenericLevel
	}
}

// hashFull provides maximum privacy protection using HMAC-SHA256
func (ph *Hasher) hashFull(data string, dataType DataType) (string, error) {
	// Create HMAC with context-specific key derivation
	contextKey := ph.deriveContextKey(dataType)

	h := hmac.New(sha256.New, contextKey)
	h.Write([]byte(data))
	hash := h.Sum(nil)

	// Return hex-encoded hash with prefix to indicate full hashing
	return fmt.Sprintf("full_%s_%s", dataType, hex.EncodeToString(hash)), nil
}

// hashPartial provides partial privacy protection preserving some analytical value
func (ph *Hasher) hashPartial(data string, dataType DataType) (string, error) {
	switch dataType {
	case DataTypeIP:
		return ph.hashIPPartial(data)
	case DataTypeEmail:
		return ph.hashEmailPartial(data)
	case DataTypeUsername:
		return ph.hashUsernamePartial(data)
	case DataTypePII:
		return ph.hashPIIPartial(data)
	default:
		// For other types, use full hashing in partial mode
		return ph.hashFull(data, dataType)
	}
}

// hashIPPartial preserves network information while hashing host portion
func (ph *Hasher) hashIPPartial(ipAddress string) (string, error) {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		// If not a valid IP, hash the entire string
		return ph.hashFull(ipAddress, DataTypeIP)
	}

	if ipv4 := ip.To4(); ipv4 != nil {
		// IPv4: preserve first two octets, hash last two
		network := fmt.Sprintf("%d.%d", ipv4[0], ipv4[1])
		hostPortion := fmt.Sprintf("%d.%d", ipv4[2], ipv4[3])

		hashedHost, err := ph.hashFull(hostPortion, DataTypeIP)
		if err != nil {
			return "", err
		}

		// Return format: network.hashedHost
		return fmt.Sprintf("%s.%s", network, hashedHost[:8]), nil
	}

	// IPv6: preserve first 4 groups, hash the rest
	ipv6 := ip.To16()
	if ipv6 == nil {
		return ph.hashFull(ipAddress, DataTypeIP)
	}

	// Extract first 64 bits (8 bytes) as network portion
	networkBytes := ipv6[:8]
	hostBytes := ipv6[8:]

	hashedHost, err := ph.hashFull(hex.EncodeToString(hostBytes), DataTypeIP)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s::%s", hex.EncodeToString(networkBytes), hashedHost[:16]), nil
}

// hashEmailPartial preserves domain while hashing local part
func (ph *Hasher) hashEmailPartial(email string) (string, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// Invalid email format, hash the entire string
		return ph.hashFull(email, DataTypeEmail)
	}

	localPart := parts[0]
	domain := parts[1]

	hashedLocal, err := ph.hashFull(localPart, DataTypeEmail)
	if err != nil {
		return "", err
	}

	// Return format: hashedLocal@domain
	return fmt.Sprintf("%s@%s", hashedLocal[:16], domain), nil
}

// hashUsernamePartial preserves username length and character patterns while hashing content
func (ph *Hasher) hashUsernamePartial(username string) (string, error) {
	if len(username) <= 2 {
		// For very short usernames, use full hashing for security
		return ph.hashFull(username, DataTypeUsername)
	}

	// Preserve first and last character, hash the middle
	firstChar := string(username[0])
	lastChar := string(username[len(username)-1])
	middlePart := username[1 : len(username)-1]

	// Hash the middle portion
	hashedMiddle, err := ph.hashFull(middlePart, DataTypeUsername)
	if err != nil {
		return "", err
	}

	// Calculate how many characters to show from the hash to preserve original length
	// Use minimum of available hash length and required middle length
	middleLen := len(middlePart)
	hashLen := len(hashedMiddle)
	if hashLen < middleLen {
		// If hash is shorter than needed, repeat it
		repeats := (middleLen / hashLen) + 1
		repeatedHash := strings.Repeat(hashedMiddle, repeats)
		hashedMiddle = repeatedHash[:middleLen]
	} else {
		// Use only the needed portion of the hash
		hashedMiddle = hashedMiddle[:middleLen]
	}

	// Return format: firstChar + hashedMiddle + lastChar
	return fmt.Sprintf("%s%s%s", firstChar, hashedMiddle, lastChar), nil
}

// hashPIIPartial provides partial hashing for PII data
// For most PII types, this falls back to full hashing for security
func (ph *Hasher) hashPIIPartial(pii string) (string, error) {
	// For PII data, partial hashing is risky as it may expose sensitive information
	// We preserve only basic structural information that's safe for analytics

	// Check if it looks like a social security number or similar ID
	if ph.looksLikeSSN(pii) {
		// For SSN-like data, use full hashing for maximum security
		return ph.hashFull(pii, DataTypePII)
	}

	// Check if it looks like a phone number (digits, spaces, dashes, parentheses)
	if ph.looksLikePhoneNumber(pii) {
		// For phone numbers, preserve length and format structure
		return ph.hashPhoneNumberPartial(pii)
	}

	// For other PII types, preserve only length information
	length := len(pii)
	hashedData, err := ph.hashFull(pii, DataTypePII)
	if err != nil {
		return "", err
	}

	// Return format indicating original length for analytics
	return fmt.Sprintf("pii_len%d_%s", length, hashedData[:16]), nil
}

// looksLikePhoneNumber checks if the string appears to be a phone number
func (ph *Hasher) looksLikePhoneNumber(data string) bool {
	// Simple heuristic: contains mostly digits with common phone formatting chars
	digitCount := 0
	totalCount := 0
	for _, r := range data {
		totalCount++
		if r >= '0' && r <= '9' {
			digitCount++
		} else if r != ' ' && r != '-' && r != '(' && r != ')' && r != '+' && r != '.' {
			// Contains non-phone-number characters
			return false
		}
	}
	// At least 70% digits and minimum 7 digits total (short phone numbers)
	return digitCount >= 7 && float64(digitCount)/float64(totalCount) >= 0.7
}

// looksLikeSSN checks if the string appears to be a social security number
func (ph *Hasher) looksLikeSSN(data string) bool {
	// Common SSN patterns: XXX-XX-XXXX, XXXXXXXXX, XXX XX XXXX
	cleanData := strings.ReplaceAll(strings.ReplaceAll(data, "-", ""), " ", "")
	if len(cleanData) != 9 {
		return false
	}
	// Check if all characters are digits
	for _, r := range cleanData {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hashPhoneNumberPartial preserves phone number structure while hashing digits
func (ph *Hasher) hashPhoneNumberPartial(phoneNumber string) (string, error) {
	// Extract digits only
	var digits strings.Builder
	var structure strings.Builder

	for _, r := range phoneNumber {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			structure.WriteRune('X') // Mark digit positions
		} else {
			structure.WriteRune(r) // Preserve formatting characters
		}
	}

	// Hash the digits
	hashedDigits, err := ph.hashFull(digits.String(), DataTypePII)
	if err != nil {
		return "", err
	}

	// Replace X's in structure with hashed digits (cycling through hash if needed)
	structureStr := structure.String()
	hashIndex := 0
	result := strings.Builder{}

	for _, r := range structureStr {
		if r == 'X' {
			// Use character from hash (cycling if necessary)
			if hashIndex < len(hashedDigits) {
				result.WriteByte(hashedDigits[hashIndex])
				hashIndex++
			} else {
				// Cycle back to beginning of hash
				hashIndex = 0
				result.WriteByte(hashedDigits[hashIndex])
				hashIndex++
			}
		} else {
			result.WriteRune(r)
		}
	}

	return result.String(), nil
}

// deriveContextKey derives a context-specific key for different data types
func (ph *Hasher) deriveContextKey(dataType DataType) []byte {
	context := fmt.Sprintf("privacy_hash_%s", dataType)

	// Use Argon2id for key derivation with the context as salt
	salt := sha256.Sum256([]byte(context))

	derivedKey := argon2.IDKey(
		ph.config.MasterKey,
		salt[:],
		ph.config.Argon2Time,
		ph.config.Argon2Memory,
		ph.config.Argon2Threads,
		ph.config.Argon2KeyLen,
	)

	return derivedKey
}

// VerifyHash verifies if a hash was created with the current configuration
// This is useful for migration scenarios and integrity checking
func (ph *Hasher) VerifyHash(originalData, hash string, dataType DataType) (bool, error) {
	expectedHash, err := ph.Hash(originalData, dataType)
	if err != nil {
		return false, err
	}

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(hash), []byte(expectedHash)) == 1, nil
}

// GetConfig returns a copy of the current configuration (without the master key for security)
func (ph *Hasher) GetConfig() *HashingConfig {
	return &HashingConfig{
		MasterKey:           nil, // Don't expose the master key
		IPLevel:             ph.config.IPLevel,
		EmailLevel:          ph.config.EmailLevel,
		UsernameLevel:       ph.config.UsernameLevel,
		PIILevel:            ph.config.PIILevel,
		GenericLevel:        ph.config.GenericLevel,
		KeyRotationEnabled:  ph.config.KeyRotationEnabled,
		KeyRotationInterval: ph.config.KeyRotationInterval,
		Argon2Memory:        ph.config.Argon2Memory,
		Argon2Time:          ph.config.Argon2Time,
		Argon2Threads:       ph.config.Argon2Threads,
		Argon2KeyLen:        ph.config.Argon2KeyLen,
	}
}

// UpdateConfig updates the hasher configuration
func (ph *Hasher) UpdateConfig(config *HashingConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate the new configuration
	if len(config.MasterKey) < 32 {
		return fmt.Errorf("master key must be at least 32 bytes")
	}

	ph.config = config
	return nil
}
