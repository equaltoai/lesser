package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAgentChallengeSignature_Ed25519_PEM(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	message := "LESSER test message"
	sig := ed25519.Sign(priv, []byte(message))

	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	err = VerifyAgentChallengeSignature("ed25519", string(pubPEM), message, base64.StdEncoding.EncodeToString(sig))
	require.NoError(t, err)
}

func TestVerifyAgentChallengeSignature_Ed25519_RawBase64(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	message := "LESSER test message"
	sig := ed25519.Sign(priv, []byte(message))

	err = VerifyAgentChallengeSignature("ed25519", base64.StdEncoding.EncodeToString(pub), message, base64.StdEncoding.EncodeToString(sig))
	require.NoError(t, err)
}

func TestVerifyAgentChallengeSignature_RSA_PKIX(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	message := "LESSER test message"
	sum := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	err = VerifyAgentChallengeSignature("rsa", string(pubPEM), message, base64.StdEncoding.EncodeToString(sig))
	require.NoError(t, err)
}

func TestVerifyAgentChallengeSignature_Invalid(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	err = VerifyAgentChallengeSignature("ed25519", string(pubPEM), "message", base64.StdEncoding.EncodeToString([]byte("nope")))
	require.Error(t, err)
}

func TestParseAgentPublicKey_ValidationErrors(t *testing.T) {
	t.Run("empty key rejected", func(t *testing.T) {
		_, err := ParseAgentPublicKey("ed25519", "")
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})

	t.Run("unsupported key type rejected", func(t *testing.T) {
		_, err := ParseAgentPublicKey("ecdsa", "anything")
		assert.ErrorIs(t, err, ErrUnsupportedAgentKeyType)
	})
}

func TestParseAgentPublicKey_Ed25519_InvalidInputs(t *testing.T) {
	t.Run("raw base64 invalid", func(t *testing.T) {
		_, err := ParseAgentPublicKey("ed25519", "%%%not-base64%%%")
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})

	t.Run("raw base64 wrong length", func(t *testing.T) {
		_, err := ParseAgentPublicKey("ed25519", base64.StdEncoding.EncodeToString([]byte("short")))
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})

	t.Run("pem decode fails", func(t *testing.T) {
		_, err := ParseAgentPublicKey("ed25519", "-----BEGIN PUBLIC KEY-----\nthis is not pem\n-----END PUBLIC KEY-----")
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})

	t.Run("pem is not ed25519 key", func(t *testing.T) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		require.NoError(t, err)
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

		_, err = ParseAgentPublicKey("ed25519", string(pubPEM))
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})
}

func TestParseAgentPublicKey_RSA_PKCS1AndFallbackBranches(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	t.Run("pkcs1 rsa public key", func(t *testing.T) {
		der := x509.MarshalPKCS1PublicKey(&priv.PublicKey)
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: der})

		pub, err := ParseAgentPublicKey("rsa", string(pubPEM))
		require.NoError(t, err)
		_, ok := pub.(*rsa.PublicKey)
		require.True(t, ok)
	})

	t.Run("pkix public key with nonstandard pem type triggers fallback", func(t *testing.T) {
		der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		require.NoError(t, err)
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "SOME PUBLIC KEY", Bytes: der})

		pub, err := ParseAgentPublicKey("rsa", string(pubPEM))
		require.NoError(t, err)
		_, ok := pub.(*rsa.PublicKey)
		require.True(t, ok)
	})

	t.Run("invalid pem rejected", func(t *testing.T) {
		_, err := ParseAgentPublicKey("rsa", "not a pem")
		assert.ErrorIs(t, err, ErrInvalidAgentPublicKey)
	})
}

func TestVerifyAgentChallengeSignature_RSA_PSS(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	message := "LESSER test message"
	sum := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPSS(rand.Reader, priv, crypto.SHA256, sum[:], nil)
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	err = VerifyAgentChallengeSignature("rsa", string(pubPEM), message, base64.StdEncoding.EncodeToString(sig))
	require.NoError(t, err)
}

func TestVerifyAgentChallengeSignature_InvalidSignatureEncoding(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	err = VerifyAgentChallengeSignature("ed25519", string(pubPEM), "message", "!!!notbase64!!!")
	assert.ErrorIs(t, err, ErrInvalidAgentSignature)
}

func TestDecodeBase64Any_Coverage(t *testing.T) {
	t.Run("empty rejects", func(t *testing.T) {
		_, err := decodeBase64Any(" ")
		require.Error(t, err)
	})

	t.Run("raw std encoding supported", func(t *testing.T) {
		raw := base64.RawStdEncoding.EncodeToString([]byte("hello"))
		out, err := decodeBase64Any(raw)
		require.NoError(t, err)
		require.Equal(t, []byte("hello"), out)
	})

	t.Run("raw url encoding supported", func(t *testing.T) {
		raw := base64.RawURLEncoding.EncodeToString([]byte("hello"))
		out, err := decodeBase64Any(raw)
		require.NoError(t, err)
		require.Equal(t, []byte("hello"), out)
	})
}
