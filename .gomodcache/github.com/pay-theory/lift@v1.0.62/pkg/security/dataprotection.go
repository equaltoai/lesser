package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// DataClassification defines data sensitivity levels
type DataClassification string

const (
	DataPublic       DataClassification = "public"
	DataInternal     DataClassification = "internal"
	DataConfidential DataClassification = "confidential"
	DataRestricted   DataClassification = "restricted"
)

// DataProtectionConfig holds configuration for data protection
type DataProtectionConfig struct {
	FieldClassifications  map[string]DataClassification        `json:"field_classifications"`
	RegionRestrictions    map[DataClassification][]string      `json:"region_restrictions"`
	RetentionPolicies     map[DataClassification]time.Duration `json:"retention_policies"`
	AccessControls        map[DataClassification][]string      `json:"access_controls"`
	MaskingRules          map[string]MaskingRule               `json:"masking_rules"`
	EncryptionKey         string                               `json:"encryption_key"`
	DefaultClassification DataClassification                   `json:"default_classification"`
}

// MaskingRule defines how to mask sensitive data
type MaskingRule struct {
	Type        string `json:"type"`        // "partial", "full", "hash", "tokenize"
	Pattern     string `json:"pattern"`     // regex pattern for partial masking
	Replacement string `json:"replacement"` // replacement character/string
}

// DataProtectionManager handles data classification and protection
type DataProtectionManager struct { //nolint:govet // fieldalignment: struct dominated by pointer fields; refactor would be invasive
	config    DataProtectionConfig
	encryptor *AESEncryptor
	tokenizer *DataTokenizer
	mu        sync.RWMutex
}

// DataContext represents data with its classification and metadata
type DataContext struct {
	Data           any                           `json:"data"`
	Classification DataClassification            `json:"classification"`
	Fields         map[string]DataClassification `json:"fields"`
	Metadata       map[string]any                `json:"metadata"`
	Timestamp      time.Time                     `json:"timestamp"`
	UserID         string                        `json:"user_id"`
	TenantID       string                        `json:"tenant_id"`
	Region         string                        `json:"region"`
	Purpose        string                        `json:"purpose"`
}

// DataProtectionRequest represents a request to access protected data
type DataProtectionRequest struct { //nolint:govet // fieldalignment: API shape preferred; further packing has limited value
	Fields         []string           `json:"fields"`
	UserID         string             `json:"user_id"`
	TenantID       string             `json:"tenant_id"`
	DataType       string             `json:"data_type"`
	Purpose        string             `json:"purpose"`
	Region         string             `json:"region"`
	Classification DataClassification `json:"classification"`
	Metadata       map[string]any     `json:"metadata"`
}

// DataAccessResult represents the result of a data access request
type DataAccessResult struct { //nolint:govet // fieldalignment: API shape preferred; further packing has limited value
	Restrictions  []string       `json:"restrictions,omitempty"`
	Violations    []string       `json:"violations,omitempty"`
	Data          any            `json:"data,omitempty"`
	MaskedData    any            `json:"masked_data,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ExpiresAt     time.Time      `json:"expires_at,omitempty"`
	Allowed       bool           `json:"allowed"`
	AuditRequired bool           `json:"audit_required"`
}

// AESEncryptor handles AES encryption/decryption
type AESEncryptor struct {
	key []byte
}

// DataTokenizer handles data tokenization for PCI compliance
type DataTokenizer struct {
	tokens map[string]string
	data   map[string]string
	mu     sync.RWMutex
}

// NewDataProtectionManager creates a new data protection manager
func NewDataProtectionManager(config DataProtectionConfig) (*DataProtectionManager, error) {
	// Initialize encryptor
	encryptor, err := NewAESEncryptor(config.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize encryptor: %w", err)
	}

	// Initialize tokenizer
	tokenizer := NewDataTokenizer()

	return &DataProtectionManager{
		config:    config,
		encryptor: encryptor,
		tokenizer: tokenizer,
	}, nil
}

// ClassifyData classifies data based on content and configuration
func (dpm *DataProtectionManager) ClassifyData(data any, context map[string]any) *DataContext {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()

	dataCtx := &DataContext{
		Data:           data,
		Classification: dpm.config.DefaultClassification,
		Fields:         make(map[string]DataClassification),
		Metadata:       context,
		Timestamp:      time.Now(),
	}

	// Extract user and tenant info from context
	if userID, ok := context["user_id"].(string); ok {
		dataCtx.UserID = userID
	}
	if tenantID, ok := context["tenant_id"].(string); ok {
		dataCtx.TenantID = tenantID
	}
	if region, ok := context["region"].(string); ok {
		dataCtx.Region = region
	}
	if purpose, ok := context["purpose"].(string); ok {
		dataCtx.Purpose = purpose
	}

	// Classify based on data content
	if jsonData, err := json.Marshal(data); err == nil {
		var dataMap map[string]any
		if json.Unmarshal(jsonData, &dataMap) == nil {
			dataCtx.Classification = dpm.classifyDataMap(dataMap, dataCtx.Fields)
		}
	}

	return dataCtx
}

// classifyDataMap classifies a data map and its fields
func (dpm *DataProtectionManager) classifyDataMap(data map[string]any, fieldClassifications map[string]DataClassification) DataClassification {
	highestClassification := DataPublic

	for field, value := range data {
		fieldClass := dpm.classifyField(field, value)
		fieldClassifications[field] = fieldClass

		// Update highest classification
		if dpm.isHigherClassification(fieldClass, highestClassification) {
			highestClassification = fieldClass
		}
	}

	return highestClassification
}

// classifyField classifies a single field
func (dpm *DataProtectionManager) classifyField(field string, value any) DataClassification {
	// Explicit config overrides
	if classification, exists := dpm.config.FieldClassifications[field]; exists {
		return classification
	}

	fl := strings.ToLower(field)

	// Structured, readable checks
	if dpm.isSensitiveNumberField(fl) {
		return DataRestricted
	}
	if dpm.isIPField(fl) {
		return DataPublic
	}
	if dpm.isIDField(fl) {
		return DataPublic
	}
	if cls, ok := dpm.classifyKeyField(fl); ok {
		return cls
	}
	if cls, ok := dpm.classifyHighSensitivityKeyword(fl); ok {
		return cls
	}
	if cls, ok := dpm.classifyByCategoryPatterns(fl); ok {
		return cls
	}

	// Value-based detection (e.g., CC, SSN)
	if strValue, ok := value.(string); ok {
		if dpm.isRestrictedValue(strValue) {
			return DataRestricted
		}
	}

	return dpm.config.DefaultClassification
}

// Helper predicates and classifiers to reduce cognitive complexity
func (dpm *DataProtectionManager) isSensitiveNumberField(fl string) bool {
	switch fl {
	case "account_number", "business_tin_ssn_number", "card_num", "card_number", "cardnumber",
		"dda_number", "ein", "employer_identification_number", "merchant_tax_id", "number",
		"owner_tin_ssn_number", "social_security", "social_security_number", "ssn", "tax_id",
		"tax_identification_number", "taxid", "tin":
		return true
	default:
		return false
	}
}

func (dpm *DataProtectionManager) isIPField(fl string) bool {
	switch fl {
	case "ip_address", "ipaddress", "client_ip", "source_ip", "remote_ip", "x_forwarded_for",
		"x_real_ip", "cf_connecting_ip", "x-forwarded-for", "x-real-ip", "cf-connecting-ip",
		"sourceip", "clientip", "remoteip", "server_ip", "user_ip", "host_ip", "ip":
		return true
	}
	if strings.HasSuffix(fl, "_ip") || strings.HasPrefix(fl, "ip_") || strings.Contains(fl, "_ip_") {
		return true
	}
	if strings.Contains(fl, "forwarded") && strings.Contains(fl, "ip") {
		return true
	}
	return false
}

func (dpm *DataProtectionManager) isIDField(fl string) bool {
	return strings.HasSuffix(fl, "_id")
}

func (dpm *DataProtectionManager) classifyKeyField(fl string) (DataClassification, bool) {
	switch fl {
	case "key", "keys", "apikey", "api_key", "access_key", "secret_key", "encryption_key",
		"signing_key", "public_key", "private_key", "key_id", "key_name", "key_type", "key_version",
		"key_arn", "master_key", "data_key", "kms_key", "session_key":
		if strings.Contains(fl, "secret") || strings.Contains(fl, "private") {
			return DataRestricted, true
		}
		return DataInternal, true
	}
	if fl == "key" || strings.HasSuffix(fl, "_key") || strings.HasPrefix(fl, "key_") ||
		strings.HasSuffix(fl, "_keys") || strings.HasPrefix(fl, "keys_") {
		if strings.Contains(fl, "secret") || strings.Contains(fl, "private") {
			return DataRestricted, true
		}
		return DataInternal, true
	}
	return "", false
}

func (dpm *DataProtectionManager) classifyHighSensitivityKeyword(fl string) (DataClassification, bool) {
	for _, sensitive := range []string{
		"password", "token", "secret", "auth", "credential",
		"email", "phone", "ssn", "card", "account", "routing",
		"pin", "cvv", "security", "private", "confidential",
	} {
		if strings.Contains(fl, sensitive) {
			switch sensitive {
			case "ssn", "card", "account", "routing", "cvv", "pin":
				return DataRestricted, true
			case "password", "token", "secret", "auth", "credential", "private":
				return DataConfidential, true
			case "email", "phone":
				return DataInternal, true
			default:
				return DataConfidential, true
			}
		}
	}
	return "", false
}

func (dpm *DataProtectionManager) classifyByCategoryPatterns(fl string) (DataClassification, bool) {
	for _, pattern := range []string{"tax_id", "passport", "driver_license", "bank_account", "medical_record", "health_record", "diagnosis", "prescription"} {
		if strings.Contains(fl, pattern) {
			return DataRestricted, true
		}
	}
	for _, pattern := range []string{"salary", "income", "financial", "revenue", "profit"} {
		if strings.Contains(fl, pattern) {
			return DataConfidential, true
		}
	}
	for _, pattern := range []string{"body", "request_body", "response_body", "user_input", "query", "search", "message", "comment", "description"} {
		if strings.Contains(fl, pattern) {
			return DataInternal, true
		}
	}
	for _, pattern := range []string{"address", "name", "birth", "employee", "organization"} {
		if strings.Contains(fl, pattern) {
			return DataInternal, true
		}
	}
	return "", false
}

// isRestrictedValue checks if a value matches restricted data patterns
func (dpm *DataProtectionManager) isRestrictedValue(value string) bool {
	// Remove spaces and dashes for pattern matching
	cleaned := strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), "-", "")

	// Credit card pattern (13-19 digits)
	if len(cleaned) >= 13 && len(cleaned) <= 19 {
		allDigits := true
		for _, char := range cleaned {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return true
		}
	}

	// SSN pattern (9 digits)
	if len(cleaned) == 9 {
		allDigits := true
		for _, char := range cleaned {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return true
		}
	}

	return false
}

// isHigherClassification checks if one classification is higher than another
func (dpm *DataProtectionManager) isHigherClassification(a, b DataClassification) bool {
	levels := map[DataClassification]int{
		DataPublic:       0,
		DataInternal:     1,
		DataConfidential: 2,
		DataRestricted:   3,
	}

	return levels[a] > levels[b]
}

// ValidateDataAccess validates if data access is allowed
func (dpm *DataProtectionManager) ValidateDataAccess(request DataProtectionRequest) *DataAccessResult {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()

	result := &DataAccessResult{
		Allowed:       true,
		AuditRequired: true,
		Metadata:      make(map[string]any),
	}

	// Check region restrictions
	if restrictions, exists := dpm.config.RegionRestrictions[request.Classification]; exists {
		allowed := false
		for _, allowedRegion := range restrictions {
			if allowedRegion == request.Region || allowedRegion == "*" {
				allowed = true
				break
			}
		}
		if !allowed {
			result.Allowed = false
			result.Violations = append(result.Violations, fmt.Sprintf("Data access not allowed in region: %s", request.Region))
			return result // Return early if region check fails
		}
	}

	// Check access controls
	if controls, exists := dpm.config.AccessControls[request.Classification]; exists {
		// This would typically check user roles/permissions
		// For now, we'll assume access is allowed if user is specified
		if request.UserID == "" {
			result.Allowed = false
			result.Violations = append(result.Violations, "User authentication required for this data classification")
			return result // Return early if user check fails
		}

		result.Restrictions = controls
	}

	// Set expiration based on classification
	if retention, exists := dpm.config.RetentionPolicies[request.Classification]; exists {
		result.ExpiresAt = time.Now().Add(retention)
	}

	// Require audit for sensitive data
	if request.Classification == DataConfidential || request.Classification == DataRestricted {
		result.AuditRequired = true
	}

	return result
}

// ValidateDataAccessFromGDPR validates data access from a GDPR DataAccessRequest
func (dpm *DataProtectionManager) ValidateDataAccessFromGDPR(request any) *DataAccessResult {
	// Convert DataAccessRequest to DataProtectionRequest
	var protectionRequest DataProtectionRequest

	// Use type assertion to handle the interface
	if dar, ok := request.(DataAccessRequest); ok {
		protectionRequest = DataProtectionRequest{
			UserID:         dar.UserID,
			TenantID:       "", // Not available in DataAccessRequest
			DataType:       dar.RequestType,
			Classification: DataClassification(dar.Purpose), // Map purpose to classification
			Purpose:        dar.Purpose,
			Region:         dar.Region,
			Fields:         dar.Scope,
			Metadata:       dar.Metadata,
		}

		// Try to infer classification from purpose or other fields
		switch dar.Purpose {
		case "restricted", "processing":
			protectionRequest.Classification = DataRestricted
		case "confidential":
			protectionRequest.Classification = DataConfidential
		case "internal":
			protectionRequest.Classification = DataInternal
		default:
			protectionRequest.Classification = DataPublic
		}
	} else {
		// If it's already a DataProtectionRequest, use it directly
		if dpr, ok := request.(DataProtectionRequest); ok {
			protectionRequest = dpr
		} else {
			// Return error result for unsupported type
			return &DataAccessResult{
				Allowed:    false,
				Violations: []string{"Unsupported request type"},
				Metadata:   make(map[string]any),
			}
		}
	}

	return dpm.ValidateDataAccess(protectionRequest)
}

// ProtectData applies protection measures to data based on classification
func (dpm *DataProtectionManager) ProtectData(dataCtx *DataContext, accessRequest DataProtectionRequest) (*DataAccessResult, error) {
	// Validate access first
	result := dpm.ValidateDataAccess(accessRequest)
	if !result.Allowed {
		return result, nil
	}

	// Apply data protection based on classification
	switch dataCtx.Classification {
	case DataRestricted:
		// Encrypt or tokenize restricted data
		if accessRequest.Purpose == "display" {
			maskedData, err := dpm.maskData(dataCtx.Data, dataCtx.Fields)
			if err != nil {
				return nil, fmt.Errorf("failed to mask data: %w", err)
			}
			result.MaskedData = maskedData
		} else {
			// For processing purposes, provide encrypted data
			encryptedData, err := dpm.encryptor.Encrypt(dataCtx.Data)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt data: %w", err)
			}
			result.Data = encryptedData
		}

	case DataConfidential:
		// Apply field-level masking for confidential data
		maskedData, err := dpm.maskData(dataCtx.Data, dataCtx.Fields)
		if err != nil {
			return nil, fmt.Errorf("failed to mask data: %w", err)
		}
		result.MaskedData = maskedData

	case DataInternal:
		// Apply minimal masking for internal data
		if accessRequest.Purpose == "external" {
			maskedData, err := dpm.maskData(dataCtx.Data, dataCtx.Fields)
			if err != nil {
				return nil, fmt.Errorf("failed to mask data: %w", err)
			}
			result.MaskedData = maskedData
		} else {
			result.Data = dataCtx.Data
		}

	case DataPublic:
		// No protection needed for public data
		result.Data = dataCtx.Data
	}

	return result, nil
}

// maskData applies masking rules to data
func (dpm *DataProtectionManager) maskData(data any, fieldClassifications map[string]DataClassification) (any, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var dataMap map[string]any
	if err := json.Unmarshal(jsonData, &dataMap); err != nil {
		return nil, err
	}

	maskedMap := make(map[string]any)

	for field, value := range dataMap {
		classification := fieldClassifications[field]

		// Apply masking based on classification and rules
		if rule, exists := dpm.config.MaskingRules[field]; exists {
			maskedValue, err := dpm.applyMaskingRule(value, rule)
			if err != nil {
				return nil, err
			}
			maskedMap[field] = maskedValue
		} else {
			// Apply default masking based on classification
			maskedMap[field] = dpm.applyDefaultMasking(value, classification)
		}
	}

	return maskedMap, nil
}

// applyMaskingRule applies a specific masking rule to a value
func (dpm *DataProtectionManager) applyMaskingRule(value any, rule MaskingRule) (any, error) {
	strValue, ok := value.(string)
	if !ok {
		return value, nil // Don't mask non-string values
	}

	switch rule.Type {
	case "full":
		return strings.Repeat(rule.Replacement, len(strValue)), nil

	case "partial":
		if len(strValue) <= 4 {
			return strings.Repeat(rule.Replacement, len(strValue)), nil
		}
		// Show first 2 and last 2 characters
		return strValue[:2] + strings.Repeat(rule.Replacement, len(strValue)-4) + strValue[len(strValue)-2:], nil

	case "hash":
		hash := sha256.Sum256([]byte(strValue))
		return base64.StdEncoding.EncodeToString(hash[:]), nil

	case "tokenize":
		token, err := dpm.tokenizer.Tokenize(strValue)
		if err != nil {
			return nil, err
		}
		return token, nil

	default:
		return value, nil
	}
}

// applyDefaultMasking applies default masking based on classification
func (dpm *DataProtectionManager) applyDefaultMasking(value any, classification DataClassification) any {
	strValue, ok := value.(string)
	if !ok {
		return value
	}

	switch classification {
	case DataRestricted:
		// Full masking for restricted data
		return strings.Repeat("*", len(strValue))

	case DataConfidential:
		// Partial masking for confidential data
		if len(strValue) <= 4 {
			return strings.Repeat("*", len(strValue))
		}
		return strValue[:2] + strings.Repeat("*", len(strValue)-4) + strValue[len(strValue)-2:]

	case DataInternal:
		// Internal data is not masked by default
		return value

	case DataPublic:
		// Public data is not masked - this includes IP addresses
		return value

	default:
		// Default to no masking
		return value
	}
}

// NewAESEncryptor creates a new AES encryptor
func NewAESEncryptor(keyString string) (*AESEncryptor, error) {
	if strings.TrimSpace(keyString) == "" {
		return nil, fmt.Errorf("encryption key cannot be empty")
	}

	// Generate key from string
	hash := sha256.Sum256([]byte(keyString))

	return &AESEncryptor{
		key: hash[:],
	}, nil
}

// Encrypt encrypts data using AES
func (e *AESEncryptor) Encrypt(data any) (string, error) {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Create cipher
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, jsonData, nil)

	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts data using AES
func (e *AESEncryptor) Decrypt(encryptedData string, result any) error {
	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return err
	}

	// Create cipher
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return fmt.Errorf("invalid encrypted data format")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	// Unmarshal JSON
	return json.Unmarshal(plaintext, result)
}

// NewDataTokenizer creates a new data tokenizer
func NewDataTokenizer() *DataTokenizer {
	return &DataTokenizer{
		tokens: make(map[string]string),
		data:   make(map[string]string),
	}
}

// Tokenize creates a token for sensitive data
func (dt *DataTokenizer) Tokenize(data string) (string, error) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Check if already tokenized
	if token, exists := dt.tokens[data]; exists {
		return token, nil
	}

	// Generate random token
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}

	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Store mapping
	dt.tokens[data] = token
	dt.data[token] = data

	return token, nil
}

// Detokenize retrieves original data from token
func (dt *DataTokenizer) Detokenize(token string) (string, error) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	data, exists := dt.data[token]
	if !exists {
		return "", fmt.Errorf("invalid or expired token")
	}

	return data, nil
}

// DataProtection creates middleware for data protection
func DataProtection(config DataProtectionConfig) LiftMiddleware {
	manager, err := NewDataProtectionManager(config)
	if err != nil {
		// Return a middleware that always returns an error
		return func(_ LiftHandler) LiftHandler {
			return LiftHandlerFunc(func(_ LiftContext) error {
				// Don't expose internal implementation details
				return fmt.Errorf("data protection service unavailable: configuration error")
			})
		}
	}

	return func(next LiftHandler) LiftHandler {
		return LiftHandlerFunc(func(ctx LiftContext) error {
			// Store data protection manager in context
			ctx.Set("data_protection", manager)

			// Execute handler
			return next.Handle(ctx)
		})
	}
}

// GetDataProtectionManager retrieves the data protection manager from context
func GetDataProtectionManager(ctx LiftContext) (*DataProtectionManager, error) {
	manager, ok := ctx.Get("data_protection").(*DataProtectionManager)
	if !ok {
		return nil, fmt.Errorf("data protection manager not found in context")
	}
	return manager, nil
}
