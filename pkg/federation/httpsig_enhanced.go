package federation

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"go.uber.org/zap"
)

// Enhanced HTTPSignature with support for hs2019
type EnhancedHTTPSignature struct {
	HTTPSignature
	Created int64 // Unix timestamp (for hs2019)
	Expires int64 // Unix timestamp (for hs2019)
}

// VerifyHTTPSignatureV2 verifies signatures according to draft-ietf-httpbis-message-signatures
func VerifyHTTPSignatureV2(req *http.Request, publicKey crypto.PublicKey) error {
	// This is a simplified implementation of the new draft
	// Full implementation would require structured field parsing

	signatureInput := req.Header.Get(SignatureInputHeader)
	signature := req.Header.Get("Signature")

	if signatureInput == "" || signature == "" {
		// Fall back to legacy verification
		return VerifyHTTPSignature(req, publicKey)
	}

	// For now, fall back to legacy verification
	// A full implementation would parse structured fields according to the new draft
	return VerifyHTTPSignature(req, publicKey)
}

// VerifyHTTPSignatureEnhanced verifies signatures with support for multiple algorithms
func VerifyHTTPSignatureEnhanced(req *http.Request, publicKey crypto.PublicKey, sig *HTTPSignature) error {
	log := common.Logger()

	// Build signature string
	sigString, err := buildSignatureString(req, sig.Headers)
	if err != nil {
		return fmt.Errorf("failed to build signature string: %w", err)
	}

	// Verify the signature based on algorithm
	var verifyErr error
	switch sig.Algorithm {
	case AlgorithmHS2019:
		// hs2019 can use different algorithms based on key type
		verifyErr = verifyWithKey(sigString, sig.Signature, publicKey)

	case AlgorithmRSASHA256:
		rsaKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return common.AuthenticationError{Message: "public key is not RSA"}
		}
		hash := sha256.Sum256([]byte(sigString))
		verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig.Signature)

	case AlgorithmRSASHA512:
		rsaKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return common.AuthenticationError{Message: "public key is not RSA"}
		}
		hash := sha512.Sum512([]byte(sigString))
		verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA512, hash[:], sig.Signature)

	case AlgorithmECDSASHA256:
		ecdsaKey, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return common.AuthenticationError{Message: "public key is not ECDSA"}
		}
		hash := sha256.Sum256([]byte(sigString))
		valid := ecdsa.VerifyASN1(ecdsaKey, hash[:], sig.Signature)
		if !valid {
			verifyErr = fmt.Errorf("ECDSA signature verification failed")
		}

	case AlgorithmEd25519:
		ed25519Key, ok := publicKey.(ed25519.PublicKey)
		if !ok {
			return common.AuthenticationError{Message: "public key is not Ed25519"}
		}
		if !ed25519.Verify(ed25519Key, []byte(sigString), sig.Signature) {
			verifyErr = fmt.Errorf("Ed25519 signature verification failed")
		}

	default:
		// Fall back to original implementation for rsa-sha256
		if sig.Algorithm == "rsa-sha256" {
			rsaKey, ok := publicKey.(*rsa.PublicKey)
			if !ok {
				return common.AuthenticationError{Message: "public key is not RSA"}
			}
			hash := sha256.Sum256([]byte(sigString))
			verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig.Signature)
		} else {
			return common.AuthenticationError{Message: fmt.Sprintf("unsupported algorithm: %s", sig.Algorithm)}
		}
	}

	if verifyErr != nil {
		return common.AuthenticationError{Message: "signature verification failed"}
	}

	log.Info("verified HTTP signature",
		zap.String("key_id", sig.KeyID),
		zap.String("algorithm", sig.Algorithm),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path))

	return nil
}

// verifyWithKey verifies a signature with automatic algorithm detection based on key type
func verifyWithKey(sigString string, signature []byte, publicKey crypto.PublicKey) error {
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		hash := sha256.Sum256([]byte(sigString))
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature)

	case *ecdsa.PublicKey:
		hash := sha256.Sum256([]byte(sigString))
		if !ecdsa.VerifyASN1(key, hash[:], signature) {
			return fmt.Errorf("ECDSA signature verification failed")
		}
		return nil

	case ed25519.PublicKey:
		if !ed25519.Verify(key, []byte(sigString), signature) {
			return fmt.Errorf("Ed25519 signature verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported public key type for hs2019")
	}
}

// SignHTTPRequestWithAlgorithm signs a request with a specific algorithm
func SignHTTPRequestWithAlgorithm(req *http.Request, privateKey crypto.PrivateKey, keyID, algorithm string) error {
	log := common.Logger()

	// Set date header if not present
	if req.Header.Get(DateHeader) == "" {
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
	}

	// Determine headers to sign
	headers := []string{"(request-target)", "host", "date"}
	if req.Header.Get(DigestHeader) != "" {
		headers = append(headers, "digest")
	}
	if req.Header.Get("Content-Type") != "" {
		headers = append(headers, "content-type")
	}

	// Build signature string
	sigString, err := buildSignatureString(req, headers)
	if err != nil {
		return fmt.Errorf("failed to build signature string: %w", err)
	}

	// Sign the string based on algorithm
	var signature []byte
	switch algorithm {
	case AlgorithmHS2019:
		signature, err = signWithKey(sigString, privateKey)

	case AlgorithmRSASHA256:
		key, ok := privateKey.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("algorithm %s requires RSA key", algorithm)
		}
		hash := sha256.Sum256([]byte(sigString))
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])

	case AlgorithmRSASHA512:
		key, ok := privateKey.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("algorithm %s requires RSA key", algorithm)
		}
		hash := sha512.Sum512([]byte(sigString))
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, hash[:])

	case AlgorithmECDSASHA256:
		key, ok := privateKey.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("algorithm %s requires ECDSA key", algorithm)
		}
		hash := sha256.Sum256([]byte(sigString))
		signature, err = ecdsa.SignASN1(rand.Reader, key, hash[:])

	case AlgorithmEd25519:
		key, ok := privateKey.(ed25519.PrivateKey)
		if !ok {
			return fmt.Errorf("algorithm %s requires Ed25519 key", algorithm)
		}
		signature = ed25519.Sign(key, []byte(sigString))

	default:
		return fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	if err != nil {
		return fmt.Errorf("failed to sign: %w", err)
	}

	// Build signature header
	sigHeader := fmt.Sprintf(
		`keyId="%s",algorithm="%s",headers="%s",signature="%s"`,
		keyID,
		algorithm,
		strings.Join(headers, " "),
		base64.StdEncoding.EncodeToString(signature),
	)

	req.Header.Set(SignatureHeader, sigHeader)

	log.Debug("signed HTTP request",
		zap.String("key_id", keyID),
		zap.String("algorithm", algorithm),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path))

	return nil
}

// signWithKey signs with automatic algorithm detection based on key type
func signWithKey(sigString string, privateKey crypto.PrivateKey) ([]byte, error) {
	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		hash := sha256.Sum256([]byte(sigString))
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])

	case *ecdsa.PrivateKey:
		hash := sha256.Sum256([]byte(sigString))
		return ecdsa.SignASN1(rand.Reader, key, hash[:])

	case ed25519.PrivateKey:
		return ed25519.Sign(key, []byte(sigString)), nil

	default:
		return nil, fmt.Errorf("unsupported private key type for hs2019")
	}
}

// NegotiateSignatureAlgorithm selects the best algorithm based on preferences and key type
func NegotiateSignatureAlgorithm(acceptedAlgorithms []string, keyType string) string {
	// Order of preference
	preferences := []string{
		AlgorithmHS2019,      // Recommended
		AlgorithmRSASHA256,   // Legacy support
		AlgorithmRSASHA512,   // Stronger RSA
		AlgorithmECDSASHA256, // ECDSA support
		AlgorithmEd25519,     // EdDSA support
	}

	for _, pref := range preferences {
		for _, accepted := range acceptedAlgorithms {
			if pref == accepted && isCompatible(pref, keyType) {
				return pref
			}
		}
	}

	return AlgorithmRSASHA256 // Default fallback
}

// isCompatible checks if an algorithm is compatible with a key type
func isCompatible(algorithm, keyType string) bool {
	switch algorithm {
	case AlgorithmHS2019:
		return true // Works with all key types
	case AlgorithmRSASHA256, AlgorithmRSASHA512:
		return keyType == "RSA"
	case AlgorithmECDSASHA256:
		return keyType == "ECDSA"
	case AlgorithmEd25519:
		return keyType == "Ed25519"
	default:
		return false
	}
}

// getKeyType returns the type of a crypto key
func getKeyType(key crypto.PrivateKey) string {
	switch key.(type) {
	case *rsa.PrivateKey:
		return "RSA"
	case *ecdsa.PrivateKey:
		return "ECDSA"
	case ed25519.PrivateKey:
		return "Ed25519"
	default:
		return "unknown"
	}
}

// verifyHS2019Timestamp checks created/expires timestamps for hs2019
func verifyHS2019Timestamp(created, expires int64) error {
	now := time.Now().Unix()

	if created > 0 {
		// Check if created is not in the future (with clock skew)
		if created > now+int64(MaxClockSkew.Seconds()) {
			return common.AuthenticationError{Message: "signature created in the future"}
		}

		// Check if signature is not too old
		if created < now-int64(MaxClockSkew.Seconds()) {
			return common.AuthenticationError{Message: "signature too old"}
		}
	}

	if expires > 0 && expires < now {
		return common.AuthenticationError{Message: "signature expired"}
	}

	return nil
}
