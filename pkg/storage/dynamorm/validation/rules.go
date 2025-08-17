// Package validation provides validation rules and utilities for DynamORM model data integrity.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// Rule represents a validation rule
type Rule interface {
	Validate(value any) error
	Message() string
}

// ValidationError represents a validation error
//
//nolint:revive // Validation prefix clarifies this is validation-specific error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
//
//nolint:revive // Validation prefix clarifies this is validation-specific errors collection
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

func (e ValidationErrors) Error() string {
	messages := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		messages[i] = err.Error()
	}
	return strings.Join(messages, "; ")
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message string, value any) {
	e.Errors = append(e.Errors, ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	})
}

// Validator validates struct fields
type Validator struct {
	rules  map[string][]Rule
	logger *zap.Logger
}

// NewValidator creates a new validator
func NewValidator(logger *zap.Logger) *Validator {
	return &Validator{
		rules:  make(map[string][]Rule),
		logger: logger,
	}
}

// AddRule adds a validation rule for a field
func (v *Validator) AddRule(field string, rule Rule) {
	v.rules[field] = append(v.rules[field], rule)
}

// Validate validates a struct using registered rules
func (v *Validator) Validate(model any) error {
	if model == nil {
		return fmt.Errorf("model cannot be nil")
	}

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	if modelValue.Kind() != reflect.Struct {
		return fmt.Errorf("model must be a struct, got %s", modelValue.Kind())
	}

	modelType := modelValue.Type()
	var errors ValidationErrors

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldValue := modelValue.Field(i)

		// Skip unexported fields
		if !fieldValue.CanInterface() {
			continue
		}

		fieldName := field.Name
		value := fieldValue.Interface()

		// Validate using registered rules
		if rules, exists := v.rules[fieldName]; exists {
			for _, rule := range rules {
				if err := rule.Validate(value); err != nil {
					errors.Add(fieldName, err.Error(), value)
				}
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}

	return nil
}

// ValidateWithTags validates a struct using struct tags
func ValidateWithTags(model any) error {
	if model == nil {
		return fmt.Errorf("model cannot be nil")
	}

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	if modelValue.Kind() != reflect.Struct {
		return fmt.Errorf("model must be a struct, got %s", modelValue.Kind())
	}

	modelType := modelValue.Type()
	var errors ValidationErrors

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldValue := modelValue.Field(i)

		// Skip unexported fields
		if !fieldValue.CanInterface() {
			continue
		}

		tag := field.Tag.Get("validate")
		if tag == "" || tag == "-" {
			continue
		}

		fieldName := field.Name
		value := fieldValue.Interface()

		// Parse and validate using tags
		if err := validateFieldWithTag(fieldName, value, tag); err != nil {
			if validationErrors, ok := err.(ValidationErrors); ok {
				errors.Errors = append(errors.Errors, validationErrors.Errors...)
			} else {
				errors.Add(fieldName, err.Error(), value)
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}

	return nil
}

// validateFieldWithTag validates a field using its validation tag
func validateFieldWithTag(fieldName string, value any, tag string) error {
	var errors ValidationErrors

	// Split tag by comma to get individual rules
	rules := strings.Split(tag, ",")

	for _, ruleStr := range rules {
		ruleStr = strings.TrimSpace(ruleStr)
		if err := common.ValidateRequiredParam("ruleStr", ruleStr); err != nil {
			continue
		}

		var rule Rule
		var err error

		// Parse rule with parameters
		if strings.Contains(ruleStr, "=") {
			parts := strings.SplitN(ruleStr, "=", 2)
			ruleName := strings.TrimSpace(parts[0])
			ruleParam := strings.TrimSpace(parts[1])

			rule, err = createRuleWithParam(ruleName, ruleParam)
		} else {
			rule, err = createRule(ruleStr)
		}

		if err != nil {
			errors.Add(fieldName, fmt.Sprintf("invalid validation rule: %s", err.Error()), value)
			continue
		}

		if rule != nil {
			if err := rule.Validate(value); err != nil {
				errors.Add(fieldName, err.Error(), value)
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}

	return nil
}

// Built-in validation rules

// RequiredRule validates that a value is not empty/zero
type RequiredRule struct{}

// Validate checks that the value is not empty or zero
func (r RequiredRule) Validate(value any) error {
	if isZero(value) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for RequiredRule
func (r RequiredRule) Message() string {
	return "field is required"
}

// MinLengthRule validates minimum string length
type MinLengthRule struct {
	MinLength int
}

// Validate checks that the string value meets the minimum length requirement
func (r MinLengthRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	if len(str) < r.MinLength {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for MinLengthRule
func (r MinLengthRule) Message() string {
	return fmt.Sprintf("must be at least %d characters long", r.MinLength)
}

// MaxLengthRule validates maximum string length
type MaxLengthRule struct {
	MaxLength int
}

// Validate checks that the string or slice value does not exceed the maximum length
func (r MaxLengthRule) Validate(value any) error {
	switch v := value.(type) {
	case string:
		if len(v) > r.MaxLength {
			return errors.New(r.Message())
		}
	case []string:
		if len(v) > r.MaxLength {
			return fmt.Errorf("must have at most %d elements", r.MaxLength)
		}
	default:
		return fmt.Errorf("value must be a string or []string")
	}
	return nil
}

// Message returns the validation error message for MaxLengthRule
func (r MaxLengthRule) Message() string {
	return fmt.Sprintf("must be at most %d characters long", r.MaxLength)
}

// LengthRule validates exact string length
type LengthRule struct {
	Length int
}

// Validate checks that the string value has exactly the required length
func (r LengthRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	if len(str) != r.Length {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for LengthRule
func (r LengthRule) Message() string {
	return fmt.Sprintf("must be exactly %d characters long", r.Length)
}

// PatternRule validates using a regular expression
type PatternRule struct {
	Pattern *regexp.Regexp
	message string
}

// Validate checks that the string value matches the regular expression pattern
func (r PatternRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	if !r.Pattern.MatchString(str) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for PatternRule
func (r PatternRule) Message() string {
	if r.message != "" {
		return r.message
	}
	return "does not match required pattern"
}

// EmailRule validates email format
type EmailRule struct{}

// Validate checks that the string value is a valid email address format
func (r EmailRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	emailPattern := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailPattern.MatchString(str) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for EmailRule
func (r EmailRule) Message() string {
	return "must be a valid email address"
}

// URLRule validates URL format
type URLRule struct{}

// Validate checks that the string value is a valid URL format
func (r URLRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	urlPattern := regexp.MustCompile(`^https?://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(/.*)?$`)
	if !urlPattern.MatchString(str) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for URLRule
func (r URLRule) Message() string {
	return "must be a valid URL"
}

// MinRule validates minimum numeric value
type MinRule struct {
	Min float64
}

// Validate checks that the numeric value meets the minimum requirement
func (r MinRule) Validate(value any) error {
	num, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("value must be numeric")
	}

	if num < r.Min {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for MinRule
func (r MinRule) Message() string {
	return fmt.Sprintf("must be at least %g", r.Min)
}

// MaxRule validates maximum numeric value
type MaxRule struct {
	Max float64
}

// Validate checks that the numeric value does not exceed the maximum requirement
func (r MaxRule) Validate(value any) error {
	num, err := toFloat64(value)
	if err != nil {
		return fmt.Errorf("value must be numeric")
	}

	if num > r.Max {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for MaxRule
func (r MaxRule) Message() string {
	return fmt.Sprintf("must be at most %g", r.Max)
}

// InRule validates that value is in a list of allowed values
type InRule struct {
	AllowedValues []string
}

// Validate checks that the value is in the list of allowed values
func (r InRule) Validate(value any) error {
	str := fmt.Sprintf("%v", value)

	for _, allowed := range r.AllowedValues {
		if str == allowed {
			return nil
		}
	}

	return errors.New(r.Message())
}

// Message returns the validation error message for InRule
func (r InRule) Message() string {
	return fmt.Sprintf("must be one of: %s", strings.Join(r.AllowedValues, ", "))
}

// NotInRule validates that value is not in a list of disallowed values
type NotInRule struct {
	DisallowedValues []string
}

// Validate checks that the value is not in the list of disallowed values
func (r NotInRule) Validate(value any) error {
	str := fmt.Sprintf("%v", value)

	for _, disallowed := range r.DisallowedValues {
		if str == disallowed {
			return errors.New(r.Message())
		}
	}

	return nil
}

// Message returns the validation error message for NotInRule
func (r NotInRule) Message() string {
	return fmt.Sprintf("must not be one of: %s", strings.Join(r.DisallowedValues, ", "))
}

// AlphaRule validates that string contains only alphabetic characters
type AlphaRule struct{}

// Validate checks that the string contains only alphabetic characters
func (r AlphaRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	alphaPattern := regexp.MustCompile(`^[a-zA-Z]+$`)
	if !alphaPattern.MatchString(str) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for AlphaRule
func (r AlphaRule) Message() string {
	return "must contain only alphabetic characters"
}

// AlphaNumRule validates that string contains only alphanumeric characters
type AlphaNumRule struct{}

// Validate checks that the string contains only alphanumeric characters
func (r AlphaNumRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	alphaNumPattern := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	if !alphaNumPattern.MatchString(str) {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for AlphaNumRule
func (r AlphaNumRule) Message() string {
	return "must contain only alphanumeric characters"
}

// NumericRule validates that value is numeric
type NumericRule struct{}

// Validate checks that the value is numeric
func (r NumericRule) Validate(value any) error {
	_, err := toFloat64(value)
	if err != nil {
		return errors.New(r.Message())
	}
	return nil
}

// Message returns the validation error message for NumericRule
func (r NumericRule) Message() string {
	return "must be numeric"
}

// IntegerRule validates that value is an integer
type IntegerRule struct{}

// Validate checks that the value is an integer
func (r IntegerRule) Validate(value any) error {
	switch v := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case float32, float64:
		num, _ := toFloat64(value)
		if num == float64(int64(num)) {
			return nil
		}
	case string:
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			return nil
		}
	}

	return errors.New(r.Message())
}

// Message returns the validation error message for IntegerRule
func (r IntegerRule) Message() string {
	return "must be an integer"
}

// DateRule validates that value is a valid date
type DateRule struct{}

// Validate checks that the value is a valid date
func (r DateRule) Validate(value any) error {
	switch v := value.(type) {
	case time.Time:
		return nil
	case string:
		layouts := []string{
			common.DateFormat,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05-07:00",
			"2006-01-02 15:04:05",
		}

		for _, layout := range layouts {
			if _, err := time.Parse(layout, v); err == nil {
				return nil
			}
		}
	}

	return errors.New(r.Message())
}

// Message returns the validation error message for DateRule
func (r DateRule) Message() string {
	return "must be a valid date"
}

// Helper functions

// createRule creates a rule without parameters
func createRule(ruleName string) (Rule, error) {
	switch ruleName {
	case "required":
		return RequiredRule{}, nil
	case "email":
		return EmailRule{}, nil
	case "url":
		return URLRule{}, nil
	case "alpha":
		return AlphaRule{}, nil
	case "alphanum":
		return AlphaNumRule{}, nil
	case "numeric":
		return NumericRule{}, nil
	case "integer":
		return IntegerRule{}, nil
	case "date":
		return DateRule{}, nil
	default:
		return nil, fmt.Errorf("unknown rule: %s", ruleName)
	}
}

// createRuleWithParam creates a rule with parameters
func createRuleWithParam(ruleName, param string) (Rule, error) {
	switch ruleName {
	case "min":
		minVal, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid min parameter: %s", param)
		}
		return MinRule{Min: minVal}, nil

	case "max":
		maxVal, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid max parameter: %s", param)
		}
		return MaxRule{Max: maxVal}, nil

	case "minlen":
		minLen, err := strconv.Atoi(param)
		if err != nil {
			return nil, fmt.Errorf("invalid minlen parameter: %s", param)
		}
		return MinLengthRule{MinLength: minLen}, nil

	case "maxlen":
		maxLen, err := strconv.Atoi(param)
		if err != nil {
			return nil, fmt.Errorf("invalid maxlen parameter: %s", param)
		}
		return MaxLengthRule{MaxLength: maxLen}, nil

	case "len":
		length, err := strconv.Atoi(param)
		if err != nil {
			return nil, fmt.Errorf("invalid len parameter: %s", param)
		}
		return LengthRule{Length: length}, nil

	case "pattern":
		pattern, err := regexp.Compile(param)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern: %s", err.Error())
		}
		return PatternRule{Pattern: pattern, message: "does not match required pattern"}, nil

	case "in":
		values := strings.Split(param, "|")
		return InRule{AllowedValues: values}, nil

	case "notin":
		values := strings.Split(param, "|")
		return NotInRule{DisallowedValues: values}, nil

	default:
		return nil, fmt.Errorf("unknown rule with parameter: %s", ruleName)
	}
}

// isZero checks if a value is the zero value for its type
func isZero(value any) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Struct:
		// For time.Time, check if it's the zero time
		if t, ok := value.(time.Time); ok {
			return t.IsZero()
		}
		// For other structs, use reflection to check if all fields are zero
		return v.IsZero()
	default:
		return false
	}
}

// toFloat64 converts various numeric types to float64
func toFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

// ValidationConstraints provides predefined validation constraints for common model fields
//
//nolint:revive // Validation prefix clarifies this is validation-specific constraints
type ValidationConstraints struct {
	Username struct {
		MinLength int
		MaxLength int
		Pattern   *regexp.Regexp
	}
	Email struct {
		MaxLength int
	}
	Password struct {
		MinLength      int
		RequireUpper   bool
		RequireLower   bool
		RequireDigit   bool
		RequireSpecial bool
	}
	Content struct {
		MaxLength int
	}
}

// DefaultConstraints provides default validation constraints for the Lesser project
func DefaultConstraints() ValidationConstraints {
	return ValidationConstraints{
		Username: struct {
			MinLength int
			MaxLength int
			Pattern   *regexp.Regexp
		}{
			MinLength: 3,
			MaxLength: 30,
			Pattern:   regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		},
		Email: struct {
			MaxLength int
		}{
			MaxLength: 255,
		},
		Password: struct {
			MinLength      int
			RequireUpper   bool
			RequireLower   bool
			RequireDigit   bool
			RequireSpecial bool
		}{
			MinLength:      8,
			RequireUpper:   true,
			RequireLower:   true,
			RequireDigit:   true,
			RequireSpecial: false, // Optional for better UX
		},
		Content: struct {
			MaxLength int
		}{
			MaxLength: 500, // Status content limit
		},
	}
}

// Custom validation rules for the Lesser project

// UsernameRule validates usernames according to Lesser's requirements
type UsernameRule struct {
	Constraints ValidationConstraints
}

// Validate checks that the username meets Lesser's requirements
func (r UsernameRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("username must be a string")
	}

	if len(str) < r.Constraints.Username.MinLength {
		return fmt.Errorf("username must be at least %d characters long", r.Constraints.Username.MinLength)
	}

	if len(str) > r.Constraints.Username.MaxLength {
		return fmt.Errorf("username must be at most %d characters long", r.Constraints.Username.MaxLength)
	}

	if !r.Constraints.Username.Pattern.MatchString(str) {
		return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
	}

	return nil
}

// Message returns the validation error message for UsernameRule
func (r UsernameRule) Message() string {
	return "invalid username format"
}

// PasswordRule validates passwords according to Lesser's security requirements
type PasswordRule struct {
	Constraints ValidationConstraints
}

// Validate checks that the password meets Lesser's security requirements
func (r PasswordRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("password must be a string")
	}

	if len(str) < r.Constraints.Password.MinLength {
		return fmt.Errorf("password must be at least %d characters long", r.Constraints.Password.MinLength)
	}

	if r.Constraints.Password.RequireUpper && !regexp.MustCompile(`[A-Z]`).MatchString(str) {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}

	if r.Constraints.Password.RequireLower && !regexp.MustCompile(`[a-z]`).MatchString(str) {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}

	if r.Constraints.Password.RequireDigit && !regexp.MustCompile(`[0-9]`).MatchString(str) {
		return fmt.Errorf("password must contain at least one digit")
	}

	if r.Constraints.Password.RequireSpecial && !regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(str) {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}

// Message returns the validation error message for PasswordRule
func (r PasswordRule) Message() string {
	return "password does not meet security requirements"
}

// ContentRule validates status content according to Lesser's requirements
type ContentRule struct {
	Constraints ValidationConstraints
}

// Validate checks that the content meets Lesser's requirements
func (r ContentRule) Validate(value any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("content must be a string")
	}

	// Content can be empty (for image-only posts, etc.)
	if len(str) > r.Constraints.Content.MaxLength {
		return fmt.Errorf("content must be at most %d characters long", r.Constraints.Content.MaxLength)
	}

	return nil
}

// Message returns the validation error message for ContentRule
func (r ContentRule) Message() string {
	return "content exceeds maximum length"
}

// Validation utilities for the Lift framework integration

// ValidateRequest validates a request payload using struct tags
func ValidateRequest(request any) error {
	return ValidateWithTags(request)
}

// ValidateModel validates a DynamORM model before saving
func ValidateModel(model any) error {
	return ValidateWithTags(model)
}

// CreateModelValidator creates a validator with common model rules
func CreateModelValidator(logger *zap.Logger) *Validator {
	validator := NewValidator(logger)
	constraints := DefaultConstraints()

	// Add common validation rules
	validator.AddRule("Username", UsernameRule{Constraints: constraints})
	validator.AddRule("Email", EmailRule{})
	validator.AddRule("Password", PasswordRule{Constraints: constraints})
	validator.AddRule("Content", ContentRule{Constraints: constraints})

	return validator
}
