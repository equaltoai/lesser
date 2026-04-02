package services

import (
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCryptoAdapter_RSAKeyPairAndPEMEncoding(t *testing.T) {
	adapter := NewCryptoAdapter()

	privateKeyAny, err := adapter.GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	privateKey, ok := privateKeyAny.(*rsa.PrivateKey)
	require.True(t, ok)

	publicPEM, err := adapter.EncodePublicKeyPEM(privateKeyAny)
	require.NoError(t, err)
	require.NotEmpty(t, publicPEM)

	publicPEM, err = adapter.EncodePublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)
	require.NotEmpty(t, publicPEM)

	privatePEM, err := adapter.EncodePrivateKeyPEM(privateKeyAny)
	require.NoError(t, err)
	require.NotEmpty(t, privatePEM)

	_, err = adapter.EncodePublicKeyPEM("not-a-key")
	require.Error(t, err)

	_, err = adapter.EncodePrivateKeyPEM(&privateKey.PublicKey)
	require.Error(t, err)
}

func TestAuthAdapter_DelegatesToAuthHelpers(t *testing.T) {
	adapter := NewAuthAdapter("jwt-secret", nil)
	password := "Moss!River72"

	hash, err := adapter.HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.NotEqual(t, password, hash)

	require.NoError(t, adapter.ValidatePassword(password, "alice"))
	require.Greater(t, adapter.PasswordStrength(password), 0)
}
