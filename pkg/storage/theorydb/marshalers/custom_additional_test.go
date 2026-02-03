package marshalers

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingEncryptor struct{}

func (failingEncryptor) Encrypt([]byte) ([]byte, error) { return nil, fmt.Errorf("encrypt failed") }
func (failingEncryptor) Decrypt([]byte) ([]byte, error) { return nil, fmt.Errorf("decrypt failed") }

func TestMoney_AdditionalHelpersAndErrors(t *testing.T) {
	m := NewMoneyFromFloat(0, "USD")
	assert.True(t, m.IsZero())
	assert.Equal(t, "0.00 USD", m.String())

	usd := NewMoney(decimal.NewFromInt(1), "USD")
	eur := NewMoney(decimal.NewFromInt(1), "EUR")

	_, err := usd.Add(eur)
	assert.Error(t, err)

	_, err = usd.Sub(eur)
	assert.Error(t, err)
}

func TestAESEncryptor_NewAESEncryptor_FromConfig(t *testing.T) {
	t.Run("missing env rejected", func(t *testing.T) {
		t.Setenv("DYNAMODB_ENCRYPTION_KEY", "")
		config.ResetForTests()
		_, err := NewAESEncryptor()
		assert.Error(t, err)
	})

	t.Run("invalid base64 rejected", func(t *testing.T) {
		t.Setenv("DYNAMODB_ENCRYPTION_KEY", "not-base64")
		config.ResetForTests()
		_, err := NewAESEncryptor()
		assert.Error(t, err)
	})

	t.Run("wrong key length rejected", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString(make([]byte, 16))
		t.Setenv("DYNAMODB_ENCRYPTION_KEY", short)
		config.ResetForTests()
		_, err := NewAESEncryptor()
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		key := base64.StdEncoding.EncodeToString(make([]byte, 32))
		t.Setenv("DYNAMODB_ENCRYPTION_KEY", key)
		config.ResetForTests()
		enc, err := NewAESEncryptor()
		require.NoError(t, err)
		require.NotNil(t, enc)
	})
}

func TestAESEncryptor_EncryptDecrypt_ErrorBranches(t *testing.T) {
	t.Run("encrypt fails with invalid cipher key size", func(t *testing.T) {
		enc := &AESEncryptor{key: []byte("short")}
		_, err := enc.Encrypt([]byte("x"))
		assert.Error(t, err)
	})

	t.Run("decrypt rejects short ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		enc, err := NewAESEncryptorWithKey(key)
		require.NoError(t, err)
		_, err = enc.Decrypt([]byte{})
		assert.Error(t, err)
	})

	t.Run("decrypt fails for tampered ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		enc, err := NewAESEncryptorWithKey(key)
		require.NoError(t, err)
		ciphertext, err := enc.Encrypt([]byte("secret"))
		require.NoError(t, err)

		ciphertext[len(ciphertext)-1] ^= 0xff
		_, err = enc.Decrypt(ciphertext)
		assert.Error(t, err)
	})
}

func TestEncryptedString_MarshalUnmarshal_AndString(t *testing.T) {
	key := make([]byte, 32)
	enc, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	t.Run("marshal requires encryptor", func(t *testing.T) {
		_, err := (EncryptedString{Value: "x"}).MarshalDynamORM()
		assert.Error(t, err)
	})

	t.Run("marshal surfaces encrypt errors", func(t *testing.T) {
		es := NewEncryptedString("x", failingEncryptor{})
		_, err := es.MarshalDynamORM()
		assert.Error(t, err)
	})

	t.Run("marshal/unmarshal roundtrip", func(t *testing.T) {
		es := NewEncryptedString("hello", enc)
		raw, err := es.MarshalDynamORM()
		require.NoError(t, err)

		var decoded EncryptedString
		decoded.SetEncryptor(enc)
		require.NoError(t, decoded.UnmarshalDynamORM(raw))
		assert.Equal(t, "hello", decoded.Value)
		assert.Equal(t, "hello", decoded.String())
	})

	t.Run("unmarshal rejects wrong type", func(t *testing.T) {
		var decoded EncryptedString
		decoded.SetEncryptor(enc)
		assert.Error(t, decoded.UnmarshalDynamORM(123))
	})

	t.Run("unmarshal requires encryptor", func(t *testing.T) {
		var decoded EncryptedString
		assert.Error(t, decoded.UnmarshalDynamORM("aGVsbG8="))
	})

	t.Run("unmarshal rejects invalid base64", func(t *testing.T) {
		var decoded EncryptedString
		decoded.SetEncryptor(enc)
		assert.Error(t, decoded.UnmarshalDynamORM("not-base64"))
	})

	t.Run("unmarshal surfaces decrypt errors", func(t *testing.T) {
		var decoded EncryptedString
		decoded.SetEncryptor(failingEncryptor{})
		assert.Error(t, decoded.UnmarshalDynamORM(base64.StdEncoding.EncodeToString([]byte("nope"))))
	})
}

