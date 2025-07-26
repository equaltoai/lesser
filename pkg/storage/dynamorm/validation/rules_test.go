package validation

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Test models

type TestUser struct {
	Username string    `validate:"required,minlen=3,maxlen=30,pattern=^[a-zA-Z0-9_-]+$"`
	Email    string    `validate:"required,email"`
	Age      int       `validate:"min=13,max=120"`
	Bio      string    `validate:"maxlen=500"`
	Score    float64   `validate:"min=0,max=100"`
	Role     string    `validate:"in=admin|user|moderator"`
	Status   string    `validate:"notin=banned|suspended"`
	Active   bool      `validate:"required"`
	Tags     []string  `validate:"maxlen=5"`
	JoinedAt time.Time `validate:"date"`
}

type TestProduct struct {
	Name        string  `validate:"required,minlen=1,maxlen=100"`
	Price       float64 `validate:"required,min=0.01"`
	Category    string  `validate:"required,in=electronics|books|clothing"`
	SKU         string  `validate:"required,len=8,alphanum"`
	Description string  `validate:"maxlen=1000"`
	InStock     bool
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "username",
		Message: "is required",
		Value:   "",
	}

	expected := "validation failed for field 'username': is required"
	assert.Equal(t, expected, err.Error())
}

func TestValidationErrors_Error(t *testing.T) {
	errors := ValidationErrors{
		Errors: []ValidationError{
			{Field: "username", Message: "is required"},
			{Field: "email", Message: "invalid format"},
		},
	}

	result := errors.Error()
	assert.Contains(t, result, "username")
	assert.Contains(t, result, "email")
	assert.Contains(t, result, "is required")
	assert.Contains(t, result, "invalid format")
}

func TestValidationErrors_HasErrors(t *testing.T) {
	errors := ValidationErrors{}
	assert.False(t, errors.HasErrors())

	errors.Add("field", "message", "value")
	assert.True(t, errors.HasErrors())
}

func TestValidationErrors_Add(t *testing.T) {
	errors := ValidationErrors{}
	errors.Add("username", "is required", "")

	assert.Len(t, errors.Errors, 1)
	assert.Equal(t, "username", errors.Errors[0].Field)
	assert.Equal(t, "is required", errors.Errors[0].Message)
	assert.Equal(t, "", errors.Errors[0].Value)
}

func TestNewValidator(t *testing.T) {
	logger := zap.NewNop()
	validator := NewValidator(logger)

	assert.NotNil(t, validator)
	assert.NotNil(t, validator.rules)
	assert.Equal(t, logger, validator.logger)
}

func TestValidator_AddRule(t *testing.T) {
	validator := NewValidator(zap.NewNop())
	rule := RequiredRule{}

	validator.AddRule("username", rule)

	assert.Len(t, validator.rules["username"], 1)
}

func TestValidator_Validate_Success(t *testing.T) {
	validator := NewValidator(zap.NewNop())
	validator.AddRule("Username", RequiredRule{})
	validator.AddRule("Email", EmailRule{})

	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
	}

	err := validator.Validate(user)
	assert.NoError(t, err)
}

func TestValidator_Validate_Failure(t *testing.T) {
	validator := NewValidator(zap.NewNop())
	validator.AddRule("Username", RequiredRule{})
	validator.AddRule("Email", EmailRule{})

	user := TestUser{
		Username: "", // Required field empty
		Email:    "invalid-email",
	}

	err := validator.Validate(user)
	assert.Error(t, err)

	validationErrors, ok := err.(ValidationErrors)
	assert.True(t, ok)
	assert.Len(t, validationErrors.Errors, 2)
}

func TestValidator_Validate_NilModel(t *testing.T) {
	validator := NewValidator(zap.NewNop())
	err := validator.Validate(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestValidator_Validate_NonStruct(t *testing.T) {
	validator := NewValidator(zap.NewNop())
	err := validator.Validate("not a struct")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a struct")
}

func TestValidateWithTags_Success(t *testing.T) {
	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
		Age:      25,
		Bio:      "Short bio",
		Score:    85.5,
		Role:     "user",
		Status:   "active",
		Active:   true,
		Tags:     []string{"tag1", "tag2"},
		JoinedAt: time.Now(),
	}

	err := ValidateWithTags(user)
	assert.NoError(t, err)
}

func TestValidateWithTags_Failures(t *testing.T) {
	user := TestUser{
		Username: "x",                  // Too short
		Email:    "invalid-email",      // Invalid email
		Age:      5,                    // Too young
		Score:    150,                  // Too high
		Role:     "invalid",            // Not in allowed list
		Status:   "banned",             // In disallowed list
		JoinedAt: time.Time{},          // Zero time should be valid for date rule
	}

	err := ValidateWithTags(user)
	assert.Error(t, err)

	validationErrors, ok := err.(ValidationErrors)
	assert.True(t, ok)
	assert.True(t, len(validationErrors.Errors) >= 5) // Should have multiple errors
}

// Test individual rules

func TestRequiredRule(t *testing.T) {
	rule := RequiredRule{}

	// Test valid values
	assert.NoError(t, rule.Validate("not empty"))
	assert.NoError(t, rule.Validate(123))
	assert.NoError(t, rule.Validate(true))

	// Test invalid values
	assert.Error(t, rule.Validate(""))
	assert.Error(t, rule.Validate(0))
	assert.Error(t, rule.Validate(false))
	assert.Error(t, rule.Validate(nil))
}

func TestMinLengthRule(t *testing.T) {
	rule := MinLengthRule{MinLength: 5}

	// Test valid values
	assert.NoError(t, rule.Validate("12345"))
	assert.NoError(t, rule.Validate("123456"))

	// Test invalid values
	assert.Error(t, rule.Validate("1234"))
	assert.Error(t, rule.Validate(123)) // Not a string
}

func TestMaxLengthRule(t *testing.T) {
	rule := MaxLengthRule{MaxLength: 5}

	// Test valid values
	assert.NoError(t, rule.Validate("12345"))
	assert.NoError(t, rule.Validate("1234"))

	// Test invalid values
	assert.Error(t, rule.Validate("123456"))
	assert.Error(t, rule.Validate(123)) // Not a string
}

func TestLengthRule(t *testing.T) {
	rule := LengthRule{Length: 5}

	// Test valid values
	assert.NoError(t, rule.Validate("12345"))

	// Test invalid values
	assert.Error(t, rule.Validate("1234"))
	assert.Error(t, rule.Validate("123456"))
	assert.Error(t, rule.Validate(123)) // Not a string
}

func TestPatternRule(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	rule := PatternRule{Pattern: pattern, message: "invalid format"}

	// Test valid values
	assert.NoError(t, rule.Validate("abc123"))
	assert.NoError(t, rule.Validate("ABC"))

	// Test invalid values
	assert.Error(t, rule.Validate("abc-123"))
	assert.Error(t, rule.Validate("abc@123"))
	assert.Error(t, rule.Validate(123)) // Not a string
}

func TestEmailRule(t *testing.T) {
	rule := EmailRule{}

	// Test valid emails
	validEmails := []string{
		"test@example.com",
		"user.name@domain.co.uk",
		"user+tag@example.org",
		"123@domain.net",
	}

	for _, email := range validEmails {
		assert.NoError(t, rule.Validate(email), "Email should be valid: %s", email)
	}

	// Test invalid emails
	invalidEmails := []string{
		"invalid-email",
		"@domain.com",
		"user@",
		"user@domain",
		"user.domain.com",
		"",
	}

	for _, email := range invalidEmails {
		assert.Error(t, rule.Validate(email), "Email should be invalid: %s", email)
	}

	// Test non-string value
	assert.Error(t, rule.Validate(123))
}

func TestURLRule(t *testing.T) {
	rule := URLRule{}

	// Test valid URLs
	validURLs := []string{
		"http://example.com",
		"https://example.com",
		"https://www.example.com/path",
		"http://sub.domain.co.uk/path/to/resource",
	}

	for _, url := range validURLs {
		assert.NoError(t, rule.Validate(url), "URL should be valid: %s", url)
	}

	// Test invalid URLs
	invalidURLs := []string{
		"not-a-url",
		"ftp://example.com", // Wrong protocol
		"example.com",       // Missing protocol
		"http://",           // Incomplete
		"",
	}

	for _, url := range invalidURLs {
		assert.Error(t, rule.Validate(url), "URL should be invalid: %s", url)
	}

	// Test non-string value
	assert.Error(t, rule.Validate(123))
}

func TestMinRule(t *testing.T) {
	rule := MinRule{Min: 10}

	// Test valid values
	assert.NoError(t, rule.Validate(10))
	assert.NoError(t, rule.Validate(15))
	assert.NoError(t, rule.Validate(10.5))
	assert.NoError(t, rule.Validate("15"))

	// Test invalid values
	assert.Error(t, rule.Validate(5))
	assert.Error(t, rule.Validate(9.9))
	assert.Error(t, rule.Validate("not-a-number"))
}

func TestMaxRule(t *testing.T) {
	rule := MaxRule{Max: 100}

	// Test valid values
	assert.NoError(t, rule.Validate(100))
	assert.NoError(t, rule.Validate(50))
	assert.NoError(t, rule.Validate(99.9))
	assert.NoError(t, rule.Validate("50"))

	// Test invalid values
	assert.Error(t, rule.Validate(101))
	assert.Error(t, rule.Validate(100.1))
	assert.Error(t, rule.Validate("not-a-number"))
}

func TestInRule(t *testing.T) {
	rule := InRule{AllowedValues: []string{"admin", "user", "moderator"}}

	// Test valid values
	assert.NoError(t, rule.Validate("admin"))
	assert.NoError(t, rule.Validate("user"))
	assert.NoError(t, rule.Validate("moderator"))

	// Test invalid values
	assert.Error(t, rule.Validate("guest"))
	assert.Error(t, rule.Validate(""))
	assert.Error(t, rule.Validate("ADMIN")) // Case sensitive
}

func TestNotInRule(t *testing.T) {
	rule := NotInRule{DisallowedValues: []string{"banned", "suspended"}}

	// Test valid values
	assert.NoError(t, rule.Validate("active"))
	assert.NoError(t, rule.Validate("pending"))
	assert.NoError(t, rule.Validate(""))

	// Test invalid values
	assert.Error(t, rule.Validate("banned"))
	assert.Error(t, rule.Validate("suspended"))
}

func TestAlphaRule(t *testing.T) {
	rule := AlphaRule{}

	// Test valid values
	assert.NoError(t, rule.Validate("abc"))
	assert.NoError(t, rule.Validate("ABC"))
	assert.NoError(t, rule.Validate("AbCdEf"))

	// Test invalid values
	assert.Error(t, rule.Validate("abc123"))
	assert.Error(t, rule.Validate("abc-def"))
	assert.Error(t, rule.Validate(""))
	assert.Error(t, rule.Validate(123))
}

func TestAlphaNumRule(t *testing.T) {
	rule := AlphaNumRule{}

	// Test valid values
	assert.NoError(t, rule.Validate("abc"))
	assert.NoError(t, rule.Validate("123"))
	assert.NoError(t, rule.Validate("abc123"))
	assert.NoError(t, rule.Validate("ABC123"))

	// Test invalid values
	assert.Error(t, rule.Validate("abc-123"))
	assert.Error(t, rule.Validate("abc@123"))
	assert.Error(t, rule.Validate(""))
	assert.Error(t, rule.Validate(123))
}

func TestNumericRule(t *testing.T) {
	rule := NumericRule{}

	// Test valid values
	assert.NoError(t, rule.Validate(123))
	assert.NoError(t, rule.Validate(123.45))
	assert.NoError(t, rule.Validate("123"))
	assert.NoError(t, rule.Validate("123.45"))

	// Test invalid values
	assert.Error(t, rule.Validate("abc"))
	assert.Error(t, rule.Validate("123abc"))
	assert.Error(t, rule.Validate(true))
}

func TestIntegerRule(t *testing.T) {
	rule := IntegerRule{}

	// Test valid values
	assert.NoError(t, rule.Validate(123))
	assert.NoError(t, rule.Validate(int64(123)))
	assert.NoError(t, rule.Validate(uint(123)))
	assert.NoError(t, rule.Validate(float64(123)))
	assert.NoError(t, rule.Validate("123"))

	// Test invalid values
	assert.Error(t, rule.Validate(123.45))
	assert.Error(t, rule.Validate("123.45"))
	assert.Error(t, rule.Validate("abc"))
	assert.Error(t, rule.Validate(true))
}

func TestDateRule(t *testing.T) {
	rule := DateRule{}

	// Test valid values
	assert.NoError(t, rule.Validate(time.Now()))
	assert.NoError(t, rule.Validate("2023-01-01"))
	assert.NoError(t, rule.Validate("2023-01-01T12:00:00Z"))
	assert.NoError(t, rule.Validate("2023-01-01 12:00:00"))

	// Test invalid values
	assert.Error(t, rule.Validate("not-a-date"))
	assert.Error(t, rule.Validate("2023-13-01")) // Invalid month
	assert.Error(t, rule.Validate(123))
}

// Test helper functions

func TestIsZero(t *testing.T) {
	// Test zero values
	assert.True(t, isZero(""))
	assert.True(t, isZero(0))
	assert.True(t, isZero(0.0))
	assert.True(t, isZero(false))
	assert.True(t, isZero([]string{}))
	assert.True(t, isZero(map[string]string{}))
	assert.True(t, isZero((*string)(nil)))
	assert.True(t, isZero(time.Time{}))

	// Test non-zero values
	assert.False(t, isZero("hello"))
	assert.False(t, isZero(1))
	assert.False(t, isZero(0.1))
	assert.False(t, isZero(true))
	assert.False(t, isZero([]string{"item"}))
	assert.False(t, isZero(map[string]string{"key": "value"}))
	assert.False(t, isZero(time.Now()))
}

func TestToFloat64(t *testing.T) {
	// Test valid conversions
	tests := []struct {
		input    any
		expected float64
	}{
		{int(123), 123.0},
		{int64(123), 123.0},
		{float32(123.45), 123.45},
		{float64(123.45), 123.45},
		{"123", 123.0},
		{"123.45", 123.45},
	}

	for _, test := range tests {
		result, err := toFloat64(test.input)
		assert.NoError(t, err)
		assert.InDelta(t, test.expected, result, 0.01)
	}

	// Test invalid conversions
	_, err := toFloat64("not-a-number")
	assert.Error(t, err)

	_, err = toFloat64(true)
	assert.Error(t, err)
}

// Test rule creation functions

func TestCreateRule(t *testing.T) {
	tests := []struct {
		ruleName    string
		expectedType string
	}{
		{"required", "RequiredRule"},
		{"email", "EmailRule"},
		{"url", "URLRule"},
		{"alpha", "AlphaRule"},
		{"alphanum", "AlphaNumRule"},
		{"numeric", "NumericRule"},
		{"integer", "IntegerRule"},
		{"date", "DateRule"},
	}

	for _, test := range tests {
		rule, err := createRule(test.ruleName)
		assert.NoError(t, err)
		assert.Contains(t, fmt.Sprintf("%T", rule), test.expectedType)
	}

	// Test unknown rule
	_, err := createRule("unknown")
	assert.Error(t, err)
}

func TestCreateRuleWithParam(t *testing.T) {
	// Test min rule
	rule, err := createRuleWithParam("min", "10")
	assert.NoError(t, err)
	minRule, ok := rule.(MinRule)
	assert.True(t, ok)
	assert.Equal(t, 10.0, minRule.Min)

	// Test max rule
	rule, err = createRuleWithParam("max", "100")
	assert.NoError(t, err)
	maxRule, ok := rule.(MaxRule)
	assert.True(t, ok)
	assert.Equal(t, 100.0, maxRule.Max)

	// Test minlen rule
	rule, err = createRuleWithParam("minlen", "5")
	assert.NoError(t, err)
	minLenRule, ok := rule.(MinLengthRule)
	assert.True(t, ok)
	assert.Equal(t, 5, minLenRule.MinLength)

	// Test pattern rule
	rule, err = createRuleWithParam("pattern", "^[a-z]+$")
	assert.NoError(t, err)
	patternRule, ok := rule.(PatternRule)
	assert.True(t, ok)
	assert.NotNil(t, patternRule.Pattern)

	// Test in rule
	rule, err = createRuleWithParam("in", "admin|user|guest")
	assert.NoError(t, err)
	inRule, ok := rule.(InRule)
	assert.True(t, ok)
	assert.Equal(t, []string{"admin", "user", "guest"}, inRule.AllowedValues)

	// Test invalid parameter
	_, err = createRuleWithParam("min", "not-a-number")
	assert.Error(t, err)

	// Test unknown rule
	_, err = createRuleWithParam("unknown", "param")
	assert.Error(t, err)
}

// Test custom rules for Lesser project

func TestDefaultConstraints(t *testing.T) {
	constraints := DefaultConstraints()

	assert.Equal(t, 3, constraints.Username.MinLength)
	assert.Equal(t, 30, constraints.Username.MaxLength)
	assert.NotNil(t, constraints.Username.Pattern)

	assert.Equal(t, 255, constraints.Email.MaxLength)

	assert.Equal(t, 8, constraints.Password.MinLength)
	assert.True(t, constraints.Password.RequireUpper)
	assert.True(t, constraints.Password.RequireLower)
	assert.True(t, constraints.Password.RequireDigit)

	assert.Equal(t, 500, constraints.Content.MaxLength)
}

func TestUsernameRule(t *testing.T) {
	rule := UsernameRule{Constraints: DefaultConstraints()}

	// Test valid usernames
	validUsernames := []string{
		"user123",
		"test_user",
		"user-name",
		"ABC123",
		"a1b2c3",
	}

	for _, username := range validUsernames {
		assert.NoError(t, rule.Validate(username), "Username should be valid: %s", username)
	}

	// Test invalid usernames
	invalidUsernames := []string{
		"us",              // Too short
		strings.Repeat("a", 31), // Too long
		"user@name",       // Invalid character
		"user.name",       // Invalid character
		"user name",       // Space not allowed
		"",                // Empty
	}

	for _, username := range invalidUsernames {
		assert.Error(t, rule.Validate(username), "Username should be invalid: %s", username)
	}

	// Test non-string value
	assert.Error(t, rule.Validate(123))
}

func TestPasswordRule(t *testing.T) {
	rule := PasswordRule{Constraints: DefaultConstraints()}

	// Test valid passwords
	validPasswords := []string{
		"Password123",
		"MyPass1234",
		"Secure123Pass",
	}

	for _, password := range validPasswords {
		assert.NoError(t, rule.Validate(password), "Password should be valid: %s", password)
	}

	// Test invalid passwords
	invalidPasswords := []string{
		"short1A",        // Too short
		"password123",    // No uppercase
		"PASSWORD123",    // No lowercase
		"PasswordABC",    // No digit
		"",               // Empty
	}

	for _, password := range invalidPasswords {
		assert.Error(t, rule.Validate(password), "Password should be invalid: %s", password)
	}

	// Test non-string value
	assert.Error(t, rule.Validate(123))
}

func TestContentRule(t *testing.T) {
	rule := ContentRule{Constraints: DefaultConstraints()}

	// Test valid content
	assert.NoError(t, rule.Validate(""))                              // Empty is allowed
	assert.NoError(t, rule.Validate("Short content"))                 // Short content
	assert.NoError(t, rule.Validate(strings.Repeat("a", 500)))        // Exactly at limit

	// Test invalid content
	assert.Error(t, rule.Validate(strings.Repeat("a", 501)))          // Too long

	// Test non-string value
	assert.Error(t, rule.Validate(123))
}

// Test utility functions

func TestValidateRequest(t *testing.T) {
	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
		Age:      25,
		Bio:      "Short bio",
		Score:    85.5,
		Role:     "user",
		Status:   "active",
		Active:   true,
		Tags:     []string{"tag1", "tag2"},
		JoinedAt: time.Now(),
	}

	err := ValidateRequest(user)
	assert.NoError(t, err)
}

func TestValidateModel(t *testing.T) {
	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
		Age:      25,
		Bio:      "Short bio",
		Score:    85.5,
		Role:     "user",
		Status:   "active",
		Active:   true,
		Tags:     []string{"tag1", "tag2"},
		JoinedAt: time.Now(),
	}

	err := ValidateModel(user)
	assert.NoError(t, err)
}

func TestCreateModelValidator(t *testing.T) {
	logger := zap.NewNop()
	validator := CreateModelValidator(logger)

	assert.NotNil(t, validator)
	assert.NotEmpty(t, validator.rules)

	// Test that it has the expected rules
	assert.Contains(t, validator.rules, "Username")
	assert.Contains(t, validator.rules, "Email")
	assert.Contains(t, validator.rules, "Password")
	assert.Contains(t, validator.rules, "Content")
}

// Integration tests

func TestComplexValidation(t *testing.T) {
	product := TestProduct{
		Name:        "iPhone 15",
		Price:       999.99,
		Category:    "electronics",
		SKU:         "IPH15001",
		Description: "Latest iPhone model",
		InStock:     true,
	}

	err := ValidateWithTags(product)
	assert.NoError(t, err)

	// Test with invalid data
	invalidProduct := TestProduct{
		Name:        "", // Required but empty
		Price:       -10, // Negative price
		Category:    "invalid", // Not in allowed list
		SKU:         "SHORT", // Wrong length
		Description: strings.Repeat("a", 1001), // Too long
	}

	err = ValidateWithTags(invalidProduct)
	assert.Error(t, err)

	validationErrors, ok := err.(ValidationErrors)
	assert.True(t, ok)
	assert.True(t, len(validationErrors.Errors) >= 4)
}

// Benchmark tests

func BenchmarkValidateWithTags(b *testing.B) {
	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
		Age:      25,
		Bio:      "Test bio",
		Score:    85.5,
		Role:     "user",
		Status:   "active",
		Active:   true,
		JoinedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateWithTags(user)
	}
}

func BenchmarkValidator_Validate(b *testing.B) {
	validator := CreateModelValidator(zap.NewNop())
	user := TestUser{
		Username: "testuser",
		Email:    "test@example.com",
		Age:      25,
		Active:   true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.Validate(user)
	}
}

