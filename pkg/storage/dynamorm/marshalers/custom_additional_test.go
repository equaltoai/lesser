package marshalers

import (
	"encoding/json"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubEncryptor struct {
	encryptErr error
	decryptErr error

	encrypted []byte
	decrypted []byte
}

func (s stubEncryptor) Encrypt(_ []byte) ([]byte, error) {
	if s.encryptErr != nil {
		return nil, s.encryptErr
	}
	return s.encrypted, nil
}

func (s stubEncryptor) Decrypt(_ []byte) ([]byte, error) {
	if s.decryptErr != nil {
		return nil, s.decryptErr
	}
	return s.decrypted, nil
}

func TestPreciseTime_UnmarshalPrecisionTypes(t *testing.T) {
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC).Format(time.RFC3339Nano)

	t.Run("float64 precision", func(t *testing.T) {
		var pt PreciseTime
		err := pt.UnmarshalDynamORM(map[string]any{
			"timestamp": ts,
			"precision": float64(time.Millisecond),
		})
		require.NoError(t, err)
		assert.Equal(t, time.Millisecond, pt.Precision)
	})

	t.Run("string precision", func(t *testing.T) {
		var pt PreciseTime
		err := pt.UnmarshalDynamORM(map[string]any{
			"timestamp": ts,
			"precision": "1000000",
		})
		require.NoError(t, err)
		assert.Equal(t, time.Millisecond, pt.Precision)
	})

	t.Run("unsupported precision type", func(t *testing.T) {
		var pt PreciseTime
		err := pt.UnmarshalDynamORM(map[string]any{
			"timestamp": ts,
			"precision": true,
		})
		require.Error(t, err)
	})
}

func TestMoney_MarshalRequiresCurrency(t *testing.T) {
	m := NewMoney(decimal.NewFromInt(1), "")
	_, err := m.MarshalDynamORM()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currency is required")
}

func TestMoney_AddSubCurrencyMismatch(t *testing.T) {
	a := NewMoney(decimal.NewFromInt(1), "USD")
	b := NewMoney(decimal.NewFromInt(1), "EUR")

	_, err := a.Add(b)
	require.Error(t, err)

	_, err = a.Sub(b)
	require.Error(t, err)
}

func TestNewMoneyFromString_InvalidAmount(t *testing.T) {
	_, err := NewMoneyFromString("not-a-number", "USD")
	require.Error(t, err)
}

func TestAESEncryptor_DecryptCiphertextTooShort(t *testing.T) {
	key := make([]byte, 32)
	enc, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	_, err = enc.Decrypt([]byte{0x01, 0x02})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestEncryptedString_ErrorBranches(t *testing.T) {
	t.Run("encryption fails", func(t *testing.T) {
		es := NewEncryptedString("secret", stubEncryptor{encryptErr: stdErrors.New("boom")})
		_, err := es.MarshalDynamORM()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encryption failed")
	})

	t.Run("unmarshal input not string", func(t *testing.T) {
		var es EncryptedString
		es.SetEncryptor(stubEncryptor{})
		err := es.UnmarshalDynamORM(123)
		require.Error(t, err)
	})

	t.Run("invalid base64", func(t *testing.T) {
		var es EncryptedString
		es.SetEncryptor(stubEncryptor{})
		err := es.UnmarshalDynamORM("%%%")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode encrypted data")
	})

	t.Run("decryption fails", func(t *testing.T) {
		encrypted := "AQID" // base64 for 0x01 0x02 0x03
		var es EncryptedString
		es.SetEncryptor(stubEncryptor{decryptErr: stdErrors.New("boom")})
		err := es.UnmarshalDynamORM(encrypted)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decryption failed")
	})
}

func TestJSONField_ErrorBranches(t *testing.T) {
	t.Run("marshal fails", func(t *testing.T) {
		jf := NewJSONField(make(chan int))
		_, err := jf.MarshalDynamORM()
		require.Error(t, err)
	})

	t.Run("unmarshal invalid type", func(t *testing.T) {
		var jf JSONField
		err := jf.UnmarshalDynamORM(123)
		require.Error(t, err)
	})

	t.Run("unmarshal empty string treated as null", func(t *testing.T) {
		var jf JSONField
		err := jf.UnmarshalDynamORM("")
		require.NoError(t, err)
		assert.Nil(t, jf.Data)
	})

	t.Run("unmarshal invalid json", func(t *testing.T) {
		var jf JSONField
		err := jf.UnmarshalDynamORM("{not-json}")
		require.Error(t, err)
	})

	t.Run("unmarshal into fails when data can't be marshaled", func(t *testing.T) {
		jf := NewJSONField(make(chan int))
		var out any
		err := jf.UnmarshalInto(&out)
		require.Error(t, err)
	})

	t.Run("string returns error when marshal fails", func(t *testing.T) {
		jf := NewJSONField(make(chan int))
		str := jf.String()
		assert.Contains(t, str, "error:")
	})

	t.Run("string contains json for maps", func(t *testing.T) {
		jf := NewJSONField(map[string]any{"k": "v"})
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(jf.String()), &decoded))
	})
}

func TestStringSet_UnmarshalAdditionalCases(t *testing.T) {
	t.Run("unmarshal from []string", func(t *testing.T) {
		var ss StringSet
		require.NoError(t, ss.UnmarshalDynamORM([]string{"a", "b"}))
		assert.Equal(t, []string{"a", "b"}, ss.Values)
	})

	t.Run("unmarshal invalid element", func(t *testing.T) {
		var ss StringSet
		err := ss.UnmarshalDynamORM([]interface{}{"a", 1})
		require.Error(t, err)
	})
}
