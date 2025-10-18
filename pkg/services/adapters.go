package services

import (
	"crypto/rsa"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// CryptoAdapter implements accounts.CryptoService using the federation package
type CryptoAdapter struct{}

// NewCryptoAdapter creates a new crypto adapter
func NewCryptoAdapter() *CryptoAdapter {
	return &CryptoAdapter{}
}

// GenerateRSAKeyPair generates an RSA key pair
func (c *CryptoAdapter) GenerateRSAKeyPair(bits int) (interface{}, error) {
	// Use the federation package implementation
	privateKey, err := federation.GenerateRSAKeyPair(bits)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// EncodePublicKeyPEM encodes a public key to PEM format
func (c *CryptoAdapter) EncodePublicKeyPEM(publicKey interface{}) ([]byte, error) {
	// Type assert to RSA private key (which contains the public key)
	switch key := publicKey.(type) {
	case *rsa.PrivateKey:
		// Use the federation package implementation
		return federation.EncodePublicKeyPEM(&key.PublicKey)
	case *rsa.PublicKey:
		return federation.EncodePublicKeyPEM(key)
	default:
		return nil, ErrUnsupportedKeyType
	}
}

// EncodePrivateKeyPEM encodes a private key to PEM format
func (c *CryptoAdapter) EncodePrivateKeyPEM(privateKey interface{}) ([]byte, error) {
	// Type assert to RSA private key
	key, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidPrivateKeyType
	}
	// Use the federation package implementation
	return federation.EncodePrivateKeyPEM(key)
}

// AuthAdapter implements accounts.AuthService using the auth package
type AuthAdapter struct {
	jwtSecret string
	storage   core.RepositoryStorage
}

// NewAuthAdapter creates a new auth adapter
func NewAuthAdapter(jwtSecret string, storage core.RepositoryStorage) *AuthAdapter {
	return &AuthAdapter{
		jwtSecret: jwtSecret,
		storage:   storage,
	}
}

// HashPassword hashes a password
func (a *AuthAdapter) HashPassword(password string) (string, error) {
	return auth.HashPassword(password)
}

// ValidatePassword validates a password against requirements
func (a *AuthAdapter) ValidatePassword(password, username string) error {
	return auth.ValidatePassword(password, username)
}

// PasswordStrength returns the strength score of a password
func (a *AuthAdapter) PasswordStrength(password string) int {
	return auth.PasswordStrength(password)
}

// Ensure adapters implement the interfaces
var (
	_ accounts.CryptoService = (*CryptoAdapter)(nil)
	_ accounts.AuthService   = (*AuthAdapter)(nil)
)
