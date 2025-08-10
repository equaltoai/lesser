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
	sig := &HTTPSignature{}

	// Parse key-value pairs from the header
	parts := strings.Split(header, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid signature header format")
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
				return nil, fmt.Errorf("failed to decode signature: %w", err)
			}
			sig.Signature = decoded
		}
	}

	// Validate required fields
	if sig.KeyID == "" {
		return nil, fmt.Errorf("missing keyId in signature")
	}
	if len(sig.Signature) == 0 {
		return nil, fmt.Errorf("missing signature value")
	}
	if sig.Algorithm == "" {
		sig.Algorithm = DefaultAlgorithm
	}
	if len(sig.Headers) == 0 {
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
			if value == "" {
				value = req.URL.Host
			}
		default:
			// Get header value (case-insensitive)
			value = req.Header.Get(header)
			if value == "" {
				return "", fmt.Errorf("required header '%s' not found", header)
			}
		}

		parts = append(parts, fmt.Sprintf("%s: %s", header, value))
	}

	return strings.Join(parts, "\n"), nil
}

// verifyTimestamp checks if the request date is within acceptable range
func verifyTimestamp(dateStr string) error {
	if dateStr == "" {
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
	if diff < -MaxClockSkew || diff > MaxClockSkew {
		return common.AuthenticationError{Message: fmt.Sprintf("request timestamp out of range: %v", diff)}
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
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	switch block.Type {
	case "PUBLIC KEY":
		return x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}

// ParsePrivateKeyPEM parses a PEM-encoded private key
func ParsePrivateKeyPEM(pemData []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	switch block.Type {
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}

// VerifyHTTPSignature verifies an incoming HTTP request's signature
func VerifyHTTPSignature(req *http.Request, publicKey crypto.PublicKey) error {
	log := common.Logger()

	// Parse signature header
	sigHeader := req.Header.Get(SignatureHeader)
	if sigHeader == "" {
		return common.AuthenticationError{Message: "missing signature header"}
	}

	sig, err := ParseSignatureHeader(sigHeader)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
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
	if req.Header.Get(DateHeader) == "" {
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
	}

	// Calculate and set digest header if there's a body
	if req.Body != nil && req.ContentLength != 0 {
		// Read body
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
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
		return nil, fmt.Errorf("key size must be at least 2048 bits")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key pair: %w", err)
	}

	return privateKey, nil
}

// EncodePublicKeyPEM encodes an RSA public key to PEM format
func EncodePublicKeyPEM(publicKey *rsa.PublicKey) ([]byte, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
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
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
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
	if digestHeader == "" {
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
		return common.AuthenticationError{Message: fmt.Sprintf("unsupported digest algorithm: %s", algorithm)}
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
