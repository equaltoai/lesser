package federation

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNegotiateSignatureAlgorithm(t *testing.T) {
	assert.Equal(t, AlgorithmHS2019, NegotiateSignatureAlgorithm([]string{AlgorithmHS2019, AlgorithmRSASHA256}, keyTypeRSA))
	assert.Equal(t, AlgorithmRSASHA256, NegotiateSignatureAlgorithm([]string{AlgorithmRSASHA256, AlgorithmEd25519}, keyTypeRSA))
	assert.Equal(t, AlgorithmECDSASHA256, NegotiateSignatureAlgorithm([]string{AlgorithmRSASHA256, AlgorithmECDSASHA256}, keyTypeECDSA))
	assert.Equal(t, AlgorithmEd25519, NegotiateSignatureAlgorithm([]string{AlgorithmEd25519}, keyTypeEd25519))
	assert.Equal(t, AlgorithmRSASHA256, NegotiateSignatureAlgorithm([]string{"weird"}, keyTypeUnknown))
}

func TestIsCompatible(t *testing.T) {
	assert.True(t, isCompatible(AlgorithmHS2019, keyTypeUnknown))
	assert.True(t, isCompatible(AlgorithmRSASHA256, keyTypeRSA))
	assert.False(t, isCompatible(AlgorithmRSASHA256, keyTypeECDSA))
	assert.True(t, isCompatible(AlgorithmECDSASHA256, keyTypeECDSA))
	assert.False(t, isCompatible("weird", keyTypeRSA))
}

func TestExtractParameter(t *testing.T) {
	assert.Equal(t, "abc", extractParameter(`;keyid="abc";alg="hs2019"`, "keyid"))
	assert.Equal(t, "hs2019", extractParameter(`;keyid="abc";alg=hs2019`, "alg"))
	assert.Equal(t, "", extractParameter(`;keyid="abc"`, "missing"))
	assert.Equal(t, "", extractParameter(`;alg=`, "alg"))
}

func TestParseStructuredSignature(t *testing.T) {
	t.Run("invalid_format", func(t *testing.T) {
		_, err := parseStructuredSignature(`sig="@method";keyid="x"`, base64.StdEncoding.EncodeToString([]byte("test")))
		assert.ErrorIs(t, err, ErrInvalidSignatureInputFormat)
	})

	t.Run("bad_base64", func(t *testing.T) {
		_, err := parseStructuredSignature(`sig=("@method");keyid="https://example.com/users/alice#main-key"`, "!!!notbase64")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDecodeSignature)
	})

	t.Run("maps_headers_and_defaults_algorithm", func(t *testing.T) {
		sig, err := parseStructuredSignature(
			`sig=("@method" "@path" "host" "date");keyid="https://example.com/users/alice#main-key"`,
			base64.StdEncoding.EncodeToString([]byte("test")),
		)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/users/alice#main-key", sig.KeyID)
		assert.Equal(t, AlgorithmHS2019, sig.Algorithm)
		assert.Equal(t, []string{RequestTargetHeader, RequestTargetHeader, "host", "date"}, sig.Headers)
	})
}

func TestVerifyHTTPSignatureV2_FallsBackToLegacy(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	require.NoError(t, SignHTTPRequest(req, privateKey, "https://example.com/users/alice#main-key"))

	// Missing Signature-Input header should fall back to legacy verification.
	require.NoError(t, VerifyHTTPSignatureV2(req, &privateKey.PublicKey))

	// Invalid Signature-Input should also fall back to legacy verification.
	req.Header.Set(SignatureInputHeader, "not a structured field")
	require.NoError(t, VerifyHTTPSignatureV2(req, &privateKey.PublicKey))
}

func TestVerifyHTTPSignatureV2_StructuredSignature_Success(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox?x=1", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(http.TimeFormat))

	signatureInput := `sig=("@method" "@path" "host" "date");keyid="https://example.com/users/alice#main-key";alg="rsa-sha256"`
	headers := []string{RequestTargetHeader, RequestTargetHeader, "host", "date"}
	sigString, err := buildSignatureString(req, headers)
	require.NoError(t, err)

	hash := sha256.Sum256([]byte(sigString))
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	require.NoError(t, err)

	req.Header.Set(SignatureInputHeader, signatureInput)
	req.Header.Set("Signature", base64.StdEncoding.EncodeToString(sigBytes))

	require.NoError(t, VerifyHTTPSignatureV2(req, &privateKey.PublicKey))
}

func TestVerifyHTTPSignatureEnhanced_ECDSAAndEd25519(t *testing.T) {
	t.Run("ecdsa_sha256_success", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
		req.Host = "example.com"
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
		req.Header.Set("Content-Type", "application/activity+json")

		require.NoError(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmECDSASHA256))
		require.NoError(t, VerifyHTTPSignature(req, &privateKey.PublicKey))
	})

	t.Run("hs2019_ecdsa_success", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
		req.Host = "example.com"
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
		req.Header.Set("Content-Type", "application/activity+json")

		require.NoError(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmHS2019))
		require.NoError(t, VerifyHTTPSignature(req, &privateKey.PublicKey))
	})

	t.Run("ed25519_success", func(t *testing.T) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
		req.Host = "example.com"
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
		req.Header.Set("Content-Type", "application/activity+json")

		require.NoError(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmEd25519))
		require.NoError(t, VerifyHTTPSignature(req, publicKey))
	})

	t.Run("hs2019_ed25519_success", func(t *testing.T) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "https://example.com/inbox", strings.NewReader(`{"type":"Note"}`))
		req.Host = "example.com"
		req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))
		req.Header.Set("Content-Type", "application/activity+json")

		require.NoError(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmHS2019))
		require.NoError(t, VerifyHTTPSignature(req, publicKey))
	})
}

func TestSignAndVerifyAlgorithmMismatchErrors(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	assert.ErrorIs(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", "bogus"), ErrUnsupportedAlgorithm)
	assert.ErrorIs(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmECDSASHA256), ErrAlgorithmRequiresECDSA)
	assert.ErrorIs(t, SignHTTPRequestWithAlgorithm(req, privateKey, "https://example.com/users/alice#main-key", AlgorithmEd25519), ErrAlgorithmRequiresEd25519)

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	assert.ErrorIs(t, SignHTTPRequestWithAlgorithm(req, ecdsaKey, "https://example.com/users/alice#main-key", AlgorithmRSASHA256), ErrAlgorithmRequiresRSA)
}

func TestVerifyHTTPSignatureEnhanced_ErrorBranches(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	// Unsupported algorithm should surface from enhanced verification.
	req.Header.Set(SignatureHeader, `keyId="https://example.com/users/alice#main-key",algorithm="weird",headers="(request-target) host date",signature="dGVzdA=="`)
	assert.ErrorIs(t, VerifyHTTPSignature(req, &privateKey.PublicKey), ErrUnsupportedAlgorithm)

	// Algorithm/key mismatch returns a specific error before signature verification.
	req.Header.Set(SignatureHeader, `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha512",headers="(request-target) host date",signature="dGVzdA=="`)
	ecdsaPub, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	assert.ErrorIs(t, VerifyHTTPSignature(req, &ecdsaPub.PublicKey), ErrAlgorithmRequiresRSA)
}

func TestSignWithKey_UnsupportedPrivateKeyType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	err := SignHTTPRequestWithAlgorithm(req, struct{}{}, "https://example.com/users/alice#main-key", AlgorithmHS2019)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureFailed)
	assert.ErrorIs(t, err, ErrUnsupportedPrivateKeyType)
}

func TestVerifyWithKey_UnsupportedPublicKeyType(t *testing.T) {
	assert.ErrorIs(t, verifyWithKey("sig", []byte("sig"), struct{}{}), ErrUnsupportedPublicKeyType)
}

func TestDetectKeyType_Unknown(t *testing.T) {
	assert.Equal(t, keyTypeUnknown, DetectKeyType(struct{}{}))
}

func TestDetermineSigningAlgorithm_UnknownKeyTypeFallsBack(t *testing.T) {
	assert.Equal(t, AlgorithmRSASHA256, DetermineSigningAlgorithm(struct{}{}, false))
}

func TestVerifyHTTPSignatureEnhanced_Ed25519FailureReturnsSignatureFailed(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	// Signature bytes are not valid for this request/key.
	req.Header.Set(SignatureHeader, `keyId="https://example.com/users/alice#main-key",algorithm="ed25519",headers="(request-target) host date",signature="`+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 64))+`"`)
	assert.ErrorIs(t, VerifyHTTPSignature(req, publicKey), ErrSignatureFailed)
}

func TestVerifyHTTPSignatureEnhanced_RSASHA512InvalidSignature(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"
	req.Header.Set(DateHeader, time.Now().UTC().Format(time.RFC1123))

	req.Header.Set(SignatureHeader, `keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha512",headers="(request-target) host date",signature="dGVzdA=="`)
	assert.ErrorIs(t, VerifyHTTPSignature(req, &privateKey.PublicKey), ErrSignatureFailed)
}
