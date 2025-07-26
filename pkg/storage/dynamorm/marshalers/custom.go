package marshalers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/shopspring/decimal"
)

// Marshaler interface for custom DynamoDB marshaling
type Marshaler interface {
	MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error)
}

// Unmarshaler interface for custom DynamoDB unmarshaling  
type Unmarshaler interface {
	UnmarshalDynamoDB(map[string]*dynamodb.AttributeValue) error
}

// MarshalUnmarshaler combines both interfaces
type MarshalUnmarshaler interface {
	Marshaler
	Unmarshaler
}

// PreciseTime represents a time with configurable precision
type PreciseTime struct {
	time.Time
	Precision time.Duration
}

// NewPreciseTime creates a new PreciseTime with the specified precision
func NewPreciseTime(t time.Time, precision time.Duration) PreciseTime {
	return PreciseTime{
		Time:      t.Truncate(precision),
		Precision: precision,
	}
}

// NewPreciseTimeNow creates a new PreciseTime with the current time and specified precision
func NewPreciseTimeNow(precision time.Duration) PreciseTime {
	return NewPreciseTime(time.Now(), precision)
}

// MarshalDynamoDB implements the Marshaler interface for PreciseTime
func (pt PreciseTime) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
	truncated := pt.Time.Truncate(pt.Precision)
	
	// Store as a map with timestamp and precision
	return map[string]*dynamodb.AttributeValue{
		"M": {
			M: map[string]*dynamodb.AttributeValue{
				"timestamp": {S: aws.String(truncated.Format(time.RFC3339Nano))},
				"precision": {N: aws.String(strconv.FormatInt(int64(pt.Precision), 10))},
			},
		},
	}, nil
}

// UnmarshalDynamoDB implements the Unmarshaler interface for PreciseTime
func (pt *PreciseTime) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
	if av["M"] == nil || av["M"].M == nil {
		return fmt.Errorf("invalid PreciseTime format: expected map")
	}

	m := av["M"].M

	// Parse timestamp
	if m["timestamp"] == nil || m["timestamp"].S == nil {
		return fmt.Errorf("invalid PreciseTime format: missing timestamp")
	}

	t, err := time.Parse(time.RFC3339Nano, *m["timestamp"].S)
	if err != nil {
		return fmt.Errorf("invalid PreciseTime timestamp: %w", err)
	}

	// Parse precision
	if m["precision"] == nil || m["precision"].N == nil {
		return fmt.Errorf("invalid PreciseTime format: missing precision")
	}

	precisionNanos, err := strconv.ParseInt(*m["precision"].N, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PreciseTime precision: %w", err)
	}

	pt.Time = t
	pt.Precision = time.Duration(precisionNanos)

	return nil
}

// String returns a string representation of PreciseTime
func (pt PreciseTime) String() string {
	return fmt.Sprintf("%s (precision: %s)", pt.Time.Format(time.RFC3339Nano), pt.Precision)
}

// Money represents monetary values with currency information
type Money struct {
	Amount   decimal.Decimal `json:"amount"`
	Currency string          `json:"currency"`
}

// NewMoney creates a new Money instance
func NewMoney(amount decimal.Decimal, currency string) Money {
	return Money{
		Amount:   amount,
		Currency: currency,
	}
}

// NewMoneyFromFloat creates a new Money instance from a float64
func NewMoneyFromFloat(amount float64, currency string) Money {
	return Money{
		Amount:   decimal.NewFromFloat(amount),
		Currency: currency,
	}
}

// NewMoneyFromString creates a new Money instance from a string
func NewMoneyFromString(amount, currency string) (Money, error) {
	dec, err := decimal.NewFromString(amount)
	if err != nil {
		return Money{}, fmt.Errorf("invalid amount: %w", err)
	}
	return Money{
		Amount:   dec,
		Currency: currency,
	}, nil
}

// MarshalDynamoDB implements the Marshaler interface for Money
func (m Money) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
	if m.Currency == "" {
		return nil, fmt.Errorf("currency is required")
	}

	return map[string]*dynamodb.AttributeValue{
		"M": {
			M: map[string]*dynamodb.AttributeValue{
				"amount":   {N: aws.String(m.Amount.String())},
				"currency": {S: aws.String(m.Currency)},
			},
		},
	}, nil
}

// UnmarshalDynamoDB implements the Unmarshaler interface for Money
func (m *Money) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
	if av["M"] == nil || av["M"].M == nil {
		return fmt.Errorf("invalid Money format: expected map")
	}

	mapVal := av["M"].M

	// Parse amount
	if mapVal["amount"] == nil || mapVal["amount"].N == nil {
		return fmt.Errorf("invalid Money format: missing amount")
	}

	amount, err := decimal.NewFromString(*mapVal["amount"].N)
	if err != nil {
		return fmt.Errorf("invalid Money amount: %w", err)
	}

	// Parse currency
	if mapVal["currency"] == nil || mapVal["currency"].S == nil {
		return fmt.Errorf("invalid Money format: missing currency")
	}

	m.Amount = amount
	m.Currency = *mapVal["currency"].S

	return nil
}

// String returns a string representation of Money
func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.Amount.StringFixed(2), m.Currency)
}

// IsZero returns true if the money amount is zero
func (m Money) IsZero() bool {
	return m.Amount.IsZero()
}

// Add adds another Money value (must be same currency)
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("cannot add different currencies: %s and %s", m.Currency, other.Currency)
	}
	return Money{
		Amount:   m.Amount.Add(other.Amount),
		Currency: m.Currency,
	}, nil
}

// Sub subtracts another Money value (must be same currency)
func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("cannot subtract different currencies: %s and %s", m.Currency, other.Currency)
	}
	return Money{
		Amount:   m.Amount.Sub(other.Amount),
		Currency: m.Currency,
	}, nil
}

// EncryptedString represents an encrypted string value
type EncryptedString struct {
	Value     string
	encryptor Encryptor
}

// Encryptor interface for encryption operations
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AESEncryptor implements AES encryption
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor creates a new AES encryptor from environment variable
func NewAESEncryptor() (*AESEncryptor, error) {
	keyBase64 := os.Getenv("DYNAMODB_ENCRYPTION_KEY")
	if keyBase64 == "" {
		return nil, fmt.Errorf("DYNAMODB_ENCRYPTION_KEY environment variable not set")
	}

	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key: %w", err)
	}

	if len(key) != 32 { // AES-256
		return nil, fmt.Errorf("encryption key must be 32 bytes for AES-256")
	}

	return &AESEncryptor{key: key}, nil
}

// NewAESEncryptorWithKey creates a new AES encryptor with provided key
func NewAESEncryptorWithKey(key []byte) (*AESEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes for AES-256")
	}
	return &AESEncryptor{key: key}, nil
}

// Encrypt encrypts plaintext using AES-GCM
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-GCM
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// NewEncryptedString creates a new EncryptedString
func NewEncryptedString(value string, encryptor Encryptor) EncryptedString {
	return EncryptedString{
		Value:     value,
		encryptor: encryptor,
	}
}

// MarshalDynamoDB implements the Marshaler interface for EncryptedString
func (es EncryptedString) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
	if es.encryptor == nil {
		return nil, fmt.Errorf("encryptor is required")
	}

	encrypted, err := es.encryptor.Encrypt([]byte(es.Value))
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	return map[string]*dynamodb.AttributeValue{
		"B": {B: encrypted},
	}, nil
}

// UnmarshalDynamoDB implements the Unmarshaler interface for EncryptedString
func (es *EncryptedString) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
	if av["B"] == nil || len(av["B"].B) == 0 {
		return fmt.Errorf("invalid EncryptedString format: expected binary data")
	}

	if es.encryptor == nil {
		return fmt.Errorf("encryptor is required for decryption")
	}

	decrypted, err := es.encryptor.Decrypt(av["B"].B)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	es.Value = string(decrypted)
	return nil
}

// String returns the decrypted value (be careful with logging!)
func (es EncryptedString) String() string {
	return es.Value
}

// SetEncryptor sets the encryptor for the EncryptedString
func (es *EncryptedString) SetEncryptor(encryptor Encryptor) {
	es.encryptor = encryptor
}

// JSONField represents a field that stores arbitrary JSON data
type JSONField struct {
	Data any
}

// NewJSONField creates a new JSONField
func NewJSONField(data any) JSONField {
	return JSONField{Data: data}
}

// MarshalDynamoDB implements the Marshaler interface for JSONField
func (jf JSONField) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
	if jf.Data == nil {
		return map[string]*dynamodb.AttributeValue{
			"NULL": {NULL: aws.Bool(true)},
		}, nil
	}

	jsonBytes, err := json.Marshal(jf.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return map[string]*dynamodb.AttributeValue{
		"S": {S: aws.String(string(jsonBytes))},
	}, nil
}

// UnmarshalDynamoDB implements the Unmarshaler interface for JSONField
func (jf *JSONField) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
	if av["NULL"] != nil && aws.BoolValue(av["NULL"].NULL) {
		jf.Data = nil
		return nil
	}

	if av["S"] == nil || av["S"].S == nil {
		return fmt.Errorf("invalid JSONField format: expected string")
	}

	jsonStr := *av["S"].S
	if jsonStr == "" {
		jf.Data = nil
		return nil
	}

	// Unmarshal into interface{} to preserve type information
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	jf.Data = data
	return nil
}

// UnmarshalInto unmarshals the JSON data into a specific type
func (jf JSONField) UnmarshalInto(target any) error {
	if jf.Data == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(jf.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal intermediate JSON: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON into target: %w", err)
	}

	return nil
}

// String returns a string representation of the JSON data
func (jf JSONField) String() string {
	if jf.Data == nil {
		return "null"
	}

	jsonBytes, err := json.Marshal(jf.Data)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	return string(jsonBytes)
}

// IsNull returns true if the JSON field contains null data
func (jf JSONField) IsNull() bool {
	return jf.Data == nil
}

// StringSet represents a DynamoDB string set with additional functionality
type StringSet struct {
	Values []string
}

// NewStringSet creates a new StringSet
func NewStringSet(values ...string) StringSet {
	// Remove duplicates and sort
	unique := make(map[string]bool)
	var result []string
	
	for _, v := range values {
		if v != "" && !unique[v] {
			unique[v] = true
			result = append(result, v)
		}
	}

	return StringSet{Values: result}
}

// MarshalDynamoDB implements the Marshaler interface for StringSet
func (ss StringSet) MarshalDynamoDB() (map[string]*dynamodb.AttributeValue, error) {
	if len(ss.Values) == 0 {
		return map[string]*dynamodb.AttributeValue{
			"NULL": {NULL: aws.Bool(true)},
		}, nil
	}

	stringSet := make([]*string, len(ss.Values))
	for i, v := range ss.Values {
		stringSet[i] = aws.String(v)
	}

	return map[string]*dynamodb.AttributeValue{
		"SS": {SS: stringSet},
	}, nil
}

// UnmarshalDynamoDB implements the Unmarshaler interface for StringSet
func (ss *StringSet) UnmarshalDynamoDB(av map[string]*dynamodb.AttributeValue) error {
	if av["NULL"] != nil && aws.BoolValue(av["NULL"].NULL) {
		ss.Values = nil
		return nil
	}

	if av["SS"] == nil {
		return fmt.Errorf("invalid StringSet format: expected string set")
	}

	values := make([]string, len(av["SS"].SS))
	for i, v := range av["SS"].SS {
		if v != nil {
			values[i] = *v
		}
	}

	ss.Values = values
	return nil
}

// Add adds values to the string set
func (ss *StringSet) Add(values ...string) {
	existing := make(map[string]bool)
	for _, v := range ss.Values {
		existing[v] = true
	}

	for _, v := range values {
		if v != "" && !existing[v] {
			ss.Values = append(ss.Values, v)
			existing[v] = true
		}
	}
}

// Remove removes values from the string set
func (ss *StringSet) Remove(values ...string) {
	toRemove := make(map[string]bool)
	for _, v := range values {
		toRemove[v] = true
	}

	var result []string
	for _, v := range ss.Values {
		if !toRemove[v] {
			result = append(result, v)
		}
	}

	ss.Values = result
}

// Contains checks if the set contains a value
func (ss StringSet) Contains(value string) bool {
	for _, v := range ss.Values {
		if v == value {
			return true
		}
	}
	return false
}

// Size returns the number of elements in the set
func (ss StringSet) Size() int {
	return len(ss.Values)
}

// IsEmpty returns true if the set is empty
func (ss StringSet) IsEmpty() bool {
	return len(ss.Values) == 0
}

// ToSlice returns the values as a slice
func (ss StringSet) ToSlice() []string {
	result := make([]string, len(ss.Values))
	copy(result, ss.Values)
	return result
}

// String returns a string representation of the set
func (ss StringSet) String() string {
	if len(ss.Values) == 0 {
		return "[]"
	}
	
	jsonBytes, _ := json.Marshal(ss.Values)
	return string(jsonBytes)
}

// Helper function to create encryption key for testing
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// Helper function to encode encryption key as base64
func EncodeEncryptionKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}