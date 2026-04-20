package reputation

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewSigner_LoadsPKCS8Ed25519PrivateKey(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	signer, err := NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, signer)
	require.Equal(t, base64.StdEncoding.EncodeToString(publicKey), signer.GetPublicKeyBase64())
}

func TestNewSigner_LoadsRawEd25519PrivateKey(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: privateKey,
	})

	signer, err := NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, signer)
	require.Equal(t, base64.StdEncoding.EncodeToString(publicKey), signer.GetPublicKeyBase64())
}

func TestNewSigner_ReturnsErrorOnInvalidPEM(t *testing.T) {
	t.Parallel()

	_, err := NewSigner("not pem", "https://example.com", zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse PEM block containing private key")
}

func TestNewSigner_ReturnsErrorOnUnsupportedPEMType(t *testing.T) {
	t.Parallel()

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: []byte("unused"),
	})

	_, err := NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported PEM block type")
}

func TestNewSigner_ReturnsErrorOnEd25519PEMWrongSize(t *testing.T) {
	t.Parallel()

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: make([]byte, ed25519.PrivateKeySize-1),
	})

	_, err := NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid Ed25519 private key size")
}

func TestNewSigner_ReturnsErrorOnInvalidPKCS8Key(t *testing.T) {
	t.Parallel()

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("not-asn1"),
	})

	_, err := NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to parse PKCS#8 private key")
}

func TestNewSigner_ReturnsErrorOnNonEd25519PKCS8Key(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	_, err = NewSigner(string(privateKeyPEM), "https://example.com", zap.NewNop())
	require.Error(t, err)
	require.ErrorContains(t, err, "private key is not an Ed25519 key")
}
