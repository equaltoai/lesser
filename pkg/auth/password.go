package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBcryptCost is the default cost factor for bcrypt hashing
const DefaultBcryptCost = 12

// Password related errors
var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must be less than 72 characters")
)

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
		return "", err
	}

	return string(hash), nil
}

// VerifyPassword checks if a password matches the hash
func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
