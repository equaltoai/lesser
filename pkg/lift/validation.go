package lift

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ValidationErrors represents validation errors
type ValidationErrors struct {
	Errors map[string]string `json:"errors"`
}

// RegisterCustomValidators registers custom validators
func RegisterCustomValidators(validate *validator.Validate) {
	// Register custom validators here
	// Example:
	// validate.RegisterValidation("customtag", customValidator)
}

// FormatValidationErrors formats validation errors
func FormatValidationErrors(err error) *ValidationErrors {
	if err == nil {
		return nil
	}

	errors := make(map[string]string)

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			field := e.Field()
			// Convert field name to JSON field name (lowercase first letter)
			if len(field) > 0 {
				field = strings.ToLower(field[:1]) + field[1:]
			}
			errors[field] = formatValidationError(e)
		}
	} else {
		errors["general"] = err.Error()
	}

	return &ValidationErrors{
		Errors: errors,
	}
}

// formatValidationError formats a single validation error
func formatValidationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Invalid email address"
	case "min":
		if e.Type().Kind() == reflect.String {
			return fmt.Sprintf("Must be at least %s characters long", e.Param())
		}
		return fmt.Sprintf("Must be at least %s", e.Param())
	case "max":
		if e.Type().Kind() == reflect.String {
			return fmt.Sprintf("Must be at most %s characters long", e.Param())
		}
		return fmt.Sprintf("Must be at most %s", e.Param())
	case "len":
		if e.Type().Kind() == reflect.String {
			return fmt.Sprintf("Must be exactly %s characters long", e.Param())
		}
		return fmt.Sprintf("Must be exactly %s", e.Param())
	case "eq":
		return fmt.Sprintf("Must be equal to %s", e.Param())
	case "ne":
		return fmt.Sprintf("Must not be equal to %s", e.Param())
	case "oneof":
		return fmt.Sprintf("Must be one of: %s", e.Param())
	case "url":
		return "Must be a valid URL"
	case "uuid":
		return "Must be a valid UUID"
	case "datetime":
		return fmt.Sprintf("Must be a valid datetime in format %s", e.Param())
	default:
		return fmt.Sprintf("Failed validation on %s", e.Tag())
	}
}
