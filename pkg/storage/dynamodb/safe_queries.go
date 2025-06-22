package dynamodb

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
)

// toString converts a value to string for string operations
func toString(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// Safe attribute name validation
var safeAttributeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func ValidateAttributeName(name string) error {
	if !safeAttributeRegex.MatchString(name) {
		return fmt.Errorf("invalid attribute name: %s", name)
	}
	return nil
}

// SafeQueryBuilder provides a safe way to build DynamoDB queries
type SafeQueryBuilder struct {
	keyCondition  expression.KeyConditionBuilder
	filter        expression.ConditionBuilder
	projection    expression.ProjectionBuilder
	hasKey        bool
	hasFilter     bool
	hasProjection bool
}

func NewSafeQuery() *SafeQueryBuilder {
	return &SafeQueryBuilder{}
}

func (q *SafeQueryBuilder) WithKey(attribute string, value interface{}) error {
	if err := ValidateAttributeName(attribute); err != nil {
		return err
	}

	key := expression.Key(attribute)
	q.keyCondition = key.Equal(expression.Value(value))
	q.hasKey = true
	return nil
}

func (q *SafeQueryBuilder) WithKeyRange(attribute string, startValue, endValue interface{}) error {
	if err := ValidateAttributeName(attribute); err != nil {
		return err
	}

	key := expression.Key(attribute)
	q.keyCondition = key.Between(expression.Value(startValue), expression.Value(endValue))
	q.hasKey = true
	return nil
}

func (q *SafeQueryBuilder) WithFilter(attribute string, op string, value interface{}) error {
	if err := ValidateAttributeName(attribute); err != nil {
		return err
	}

	attr := expression.Name(attribute)

	switch op {
	case "=":
		q.filter = attr.Equal(expression.Value(value))
	case "!=":
		q.filter = attr.NotEqual(expression.Value(value))
	case ">":
		q.filter = attr.GreaterThan(expression.Value(value))
	case ">=":
		q.filter = attr.GreaterThanEqual(expression.Value(value))
	case "<":
		q.filter = attr.LessThan(expression.Value(value))
	case "<=":
		q.filter = attr.LessThanEqual(expression.Value(value))
	case "contains":
		q.filter = attr.Contains(toString(value))
	case "begins_with":
		q.filter = attr.BeginsWith(toString(value))
	case "exists":
		q.filter = attr.AttributeExists()
	case "not_exists":
		q.filter = attr.AttributeNotExists()
	default:
		return fmt.Errorf("unsupported operator: %s", op)
	}

	q.hasFilter = true
	return nil
}

func (q *SafeQueryBuilder) AddFilter(attribute string, op string, value interface{}) error {
	if err := ValidateAttributeName(attribute); err != nil {
		return err
	}

	attr := expression.Name(attribute)
	var newCondition expression.ConditionBuilder

	switch op {
	case "=":
		newCondition = attr.Equal(expression.Value(value))
	case "!=":
		newCondition = attr.NotEqual(expression.Value(value))
	case ">":
		newCondition = attr.GreaterThan(expression.Value(value))
	case ">=":
		newCondition = attr.GreaterThanEqual(expression.Value(value))
	case "<":
		newCondition = attr.LessThan(expression.Value(value))
	case "<=":
		newCondition = attr.LessThanEqual(expression.Value(value))
	case "contains":
		newCondition = attr.Contains(toString(value))
	case "begins_with":
		newCondition = attr.BeginsWith(toString(value))
	case "exists":
		newCondition = attr.AttributeExists()
	case "not_exists":
		newCondition = attr.AttributeNotExists()
	default:
		return fmt.Errorf("unsupported operator: %s", op)
	}

	if q.hasFilter {
		q.filter = q.filter.And(newCondition)
	} else {
		q.filter = newCondition
		q.hasFilter = true
	}

	return nil
}

func (q *SafeQueryBuilder) WithProjection(attributes ...string) error {
	for _, attr := range attributes {
		if err := ValidateAttributeName(attr); err != nil {
			return err
		}
	}

	if len(attributes) > 0 {
		projBuilder := expression.NamesList(expression.Name(attributes[0]))
		for i := 1; i < len(attributes); i++ {
			projBuilder = projBuilder.AddNames(expression.Name(attributes[i]))
		}
		q.projection = projBuilder
		q.hasProjection = true
	}

	return nil
}

func (q *SafeQueryBuilder) Build() (expression.Expression, error) {
	builder := expression.NewBuilder()

	if q.hasKey {
		builder = builder.WithKeyCondition(q.keyCondition)
	}

	if q.hasFilter {
		builder = builder.WithFilter(q.filter)
	}

	if q.hasProjection {
		builder = builder.WithProjection(q.projection)
	}

	return builder.Build()
}

// SanitizeSearchQuery removes dangerous characters from search queries
func SanitizeSearchQuery(query string) string {
	// Remove any DynamoDB expression syntax
	dangerous := []string{
		"AND", "OR", "NOT", "BETWEEN", "IN",
		"(", ")", "[", "]", "{", "}",
		"=", "<", ">", "!", "*", "#",
		"ATTRIBUTE_EXISTS", "ATTRIBUTE_NOT_EXISTS",
		"ATTRIBUTE_TYPE", "BEGINS_WITH", "CONTAINS",
	}

	result := query
	for _, d := range dangerous {
		result = strings.ReplaceAll(result, d, "")
		result = strings.ReplaceAll(result, strings.ToLower(d), "")
	}

	// Remove multiple spaces
	result = strings.Join(strings.Fields(result), " ")

	// Limit length
	if len(result) > 100 {
		result = result[:100]
	}

	return strings.TrimSpace(result)
}

// BuildSafeUpdateExpression builds a safe update expression
func BuildSafeUpdateExpression(updates map[string]interface{}) (expression.UpdateBuilder, error) {
	var updateBuilder expression.UpdateBuilder
	first := true

	for attr, value := range updates {
		if err := ValidateAttributeName(attr); err != nil {
			return updateBuilder, err
		}

		if first {
			updateBuilder = expression.Set(expression.Name(attr), expression.Value(value))
			first = false
		} else {
			updateBuilder = updateBuilder.Set(expression.Name(attr), expression.Value(value))
		}
	}

	return updateBuilder, nil
}

// SafeScanBuilder provides safe scanning with filters
type SafeScanBuilder struct {
	filter    expression.ConditionBuilder
	hasFilter bool
}

func NewSafeScan() *SafeScanBuilder {
	return &SafeScanBuilder{}
}

func (s *SafeScanBuilder) WithFilter(attribute string, op string, value interface{}) error {
	qb := &SafeQueryBuilder{}
	if err := qb.WithFilter(attribute, op, value); err != nil {
		return err
	}
	s.filter = qb.filter
	s.hasFilter = true
	return nil
}

func (s *SafeScanBuilder) AddFilter(attribute string, op string, value interface{}) error {
	qb := &SafeQueryBuilder{filter: s.filter, hasFilter: s.hasFilter}
	if err := qb.AddFilter(attribute, op, value); err != nil {
		return err
	}
	s.filter = qb.filter
	s.hasFilter = true
	return nil
}

func (s *SafeScanBuilder) Build() (expression.Expression, error) {
	builder := expression.NewBuilder()

	if s.hasFilter {
		builder = builder.WithFilter(s.filter)
	}

	return builder.Build()
}
