// Package marshalers provides custom DynamoDB marshaling utilities with encryption support for sensitive data using DynamORM.
package marshalers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

// Marshaler interface for custom DynamORM marshaling
type Marshaler interface {
	MarshalDynamORM() (interface{}, error)
}

// Unmarshaler interface for custom DynamORM unmarshaling
type Unmarshaler interface {
	UnmarshalDynamORM(interface{}) error
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

// MarshalDynamORM implements the Marshaler interface for PreciseTime
func (pt PreciseTime) MarshalDynamORM() (interface{}, error) {
	truncated := pt.Truncate(pt.Precision)

	// Store as a map with timestamp and precision
	return map[string]interface{}{
		"timestamp": truncated.Format(time.RFC3339Nano),
		"precision": int64(pt.Precision),
	}, nil
}

// UnmarshalDynamORM implements the Unmarshaler interface for PreciseTime
func (pt *PreciseTime) UnmarshalDynamORM(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid PreciseTime format: expected map")
	}

	// Parse timestamp
	timestampRaw, exists := dataMap["timestamp"]
	if !exists {
		return fmt.Errorf("invalid PreciseTime format: missing timestamp")
	}
	
	timestampStr, ok := timestampRaw.(string)
	if !ok {
		return fmt.Errorf("invalid PreciseTime format: timestamp must be string")
	}

	t, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		return fmt.Errorf("invalid PreciseTime timestamp: %w", err)
	}

	// Parse precision
	precisionRaw, exists := dataMap["precision"]
	if !exists {
		return fmt.Errorf("invalid PreciseTime format: missing precision")
	}

	var precisionNanos int64
	switch v := precisionRaw.(type) {
	case int64:
		precisionNanos = v
	case float64:
		precisionNanos = int64(v)
	case string:
		precisionNanos, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid PreciseTime precision: %w", err)
		}
	default:
		return fmt.Errorf("invalid PreciseTime precision type: %T", v)
	}

	pt.Time = t
	pt.Precision = time.Duration(precisionNanos)

	return nil
}

// String returns a string representation of PreciseTime
func (pt PreciseTime) String() string {
	return fmt.Sprintf("%s (precision: %s)", pt.Format(time.RFC3339Nano), pt.Precision)
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

// MarshalDynamORM implements the Marshaler interface for Money
func (m Money) MarshalDynamORM() (interface{}, error) {
	if err := common.ValidateRequiredParam("m.Currency", m.Currency); err != nil {
		return nil, fmt.Errorf("currency is required")
	}

	return map[string]interface{}{
		"amount":   m.Amount.String(),
		"currency": m.Currency,
	}, nil
}

// UnmarshalDynamORM implements the Unmarshaler interface for Money
func (m *Money) UnmarshalDynamORM(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Money format: expected map")
	}

	// Parse amount
	amountRaw, exists := dataMap["amount"]
	if !exists {
		return fmt.Errorf("invalid Money format: missing amount")
	}

	amountStr, ok := amountRaw.(string)
	if !ok {
		return fmt.Errorf("invalid Money format: amount must be string")
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return fmt.Errorf("invalid Money amount: %w", err)
	}

	// Parse currency
	currencyRaw, exists := dataMap["currency"]
	if !exists {
		return fmt.Errorf("invalid Money format: missing currency")
	}

	currency, ok := currencyRaw.(string)
	if !ok {
		return fmt.Errorf("invalid Money format: currency must be string")
	}

	m.Amount = amount
	m.Currency = currency

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

// NewAESEncryptor creates a new AES encryptor from centralized config
func NewAESEncryptor() (*AESEncryptor, error) {
	cfg := config.Get()
	keyBase64 := cfg.DynamoDBEncryptionKey
	if err := common.ValidateRequiredParam("keyBase64", keyBase64); err != nil {
		return nil, fmt.Errorf("DYNAMODB_ENCRYPTION_KEY not configured")
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

// MarshalDynamORM implements the Marshaler interface for EncryptedString
func (es EncryptedString) MarshalDynamORM() (interface{}, error) {
	if es.encryptor == nil {
		return nil, fmt.Errorf("encryptor is required")
	}

	encrypted, err := es.encryptor.Encrypt([]byte(es.Value))
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	// Return base64 encoded data for DynamORM
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// UnmarshalDynamORM implements the Unmarshaler interface for EncryptedString
func (es *EncryptedString) UnmarshalDynamORM(data interface{}) error {
	encryptedStr, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid EncryptedString format: expected base64 encoded string")
	}

	if es.encryptor == nil {
		return fmt.Errorf("encryptor is required for decryption")
	}

	// Decode base64 data
	encrypted, err := base64.StdEncoding.DecodeString(encryptedStr)
	if err != nil {
		return fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	decrypted, err := es.encryptor.Decrypt(encrypted)
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

// MarshalDynamORM implements the Marshaler interface for JSONField
func (jf JSONField) MarshalDynamORM() (interface{}, error) {
	if jf.Data == nil {
		return nil, nil
	}

	jsonBytes, err := json.Marshal(jf.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// UnmarshalDynamORM implements the Unmarshaler interface for JSONField
func (jf *JSONField) UnmarshalDynamORM(data interface{}) error {
	if data == nil {
		jf.Data = nil
		return nil
	}

	jsonStr, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid JSONField format: expected string")
	}

	if err := common.ValidateRequiredParam("jsonStr", jsonStr); err != nil {
		jf.Data = nil
		return nil
	}

	// Unmarshal into interface{} to preserve type information
	var jsonData interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	jf.Data = jsonData
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

// MarshalDynamORM implements the Marshaler interface for StringSet
func (ss StringSet) MarshalDynamORM() (interface{}, error) {
	if err := common.ValidateSliceNotEmpty("ss.Values", ss.Values); err != nil {
		return nil, nil
	}

	return ss.Values, nil
}

// UnmarshalDynamORM implements the Unmarshaler interface for StringSet
func (ss *StringSet) UnmarshalDynamORM(data interface{}) error {
	if data == nil {
		ss.Values = nil
		return nil
	}

	values, ok := data.([]interface{})
	if !ok {
		// Try []string directly
		stringValues, ok := data.([]string)
		if !ok {
			return fmt.Errorf("invalid StringSet format: expected array")
		}
		ss.Values = stringValues
		return nil
	}

	stringValues := make([]string, len(values))
	for i, v := range values {
		if str, ok := v.(string); ok {
			stringValues[i] = str
		} else {
			return fmt.Errorf("invalid StringSet element at index %d: expected string, got %T", i, v)
		}
	}

	ss.Values = stringValues
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
	if err := common.ValidateSliceNotEmpty("ss.Values", ss.Values); err != nil {
		return "[]"
	}

	jsonBytes, _ := json.Marshal(ss.Values)
	return string(jsonBytes)
}

// GenerateEncryptionKey creates an encryption key for testing
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// EncodeEncryptionKey encodes an encryption key as base64
func EncodeEncryptionKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}
