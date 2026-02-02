package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnsupportedAgentKeyType = errors.New("unsupported agent key type")
	ErrInvalidAgentPublicKey   = errors.New("invalid agent public key")
	ErrInvalidAgentSignature   = errors.New("invalid agent signature")
)

func ParseAgentPublicKey(keyType string, publicKey string) (crypto.PublicKey, error) {
	keyType = strings.ToLower(strings.TrimSpace(keyType))
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return nil, ErrInvalidAgentPublicKey
	}

	switch keyType {
	case "ed25519":
		return parseEd25519PublicKey(publicKey)
	case "rsa":
		return parseRSAPublicKey(publicKey)
	default:
		return nil, ErrUnsupportedAgentKeyType
	}
}

func VerifyAgentChallengeSignature(keyType string, publicKey string, message string, signatureBase64 string) error {
	pub, err := ParseAgentPublicKey(keyType, publicKey)
	if err != nil {
		return err
	}

	sig, err := decodeBase64Any(signatureBase64)
	if err != nil {
		return ErrInvalidAgentSignature
	}

	msgBytes := []byte(message)

	switch p := pub.(type) {
	case ed25519.PublicKey:
		if len(sig) != ed25519.SignatureSize || !ed25519.Verify(p, msgBytes, sig) {
			return ErrInvalidAgentSignature
		}
		return nil
	case *rsa.PublicKey:
		sum := sha256.Sum256(msgBytes)
		if err := rsa.VerifyPKCS1v15(p, crypto.SHA256, sum[:], sig); err == nil {
			return nil
		}
		if err := rsa.VerifyPSS(p, crypto.SHA256, sum[:], sig, nil); err == nil {
			return nil
		}
		return ErrInvalidAgentSignature
	default:
		return ErrInvalidAgentPublicKey
	}
}

func parseEd25519PublicKey(publicKey string) (crypto.PublicKey, error) {
	if !strings.Contains(publicKey, "BEGIN") {
		raw, err := decodeBase64Any(publicKey)
		if err != nil {
			return nil, ErrInvalidAgentPublicKey
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, ErrInvalidAgentPublicKey
		}
		return ed25519.PublicKey(raw), nil
	}

	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return nil, ErrInvalidAgentPublicKey
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ErrInvalidAgentPublicKey
	}

	ed, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, ErrInvalidAgentPublicKey
	}
	return ed, nil
}

func parseRSAPublicKey(publicKey string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKey))
	if block == nil {
		return nil, ErrInvalidAgentPublicKey
	}

	switch strings.TrimSpace(block.Type) {
	case "RSA PUBLIC KEY":
		rsaKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, ErrInvalidAgentPublicKey
		}
		return rsaKey, nil
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, ErrInvalidAgentPublicKey
		}
		rsaKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, ErrInvalidAgentPublicKey
		}
		return rsaKey, nil
	default:
		// Attempt PKIX parse for compatibility.
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, ErrInvalidAgentPublicKey
		}
		rsaKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, ErrInvalidAgentPublicKey
		}
		return rsaKey, nil
	}
}

func decodeBase64Any(input string) ([]byte, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty base64 input")
	}

	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}

	var lastErr error
	for _, decoder := range decoders {
		decoded, err := decoder(input)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unable to decode base64 input")
	}
	return nil, lastErr
}
