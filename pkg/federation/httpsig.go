package federation

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

const (
	// SignatureHeader is the HTTP header containing the signature
	SignatureHeader = "Signature"

	// SignatureInputHeader is the new draft header for signature input
	SignatureInputHeader = "Signature-Input"

	// DigestHeader is the HTTP header containing the body digest
	DigestHeader = "Digest"

	// DateHeader is the HTTP header containing the request date
	DateHeader = "Date"

	// DefaultAlgorithm is the default signature algorithm
	DefaultAlgorithm = "rsa-sha256"

	// MaxClockSkew is the maximum clock skew allowed (5 minutes)
	MaxClockSkew = 5 * time.Minute

	// RequestTargetHeader is the pseudo-header for request target
	RequestTargetHeader = "(request-target)"
)

// Supported algorithms
const (
	AlgorithmHS2019      = "hs2019"       // Recommended in newer drafts
	AlgorithmRSASHA256   = "rsa-sha256"   // Legacy RSA
	AlgorithmRSASHA512   = "rsa-sha512"   // RSA with SHA-512
	AlgorithmECDSASHA256 = "ecdsa-sha256" // ECDSA support
	AlgorithmEd25519     = "ed25519"      // EdDSA support
)

// HTTPSignature represents a parsed HTTP signature
type HTTPSignature struct {
	KeyID     string
	Algorithm string
	Headers   []string
	Signature []byte
}

// ParseSignatureHeader parses the Signature header according to draft-cavage-http-signatures-12
func ParseSignatureHeader(header string) (*HTTPSignature, error) {
	// Validate signature header format using centralized validation
	if err := common.ValidateActivityPubSignature(header); err != nil {
		common.Logger().Error("invalid signature header format", zap.Error(err))
		return nil, errors.Join(ErrInvalidSignatureHeaderFormatWrapper, err)
	}

	sig := &HTTPSignature{}

	// Parse key-value pairs from the header
	parts := strings.Split(header, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return nil, ErrInvalidSignatureHeaderFormat
		}

		key := strings.TrimSpace(kv[0])
		value := strings.Trim(strings.TrimSpace(kv[1]), "\"")

		switch key {
		case "keyId":
			sig.KeyID = value
		case "algorithm":
			sig.Algorithm = value
		case "headers":
			sig.Headers = strings.Fields(value)
		case "signature":
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				common.Logger().Error("failed to decode base64 signature", zap.Error(err), zap.String("signature", value))
				return nil, errors.Join(ErrDecodeSignatureFailed, err)
			}
			sig.Signature = decoded
		}
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("keyId", sig.KeyID); err != nil {
		return nil, ErrMissingKeyID
	}
	if err := common.ValidateSliceNotEmpty("signature", sig.Signature); err != nil {
		return nil, ErrMissingSignatureValue
	}
	if err := common.ValidateRequiredParam("algorithm", sig.Algorithm); err != nil {
		sig.Algorithm = DefaultAlgorithm
	}
	if err := common.ValidateSliceNotEmpty("headers", sig.Headers); err != nil {
		sig.Headers = []string{"date"}
	}

	return sig, nil
}

// buildSignatureString builds the signature string from the request and headers list
func buildSignatureString(req *http.Request, headers []string) (string, error) {
	parts := make([]string, 0, len(headers))

	for _, header := range headers {
		var value string

		switch header {
		case RequestTargetHeader:
			value = fmt.Sprintf("%s %s", strings.ToLower(req.Method), req.URL.Path)
			if req.URL.RawQuery != "" {
				value += "?" + req.URL.RawQuery
			}
		case "host":
			value = req.Host
			if err := common.ValidateRequiredParam("host", value); err != nil {
				value = req.URL.Host
			}
		default:
			// Get header value (case-insensitive)
			value = req.Header.Get(header)
			if err := common.ValidateRequiredParam(fmt.Sprintf("header_%s", header), value); err != nil {
				common.Logger().Error("required header not found", zap.String("header", header))
				return "", errors.Join(ErrRequiredHeaderNotFound, err)
			}
		}

		parts = append(parts, fmt.Sprintf("%s: %s", header, value))
	}

	return strings.Join(parts, "\n"), nil
}

// BuildHTTPSignatureString exposes the legacy HTTP signature canonical string for tests and diagnostics.
func BuildHTTPSignatureString(req *http.Request, headers []string) (string, error) {
	return buildSignatureString(req, headers)
}

// verifyTimestamp checks if the request date is within acceptable range
func verifyTimestamp(dateStr string) error {
	if err := common.ValidateRequiredParam("date_header", dateStr); err != nil {
		return common.AuthenticationError{Message: "missing date header"}
	}

	// Try parsing various date formats
	var requestTime time.Time
	var err error

	// Try RFC1123 format first (most common)
	requestTime, err = time.Parse(time.RFC1123, dateStr)
	if err != nil {
		// Try RFC850 format
		requestTime, err = time.Parse(time.RFC850, dateStr)
		if err != nil {
			// Try ANSIC format
			requestTime, err = time.Parse(time.ANSIC, dateStr)
			if err != nil {
				return common.AuthenticationError{Message: "invalid date format"}
			}
		}
	}

	// Check if timestamp is within acceptable range
	now := time.Now()
	diff := now.Sub(requestTime)
	if err := common.ValidateFloatRange("clock_skew", diff.Seconds(), -MaxClockSkew.Seconds(), MaxClockSkew.Seconds()); err != nil {
		common.Logger().Error("request timestamp out of acceptable range", zap.Duration("diff", diff), zap.Duration("max_skew", MaxClockSkew))
		return common.AuthenticationError{Message: "request timestamp out of range"}
	}

	return nil
}

// calculateDigest calculates the SHA-256 digest of the request body
func calculateDigest(body []byte) string {
	hash := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(hash[:])
}

// ParsePublicKeyPEM parses a PEM-encoded public key
func ParsePublicKeyPEM(pemData []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, ErrFailedToParsePEMBlock
	}

	switch block.Type {
	case "PUBLIC KEY":
		return x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		common.Logger().Error("unsupported public key type", zap.String("type", block.Type))
		return nil, errors.Join(ErrUnsupportedKeyType, errors.New(block.Type))
	}
}

// ParsePrivateKeyPEM parses a PEM-encoded private key
func ParsePrivateKeyPEM(pemData []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, ErrFailedToParsePEMBlock
	}

	switch block.Type {
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		common.Logger().Error("unsupported private key type", zap.String("type", block.Type))
		return nil, errors.Join(ErrUnsupportedKeyType, errors.New(block.Type))
	}
}

// VerifyHTTPSignature verifies an incoming HTTP request's signature
func VerifyHTTPSignature(req *http.Request, publicKey crypto.PublicKey) error {
	log := common.Logger()

	// Parse signature header
	sigHeader := req.Header.Get(SignatureHeader)
	sig, err := ParseSignatureHeader(sigHeader)
	if err != nil {
		common.Logger().Error("failed to parse signature header", zap.Error(err))
		return errors.Join(ErrSignatureParseFailed, err)
	}

	// Verify timestamp if date header is included
	for _, header := range sig.Headers {
		if header == "date" {
			if err := verifyTimestamp(req.Header.Get(DateHeader)); err != nil {
				return err
			}
			break
		}
	}

	// Use enhanced verification for all algorithms
	if err := VerifyHTTPSignatureEnhanced(req, publicKey, sig); err != nil {
		return err
	}

	log.Info("verified HTTP signature",
		zap.String("key_id", sig.KeyID),
		zap.String("algorithm", sig.Algorithm),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path))

	return nil
}

// SignHTTPRequest signs an outgoing HTTP request
func SignHTTPRequest(req *http.Request, privateKey crypto.PrivateKey, keyID string) error {
	// Set date header if not present
	if err := common.ValidateRequiredParam("date_header", req.Header.Get(DateHeader)); err != nil {
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
	}

	// Calculate and set digest header if there's a body
	if req.Body != nil && req.ContentLength != 0 {
		// Read body
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			common.Logger().Error("failed to read request body", zap.Error(err))
			return errors.Join(ErrReadRequestBodyFailed, err)
		}

		// Reset body for future reads
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Calculate and set digest
		digest := calculateDigest(bodyBytes)
		req.Header.Set(DigestHeader, digest)
	}

	// Use enhanced signing for better algorithm support
	algorithm := DetermineSigningAlgorithm(privateKey, true) // Use legacy for max compatibility

	return SignHTTPRequestWithAlgorithm(req, privateKey, keyID, algorithm)
}

// GenerateRSAKeyPair generates a new RSA key pair
func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, error) {
	if bits < 2048 {
		return nil, ErrKeySizeTooSmall
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		common.Logger().Error("failed to generate RSA key pair", zap.Error(err), zap.Int("bits", bits))
		return nil, errors.Join(ErrRSAKeyGenFailed, err)
	}

	return privateKey, nil
}

// EncodePublicKeyPEM encodes an RSA public key to PEM format
func EncodePublicKeyPEM(publicKey *rsa.PublicKey) ([]byte, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		common.Logger().Error("failed to marshal public key", zap.Error(err))
		return nil, errors.Join(ErrMarshalPublicKeyFailed, err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return publicKeyPEM, nil
}

// EncodePrivateKeyPEM encodes an RSA private key to PEM format
func EncodePrivateKeyPEM(privateKey *rsa.PrivateKey) ([]byte, error) {
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		common.Logger().Error("failed to marshal private key", zap.Error(err))
		return nil, errors.Join(ErrMarshalPrivateKeyFailed, err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return privateKeyPEM, nil
}

// VerifyDigest verifies the digest header against the request body
func VerifyDigest(req *http.Request, body []byte) error {
	digestHeader := req.Header.Get(DigestHeader)
	if err := common.ValidateRequiredParam("digest_header", digestHeader); err != nil {
		return common.AuthenticationError{Message: "missing digest header"}
	}

	// Parse digest header (format: "SHA-256=base64hash")
	parts := strings.SplitN(digestHeader, "=", 2)
	if len(parts) != 2 {
		return common.AuthenticationError{Message: "invalid digest header format"}
	}

	algorithm := parts[0]
	expectedDigest := parts[1]

	// Only support SHA-256 for now
	if algorithm != "SHA-256" {
		common.Logger().Error("unsupported digest algorithm", zap.String("algorithm", algorithm))
		return common.AuthenticationError{Message: "unsupported digest algorithm"}
	}

	// Calculate actual digest
	actualDigest := calculateDigest(body)
	actualDigestValue := strings.SplitN(actualDigest, "=", 2)[1]

	// Compare digests
	if actualDigestValue != expectedDigest {
		return common.AuthenticationError{Message: "digest mismatch"}
	}

	return nil
}
