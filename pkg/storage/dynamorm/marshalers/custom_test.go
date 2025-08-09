package marshalers

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
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
			av, err := original.MarshalDynamoDB()
			require.NoError(t, err)
			require.NotNil(t, av["M"])

			// Unmarshal
			var unmarshaled PreciseTime
			err = unmarshaled.UnmarshalDynamoDB(av)
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
		av   map[string]*dynamodb.AttributeValue
	}{
		{
			name: "not a map",
			av: map[string]*dynamodb.AttributeValue{
				"S": {S: aws.String("not a map")},
			},
		},
		{
			name: "missing timestamp",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"precision": {N: aws.String("1000000")},
					},
				},
			},
		},
		{
			name: "invalid timestamp",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"timestamp": {S: aws.String("invalid-time")},
						"precision": {N: aws.String("1000000")},
					},
				},
			},
		},
		{
			name: "missing precision",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"timestamp": {S: aws.String("2023-10-15T14:30:45Z")},
					},
				},
			},
		},
		{
			name: "invalid precision",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"timestamp": {S: aws.String("2023-10-15T14:30:45Z")},
						"precision": {S: aws.String("not-a-number")},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pt PreciseTime
			err := pt.UnmarshalDynamoDB(tt.av)
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
			name:     "USD with cents",
			amount:   "123.45",
			currency: "USD",
		},
		{
			name:     "EUR whole number",
			amount:   "1000",
			currency: "EUR",
		},
		{
			name:     "JPY with precision",
			amount:   "999.999",
			currency: "JPY",
		},
		{
			name:     "zero amount",
			amount:   "0",
			currency: "USD",
		},
		{
			name:     "negative amount",
			amount:   "-50.25",
			currency: "GBP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tt.amount)
			require.NoError(t, err)

			original := NewMoney(amount, tt.currency)

			// Marshal
			av, err := original.MarshalDynamoDB()
			require.NoError(t, err)
			require.NotNil(t, av["M"])

			// Unmarshal
			var unmarshaled Money
			err = unmarshaled.UnmarshalDynamoDB(av)
			require.NoError(t, err)

			// Verify
			assert.True(t, original.Amount.Equal(unmarshaled.Amount))
			assert.Equal(t, original.Currency, unmarshaled.Currency)
		})
	}
}

func TestMoney_NewMoneyFromFloat(t *testing.T) {
	money := NewMoneyFromFloat(123.45, "USD")

	assert.Equal(t, "123.45", money.Amount.StringFixed(2))
	assert.Equal(t, "USD", money.Currency)
}

func TestMoney_NewMoneyFromString(t *testing.T) {
	money, err := NewMoneyFromString("123.45", "USD")
	require.NoError(t, err)

	assert.Equal(t, "123.45", money.Amount.StringFixed(2))
	assert.Equal(t, "USD", money.Currency)

	// Test invalid amount
	_, err = NewMoneyFromString("invalid", "USD")
	assert.Error(t, err)
}

func TestMoney_Operations(t *testing.T) {
	money1 := NewMoneyFromFloat(100.50, "USD")
	money2 := NewMoneyFromFloat(50.25, "USD")
	money3 := NewMoneyFromFloat(25.00, "EUR")

	// Test Add
	result, err := money1.Add(money2)
	require.NoError(t, err)
	assert.Equal(t, "150.75", result.Amount.StringFixed(2))
	assert.Equal(t, "USD", result.Currency)

	// Test Add with different currency
	_, err = money1.Add(money3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "different currencies")

	// Test Sub
	result, err = money1.Sub(money2)
	require.NoError(t, err)
	assert.Equal(t, "50.25", result.Amount.StringFixed(2))
	assert.Equal(t, "USD", result.Currency)

	// Test Sub with different currency
	_, err = money1.Sub(money3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "different currencies")

	// Test IsZero
	zero := NewMoney(decimal.Zero, "USD")
	assert.True(t, zero.IsZero())
	assert.False(t, money1.IsZero())
}

func TestMoney_String(t *testing.T) {
	money := NewMoneyFromFloat(123.45, "USD")
	str := money.String()

	assert.Equal(t, "123.45 USD", str)
}

func TestMoney_InvalidMarshal(t *testing.T) {
	money := Money{
		Amount:   decimal.NewFromFloat(100.0),
		Currency: "", // Empty currency should cause error
	}

	_, err := money.MarshalDynamoDB()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "currency is required")
}

func TestMoney_InvalidUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		av   map[string]*dynamodb.AttributeValue
	}{
		{
			name: "not a map",
			av: map[string]*dynamodb.AttributeValue{
				"S": {S: aws.String("not a map")},
			},
		},
		{
			name: "missing amount",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"currency": {S: aws.String("USD")},
					},
				},
			},
		},
		{
			name: "invalid amount",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"amount":   {S: aws.String("not-a-number")},
						"currency": {S: aws.String("USD")},
					},
				},
			},
		},
		{
			name: "missing currency",
			av: map[string]*dynamodb.AttributeValue{
				"M": {
					M: map[string]*dynamodb.AttributeValue{
						"amount": {N: aws.String("100.50")},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var money Money
			err := money.UnmarshalDynamoDB(tt.av)
			assert.Error(t, err)
		})
	}
}

func TestAESEncryptor_EncryptDecrypt(t *testing.T) {
	// Generate a test key
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	encryptor, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	testData := []string{
		"Hello, World!",
		"This is a secret message",
		"",
		"Unicode: 🔐 encrypted 🗝️",
		"Very long message: " + string(make([]byte, 1000)),
	}

	for _, original := range testData {
		t.Run("encrypt/decrypt: "+original[:mathMin(20, len(original))], func(t *testing.T) {
			// Encrypt
			encrypted, err := encryptor.Encrypt([]byte(original))
			require.NoError(t, err)
			assert.NotEqual(t, original, encrypted)

			// Decrypt
			decrypted, err := encryptor.Decrypt(encrypted)
			require.NoError(t, err)
			assert.Equal(t, original, string(decrypted))
		})
	}
}

func TestAESEncryptor_InvalidKey(t *testing.T) {
	// Test with invalid key length
	_, err := NewAESEncryptorWithKey([]byte("too short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")
}

func TestAESEncryptor_DecryptInvalidData(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	encryptor, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	// Test with too short ciphertext
	_, err = encryptor.Decrypt([]byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")

	// Test with invalid ciphertext
	invalidCiphertext := make([]byte, 20) // Valid length but invalid content
	_, err = encryptor.Decrypt(invalidCiphertext)
	assert.Error(t, err)
}

func TestEncryptedString_MarshalUnmarshal(t *testing.T) {
	// Setup encryptor
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)
	encryptor, err := NewAESEncryptorWithKey(key)
	require.NoError(t, err)

	testValues := []string{
		"password123",
		"secret api key",
		"",
		"🔐 secret 🗝️",
	}

	for _, value := range testValues {
		t.Run("value: "+value, func(t *testing.T) {
			original := NewEncryptedString(value, encryptor)

			// Marshal
			av, err := original.MarshalDynamoDB()
			require.NoError(t, err)
			require.NotNil(t, av["B"])

			// Unmarshal (need to set encryptor)
			var unmarshaled EncryptedString
			unmarshaled.SetEncryptor(encryptor)
			err = unmarshaled.UnmarshalDynamoDB(av)
			require.NoError(t, err)

			// Verify
			assert.Equal(t, original.Value, unmarshaled.Value)
		})
	}
}

func TestEncryptedString_NoEncryptor(t *testing.T) {
	es := EncryptedString{Value: "secret"}

	// Marshal without encryptor should fail
	_, err := es.MarshalDynamoDB()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "encryptor is required")

	// Unmarshal without encryptor should fail
	av := map[string]*dynamodb.AttributeValue{
		"B": {B: []byte("encrypted data")},
	}
	err = es.UnmarshalDynamoDB(av)
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
			av, err := original.MarshalDynamoDB()
			require.NoError(t, err)

			// Unmarshal
			var unmarshaled JSONField
			err = unmarshaled.UnmarshalDynamoDB(av)
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
			av, err := original.MarshalDynamoDB()
			require.NoError(t, err)

			// Unmarshal
			var unmarshaled StringSet
			err = unmarshaled.UnmarshalDynamoDB(av)
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
