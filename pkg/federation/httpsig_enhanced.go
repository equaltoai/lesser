package federation

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

const (
	// Key type constants
	keyTypeRSA     = "RSA"
	keyTypeECDSA   = "ECDSA"
	keyTypeEd25519 = "Ed25519"
	keyTypeUnknown = "unknown"
)

// EnhancedHTTPSignature provides enhanced HTTP signature with support for hs2019
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

	// Parse structured signature according to HTTP Message Signatures draft-19
	sig, err := parseStructuredSignature(signatureInput, signature)
	if err != nil {
		// If parsing fails, fall back to legacy verification
		log := common.Logger()
		log.Debug("failed to parse structured signature, falling back to legacy",
			zap.Error(err))
		return VerifyHTTPSignature(req, publicKey)
	}

	// Verify using structured signature
	return VerifyHTTPSignatureEnhanced(req, publicKey, sig)
}

// VerifyHTTPSignatureEnhanced verifies signatures with support for multiple algorithms
func VerifyHTTPSignatureEnhanced(req *http.Request, publicKey crypto.PublicKey, sig *HTTPSignature) error {
	log := common.Logger()
	trace, _ := FollowTraceFromContext(req.Context())

	// Build signature string
	sigString, err := buildSignatureString(req, sig.Headers)
	if err != nil {
		if trace != nil {
			fields := append(FollowTraceFields(trace, "signature.verify.canonical_error"),
				zap.Bool("verification_algorithm_present", strings.TrimSpace(sig.Algorithm) != ""),
				zap.Bool("verification_key_id_present", strings.TrimSpace(sig.KeyID) != ""),
				zap.String("canonical_error", err.Error()),
			)
			log.Info("federation follow trace", fields...)
		}
		return errors.Join(ErrBuildSignatureString, err)
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
			log.Error("algorithm requires RSA key", zap.String("algorithm", AlgorithmRSASHA256))
			return ErrAlgorithmRequiresRSA
		}
		hash := sha256.Sum256([]byte(sigString))
		verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig.Signature)

	case AlgorithmRSASHA512:
		rsaKey, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			log.Error("algorithm requires RSA key", zap.String("algorithm", AlgorithmRSASHA512))
			return ErrAlgorithmRequiresRSA
		}
		hash := sha512.Sum512([]byte(sigString))
		verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA512, hash[:], sig.Signature)

	case AlgorithmECDSASHA256:
		ecdsaKey, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			log.Error("algorithm requires ECDSA key", zap.String("algorithm", AlgorithmECDSASHA256))
			return ErrAlgorithmRequiresECDSA
		}
		hash := sha256.Sum256([]byte(sigString))
		valid := ecdsa.VerifyASN1(ecdsaKey, hash[:], sig.Signature)
		if !valid {
			verifyErr = ErrECDSAVerificationFailed
		}

	case AlgorithmEd25519:
		ed25519Key, ok := publicKey.(ed25519.PublicKey)
		if !ok {
			log.Error("algorithm requires Ed25519 key", zap.String("algorithm", AlgorithmEd25519))
			return ErrAlgorithmRequiresEd25519
		}
		if !ed25519.Verify(ed25519Key, []byte(sigString), sig.Signature) {
			verifyErr = ErrEd25519VerificationFailed
		}

	default:
		// Fall back to original implementation for rsa-sha256
		if sig.Algorithm == DefaultAlgorithm {
			rsaKey, ok := publicKey.(*rsa.PublicKey)
			if !ok {
				log.Error("algorithm requires RSA key", zap.String("algorithm", DefaultAlgorithm))
				return ErrAlgorithmRequiresRSA
			}
			hash := sha256.Sum256([]byte(sigString))
			verifyErr = rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig.Signature)
		} else {
			log.Error("unsupported signature algorithm",
				zap.Bool("algorithm_present", strings.TrimSpace(sig.Algorithm) != ""),
				zap.Int("algorithm_len", len(strings.TrimSpace(sig.Algorithm))))
			return ErrUnsupportedAlgorithm
		}
	}

	if verifyErr != nil {
		if trace != nil {
			fields := append(FollowTraceFields(trace, "signature.verify.crypto"),
				zap.Bool("verification_algorithm_present", strings.TrimSpace(sig.Algorithm) != ""),
				zap.Bool("verification_key_id_present", strings.TrimSpace(sig.KeyID) != ""),
				zap.Int("signature_canonical_len", len(sigString)),
				zap.String("verification_error_detail", verifyErr.Error()),
			)
			log.Info("federation follow trace", fields...)
		}
		return ErrSignatureFailed
	}

	fields := appendTraceHeaderState([]zap.Field{
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
	}, "key_id", sig.KeyID)
	fields = appendTraceHeaderState(fields, "algorithm", sig.Algorithm)
	log.Info("verified HTTP signature", fields...)

	return nil
}

// BuildSignedActivityPubRequest creates a signed ActivityPub delivery request using
// Lesser's compatibility defaults for federation interoperability.
func BuildSignedActivityPubRequest(ctx context.Context, method, targetURL, userAgent string, body []byte, privateKey crypto.PrivateKey, keyID string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	if err := SignActivityPubRequest(req, privateKey, keyID); err != nil {
		return req, err
	}

	if trace, ok := FollowTraceFromContext(ctx); ok {
		fields := append(FollowTraceFields(trace, "sender.signed_request"),
			zap.String("signing_public_key_id", keyID),
		)
		fields = append(fields, BuildSignatureTraceFields(req)...)
		common.Logger().Info("federation follow trace", fields...)
	}

	return req, nil
}

// SignActivityPubRequest signs an outbound ActivityPub delivery request using
// Lesser's compatibility defaults for federation interoperability.
func SignActivityPubRequest(req *http.Request, privateKey crypto.PrivateKey, keyID string) error {
	return SignHTTPRequest(req, privateKey, keyID)
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
			return ErrECDSAVerificationFailed
		}
		return nil

	case ed25519.PublicKey:
		if !ed25519.Verify(key, []byte(sigString), signature) {
			return ErrEd25519VerificationFailed
		}
		return nil

	default:
		return ErrUnsupportedPublicKeyType
	}
}

// SignHTTPRequestWithAlgorithm signs a request with a specific algorithm
func SignHTTPRequestWithAlgorithm(req *http.Request, privateKey crypto.PrivateKey, keyID, algorithm string) error {
	log := common.Logger()

	// Set date header if not present
	if err := common.ValidateRequiredParam("req.Header.Get(DateHeader)", req.Header.Get(DateHeader)); err != nil {
		req.Header.Set(DateHeader, time.Now().UTC().Format(http.TimeFormat))
	}

	// Determine headers to sign
	headers := []string{RequestTargetHeader, "host", "date"}
	if req.Header.Get(DigestHeader) != "" {
		headers = append(headers, "digest")
	}
	if req.Header.Get("Content-Type") != "" {
		headers = append(headers, "content-type")
	}

	// Build signature string
	sigString, err := buildSignatureString(req, headers)
	if err != nil {
		return errors.Join(ErrBuildSignatureString, err)
	}

	// Sign the string based on algorithm
	var signature []byte
	switch algorithm {
	case AlgorithmHS2019:
		signature, err = signWithKey(sigString, privateKey)

	case AlgorithmRSASHA256:
		key, ok := privateKey.(*rsa.PrivateKey)
		if !ok {
			log.Error("algorithm requires RSA key", zap.String("algorithm", algorithm))
			return ErrAlgorithmRequiresRSA
		}
		hash := sha256.Sum256([]byte(sigString))
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])

	case AlgorithmRSASHA512:
		key, ok := privateKey.(*rsa.PrivateKey)
		if !ok {
			log.Error("algorithm requires RSA key", zap.String("algorithm", algorithm))
			return ErrAlgorithmRequiresRSA
		}
		hash := sha512.Sum512([]byte(sigString))
		signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, hash[:])

	case AlgorithmECDSASHA256:
		key, ok := privateKey.(*ecdsa.PrivateKey)
		if !ok {
			log.Error("algorithm requires ECDSA key", zap.String("algorithm", algorithm))
			return ErrAlgorithmRequiresECDSA
		}
		hash := sha256.Sum256([]byte(sigString))
		signature, err = ecdsa.SignASN1(rand.Reader, key, hash[:])

	case AlgorithmEd25519:
		key, ok := privateKey.(ed25519.PrivateKey)
		if !ok {
			log.Error("algorithm requires Ed25519 key", zap.String("algorithm", algorithm))
			return ErrAlgorithmRequiresEd25519
		}
		signature = ed25519.Sign(key, []byte(sigString))

	default:
		log.Error("unsupported signature algorithm", zap.String("algorithm", algorithm))
		return ErrUnsupportedAlgorithm
	}

	if err != nil {
		return errors.Join(ErrSignatureFailed, err)
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
		return nil, ErrUnsupportedPrivateKeyType
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
		return keyType == keyTypeRSA
	case AlgorithmECDSASHA256:
		return keyType == keyTypeECDSA
	case AlgorithmEd25519:
		return keyType == keyTypeEd25519
	default:
		return false
	}
}

// parseStructuredSignature parses structured signature fields from draft-19
func parseStructuredSignature(signatureInput, signature string) (*HTTPSignature, error) {
	// Parse signature-input structured field
	// Format: sig=("@method" "@path" "host" "date");keyid="...";alg="...";created=...

	// Find the signature parameters within parentheses
	startParen := strings.Index(signatureInput, "(")
	endParen := strings.Index(signatureInput, ")")
	if startParen == -1 || endParen == -1 {
		return nil, ErrInvalidSignatureInputFormat
	}

	// Extract headers from within parentheses
	headersStr := signatureInput[startParen+1 : endParen]
	headerParts := strings.Fields(headersStr)
	headers := make([]string, len(headerParts))
	for i, h := range headerParts {
		// Remove quotes and convert draft-19 format to legacy format
		h = strings.Trim(h, `"`)
		if h == "@method" {
			h = RequestTargetHeader
		} else if h == "@path" {
			h = RequestTargetHeader // Simplified mapping
		} else if strings.HasPrefix(h, "@") {
			// Map other draft-19 pseudo-headers as needed
			h = strings.TrimPrefix(h, "@")
		}
		headers[i] = h
	}

	// Parse parameters after the parentheses
	params := signatureInput[endParen+1:]

	// Extract keyid, algorithm, and other parameters
	keyID := extractParameter(params, "keyid")
	algorithm := extractParameter(params, "alg")
	if err := common.ValidateRequiredParam("algorithm", algorithm); err != nil {
		algorithm = AlgorithmHS2019 // Default
	}

	// Parse signature value (base64 encoded)
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return nil, errors.Join(ErrDecodeSignature, err)
	}

	return &HTTPSignature{
		KeyID:     keyID,
		Algorithm: algorithm,
		Headers:   headers,
		Signature: sigBytes,
	}, nil
}

// extractParameter extracts a parameter value from structured field parameters
func extractParameter(params, name string) string {
	// Look for name="value" pattern
	pattern := name + "="
	start := strings.Index(params, pattern)
	if start == -1 {
		return ""
	}

	start += len(pattern)
	if start >= len(params) {
		return ""
	}

	// Handle quoted values
	if params[start] == '"' {
		start++
		end := strings.Index(params[start:], `"`)
		if end == -1 {
			return ""
		}
		return params[start : start+end]
	}

	// Handle unquoted values (up to semicolon or end)
	end := strings.Index(params[start:], ";")
	if end == -1 {
		return strings.TrimSpace(params[start:])
	}
	return strings.TrimSpace(params[start : start+end])
}

// getKeyType returns the type of a crypto key

// DetermineSigningAlgorithm determines the best signing algorithm based on key type and compatibility
func DetermineSigningAlgorithm(privateKey crypto.PrivateKey, preferLegacy bool) string {
	if preferLegacy {
		// For maximum compatibility, use rsa-sha256 if the key supports it
		if _, ok := privateKey.(*rsa.PrivateKey); ok {
			return AlgorithmRSASHA256
		}
	}

	// Use modern algorithm selection
	switch privateKey.(type) {
	case *rsa.PrivateKey:
		return AlgorithmHS2019
	case *ecdsa.PrivateKey:
		return AlgorithmHS2019
	case ed25519.PrivateKey:
		return AlgorithmHS2019
	default:
		return AlgorithmRSASHA256
	}
}

// DetectKeyType detects the type of a crypto key (public or private)
func DetectKeyType(key interface{}) string {
	switch key.(type) {
	case *rsa.PublicKey, *rsa.PrivateKey:
		return keyTypeRSA
	case *ecdsa.PublicKey, *ecdsa.PrivateKey:
		return keyTypeECDSA
	case ed25519.PublicKey, ed25519.PrivateKey:
		return keyTypeEd25519
	default:
		return keyTypeUnknown
	}
}
