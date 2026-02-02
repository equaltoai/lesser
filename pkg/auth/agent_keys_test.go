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
