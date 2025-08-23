package marshalers

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreciseTime_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		time      time.Time
		precision time.Duration
	}{
		{
			name:      "second precision",
			time:      time.Date(2023, 10, 15, 14, 30, 45, 123456789, time.UTC),
			precision: time.Second,
		},
		{
			name:      "millisecond precision",
			time:      time.Date(2023, 10, 15, 14, 30, 45, 123456789, time.UTC),
			precision: time.Millisecond,
		},
		{
			name:      "microsecond precision",
			time:      time.Date(2023, 10, 15, 14, 30, 45, 123456789, time.UTC),
			precision: time.Microsecond,
		},
		{
			name:      "nanosecond precision",
			time:      time.Date(2023, 10, 15, 14, 30, 45, 123456789, time.UTC),
			precision: time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewPreciseTime(tt.time, tt.precision)

			// Marshal
			data, err := original.MarshalDynamORM()
			require.NoError(t, err)
			require.NotNil(t, data)

			// Unmarshal
			var unmarshaled PreciseTime
			err = unmarshaled.UnmarshalDynamORM(data)
			require.NoError(t, err)

			// Verify
			assert.Equal(t, original.Truncate(tt.precision), unmarshaled.Time)
			assert.Equal(t, original.Precision, unmarshaled.Precision)
		})
	}
}

func TestPreciseTime_NewPreciseTimeNow(t *testing.T) {
	precision := time.Millisecond
	pt := NewPreciseTimeNow(precision)

	assert.Equal(t, precision, pt.Precision)
	assert.True(t, time.Since(pt.Time) < time.Second) // Should be recent
}

func TestPreciseTime_String(t *testing.T) {
	pt := NewPreciseTime(time.Date(2023, 10, 15, 14, 30, 45, 123456789, time.UTC), time.Millisecond)
	str := pt.String()

	assert.Contains(t, str, "2023-10-15T14:30:45")
	assert.Contains(t, str, "precision: 1ms")
}

func TestPreciseTime_InvalidUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "not a map",
			data: "not a map",
		},
		{
			name: "missing timestamp",
			data: map[string]interface{}{
				"precision": int64(1000000),
			},
		},
		{
			name: "invalid timestamp",
			data: map[string]interface{}{
				"timestamp": "invalid-time",
				"precision": int64(1000000),
			},
		},
		{
			name: "missing precision",
			data: map[string]interface{}{
				"timestamp": "2023-10-15T14:30:45Z",
			},
		},
		{
			name: "invalid precision",
			data: map[string]interface{}{
				"timestamp": "2023-10-15T14:30:45Z",
				"precision": "not-a-number",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pt PreciseTime
			err := pt.UnmarshalDynamORM(tt.data)
			assert.Error(t, err)
		})
	}
}

func TestMoney_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency string
	}{
		{
			name:     "USD",
			amount:   "100.50",
			currency: "USD",
		},
		{
			name:     "EUR",
			amount:   "75.25",
			currency: "EUR",
		},
		{
			name:     "JPY",
			amount:   "10000",
			currency: "JPY",
		},
		{
			name:     "zero amount",
			amount:   "0.00",
			currency: "USD",
		},
		{
			name:     "large amount",
			amount:   "9999999999.99",
			currency: "USD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, _ := decimal.NewFromString(tt.amount)
			original := NewMoney(amount, tt.currency)

			// Marshal
			data, err := original.MarshalDynamORM()
			require.NoError(t, err)
			require.NotNil(t, data)

			// Unmarshal
			var unmarshaled Money
			err = unmarshaled.UnmarshalDynamORM(data)
			require.NoError(t, err)

			// Verify
			assert.True(t, original.Amount.Equal(unmarshaled.Amount))
			assert.Equal(t, original.Currency, unmarshaled.Currency)
		})
	}
}

func TestMoney_String(t *testing.T) {
	amount, _ := decimal.NewFromString("100.50")
	money := NewMoney(amount, "USD")
	str := money.String()

	assert.Contains(t, str, "100.50")
	assert.Contains(t, str, "USD")
}

func TestMoney_InvalidUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "not a map",
			data: "not a map",
		},
		{
			name: "missing amount",
			data: map[string]interface{}{
				"currency": "USD",
			},
		},
		{
			name: "invalid amount",
			data: map[string]interface{}{
				"amount":   "not-a-number",
				"currency": "USD",
			},
		},
		{
			name: "missing currency",
			data: map[string]interface{}{
				"amount": "100.50",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var money Money
			err := money.UnmarshalDynamORM(tt.data)
			assert.Error(t, err)
		})
	}
}

func TestEncryptedString_MarshalUnmarshal(t *testing.T) {
	// Generate test encryption key
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	encryptor, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "simple text",
			value: "secret message",
		},
		{
			name:  "empty string",
			value: "",
		},
		{
			name:  "unicode text",
			value: "🔐 secret émojis and accénts",
		},
		{
			name:  "long text",
			value: "This is a very long secret message that should be encrypted properly and decrypted back to the original value without any loss of data or corruption",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewEncryptedString(tt.value, encryptor)

			// Marshal
			data, err := original.MarshalDynamORM()
			require.NoError(t, err)
			require.NotNil(t, data)

			// Unmarshal (need to set encryptor)
			var unmarshaled EncryptedString
			unmarshaled.SetEncryptor(encryptor)
			err = unmarshaled.UnmarshalDynamORM(data)
			require.NoError(t, err)

			// Verify
			assert.Equal(t, original.Value, unmarshaled.Value)
		})
	}
}

func TestEncryptedString_NoEncryptor(t *testing.T) {
	es := EncryptedString{Value: "secret", encryptor: nil}

	// Marshal without encryptor should fail
	_, err := es.MarshalDynamORM()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryptor is required")

	// Unmarshal without encryptor should fail
	data := []byte("encrypted data")
	err = es.UnmarshalDynamORM(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryptor is required")
}

func TestJSONField_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{
			name: "string",
			data: "hello world",
		},
		{
			name: "number",
			data: 42,
		},
		{
			name: "boolean",
			data: true,
		},
		{
			name: "array",
			data: []string{"a", "b", "c"},
		},
		{
			name: "object",
			data: map[string]any{
				"name":  "John",
				"age":   30,
				"email": "john@example.com",
			},
		},
		{
			name: "null",
			data: nil,
		},
		{
			name: "complex nested",
			data: map[string]any{
				"users": []map[string]any{
					{"id": 1, "name": "Alice"},
					{"id": 2, "name": "Bob"},
				},
				"metadata": map[string]any{
					"total": 2,
					"page":  1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewJSONField(tt.data)

			// Marshal
			data, err := original.MarshalDynamORM()
			require.NoError(t, err)

			// Unmarshal
			var unmarshaled JSONField
			err = unmarshaled.UnmarshalDynamORM(data)
			require.NoError(t, err)

			// Verify
			if tt.data == nil {
				assert.Nil(t, unmarshaled.Data)
			} else {
				// Convert both to JSON for comparison
				originalJSON, _ := json.Marshal(tt.data)
				unmarshaledJSON, _ := json.Marshal(unmarshaled.Data)
				assert.JSONEq(t, string(originalJSON), string(unmarshaledJSON))
			}
		})
	}
}

func TestJSONField_UnmarshalInto(t *testing.T) {
	// Test data
	data := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john@example.com",
	}

	jf := NewJSONField(data)

	// Unmarshal into struct
	type User struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"email"`
	}

	var user User
	err := jf.UnmarshalInto(&user)
	require.NoError(t, err)

	assert.Equal(t, "John Doe", user.Name)
	assert.Equal(t, 30, user.Age)
	assert.Equal(t, "john@example.com", user.Email)
}

func TestJSONField_String(t *testing.T) {
	// Test with data
	data := map[string]string{"key": "value"}
	jf := NewJSONField(data)
	str := jf.String()
	assert.Contains(t, str, "key")
	assert.Contains(t, str, "value")

	// Test with null
	jfNull := NewJSONField(nil)
	assert.Equal(t, "null", jfNull.String())
}

func TestJSONField_IsNull(t *testing.T) {
	jfNull := NewJSONField(nil)
	assert.True(t, jfNull.IsNull())

	jfData := NewJSONField("data")
	assert.False(t, jfData.IsNull())
}

func TestStringSet_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{
			name:   "multiple values",
			values: []string{"a", "b", "c"},
		},
		{
			name:   "single value",
			values: []string{"single"},
		},
		{
			name:   "empty set",
			values: []string{},
		},
		{
			name:   "with duplicates (should be removed)",
			values: []string{"a", "b", "a", "c", "b"},
		},
		{
			name:   "with empty strings (should be removed)",
			values: []string{"a", "", "b", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewStringSet(tt.values...)

			// Marshal
			data, err := original.MarshalDynamORM()
			require.NoError(t, err)

			// Unmarshal
			var unmarshaled StringSet
			err = unmarshaled.UnmarshalDynamORM(data)
			require.NoError(t, err)

			// Verify (sets might be in different order)
			assert.Equal(t, len(original.Values), len(unmarshaled.Values))
			for _, v := range original.Values {
				assert.Contains(t, unmarshaled.Values, v)
			}
		})
	}
}

func TestStringSet_Operations(t *testing.T) {
	ss := NewStringSet("a", "b", "c")

	// Test Add
	ss.Add("d", "e")
	assert.Equal(t, 5, ss.Size())
	assert.True(t, ss.Contains("d"))
	assert.True(t, ss.Contains("e"))

	// Test Add duplicate
	ss.Add("a") // Should not add duplicate
	assert.Equal(t, 5, ss.Size())

	// Test Remove
	ss.Remove("b", "c")
	assert.Equal(t, 3, ss.Size())
	assert.False(t, ss.Contains("b"))
	assert.False(t, ss.Contains("c"))

	// Test Contains
	assert.True(t, ss.Contains("a"))
	assert.False(t, ss.Contains("removed"))

	// Test IsEmpty
	assert.False(t, ss.IsEmpty())

	emptySet := NewStringSet()
	assert.True(t, emptySet.IsEmpty())

	// Test ToSlice
	slice := ss.ToSlice()
	assert.Equal(t, ss.Size(), len(slice))
}

func TestStringSet_String(t *testing.T) {
	ss := NewStringSet("a", "b", "c")
	str := ss.String()

	// Should be valid JSON array
	var result []string
	err := json.Unmarshal([]byte(str), &result)
	require.NoError(t, err)
	assert.Equal(t, len(ss.Values), len(result))

	// Empty set
	empty := NewStringSet()
	assert.Equal(t, "[]", empty.String())
}

func TestNewAESEncryptor_FromEnvironment(t *testing.T) {
	// Test with valid environment variable
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)
	keyBase64 := EncodeEncryptionKey(key)

	_ = os.Setenv("DYNAMODB_ENCRYPTION_KEY", keyBase64)
	defer func() { _ = os.Unsetenv("DYNAMODB_ENCRYPTION_KEY") }()

	encryptor, err := NewAESEncryptor()
	require.NoError(t, err)
	assert.NotNil(t, encryptor)

	// Test encryption/decryption works
	plaintext := "test message"
	encrypted, err := encryptor.Encrypt([]byte(plaintext))
	require.NoError(t, err)

	decrypted, err := encryptor.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(decrypted))
}

func TestNewAESEncryptor_FromEnvironment_Errors(t *testing.T) {
	// Test with missing environment variable
	_ = os.Unsetenv("DYNAMODB_ENCRYPTION_KEY")
	_, err := NewAESEncryptor()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "environment variable not set")

	// Test with invalid base64
	_ = os.Setenv("DYNAMODB_ENCRYPTION_KEY", "invalid-base64!")
	defer func() { _ = os.Unsetenv("DYNAMODB_ENCRYPTION_KEY") }()
	_, err = NewAESEncryptor()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid encryption key")

	// Test with wrong key length
	shortKey := []byte("short")
	_ = os.Setenv("DYNAMODB_ENCRYPTION_KEY", EncodeEncryptionKey(shortKey))
	_, err = NewAESEncryptor()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")
}

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)
	assert.Equal(t, 32, len(key))

	// Generate another key and ensure they're different
	key2, err := GenerateEncryptionKey()
	require.NoError(t, err)
	assert.NotEqual(t, key, key2)
}

func TestEncodeEncryptionKey(t *testing.T) {
	key := []byte("this-is-exactly-32-bytes-long!!!")
	encoded := EncodeEncryptionKey(key)

	// Should be valid base64
	assert.NotEmpty(t, encoded)

	// Should decode back to original key
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, key, decoded)
}

// Helper function to get minimum of two integers
func mathMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}