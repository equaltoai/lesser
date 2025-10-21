// Package query provides aggregate functionality for DynamoDB queries
package query

import (
	"fmt"
	"reflect"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// AggregateResult holds the result of an aggregate operation
type AggregateResult struct {
	Count   int64
	Sum     float64
	Average float64
	Min     any
	Max     any
}

// Sum calculates the sum of a numeric field
func (q *Query) Sum(field string) (float64, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	sum := 0.0
	for _, item := range items {
		value, err := extractNumericValue(item, field)
		if err != nil {
			continue // Skip invalid values
		}
		sum += value
	}

	return sum, nil
}

// Average calculates the average of a numeric field
func (q *Query) Average(field string) (float64, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	if len(items) == 0 {
		return 0, nil
	}

	sum := 0.0
	count := 0
	for _, item := range items {
		value, err := extractNumericValue(item, field)
		if err != nil {
			continue // Skip invalid values
		}
		sum += value
		count++
	}

	if count == 0 {
		return 0, nil
	}

	return sum / float64(count), nil
}

// Min finds the minimum value of a field
func (q *Query) Min(field string) (any, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var minValue any
	first := true

	for _, item := range items {
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			minValue = value
			first = false
			continue
		}

		if compareValues(value, minValue) < 0 {
			minValue = value
		}
	}

	if minValue == nil {
		return nil, fmt.Errorf("no valid values found for field %s", field)
	}

	return minValue, nil
}

// Max finds the maximum value of a field
func (q *Query) Max(field string) (any, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found")
	}

	var maxValue any
	first := true

	for _, item := range items {
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			maxValue = value
			first = false
			continue
		}

		if compareValues(value, maxValue) > 0 {
			maxValue = value
		}
	}

	if maxValue == nil {
		return nil, fmt.Errorf("no valid values found for field %s", field)
	}

	return maxValue, nil
}

// Aggregate performs multiple aggregate operations in a single pass
func (q *Query) Aggregate(fields ...string) (*AggregateResult, error) {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return nil, err
	}

	result := &AggregateResult{
		Count: int64(len(items)),
	}

	if len(fields) == 0 {
		return result, nil
	}

	// Calculate aggregates for the first field
	field := fields[0]
	sum := 0.0
	count := 0
	var minValue, maxValue any
	first := true

	for _, item := range items {
		// Try numeric operations
		numValue, err := extractNumericValue(item, field)
		if err == nil {
			sum += numValue
			count++
		}

		// Get general value for min/max
		value := extractFieldValue(item, field)
		if value == nil {
			continue
		}

		if first {
			minValue = value
			maxValue = value
			first = false
			continue
		}

		if compareValues(value, minValue) < 0 {
			minValue = value
		}
		if compareValues(value, maxValue) > 0 {
			maxValue = value
		}
	}

	result.Sum = sum
	if count > 0 {
		result.Average = sum / float64(count)
	}
	result.Min = minValue
	result.Max = maxValue

	return result, nil
}

// GroupBy groups results by a field and performs aggregate operations
type GroupedResult struct {
	Key        any
	Count      int64
	Items      []any
	Aggregates map[string]*AggregateResult
}

// GroupBy groups items by a field
func (q *Query) GroupBy(field string) *GroupByQuery {
	// Get all items
	items, err := q.getAllItems()
	if err != nil {
		return &GroupByQuery{err: err}
	}

	// Create GroupByQuery to enable chaining
	return &GroupByQuery{
		query:   q,
		items:   items,
		groupBy: field,
		groups:  make(map[string]*GroupedResult),
	}
}

// GroupByQuery enables chaining aggregate operations on grouped data
type GroupByQuery struct {
	query         *Query
	items         []any
	groupBy       string
	groups        map[string]*GroupedResult
	aggregates    []aggregateOp
	havingClauses []havingClause
	err           error
}

type aggregateOp struct {
	function string // "COUNT", "SUM", "AVG", "MIN", "MAX"
	field    string
	alias    string
}

type havingClause struct {
	aggregate string // e.g., "COUNT(*)", "SUM(price)"
	operator  string
	value     any
}

// Count adds a COUNT aggregate
func (g *GroupByQuery) Count(alias string) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.aggregates = append(g.aggregates, aggregateOp{
		function: "COUNT",
		field:    "*",
		alias:    alias,
	})
	return g
}

// Sum adds a SUM aggregate on a field
func (g *GroupByQuery) Sum(field, alias string) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.aggregates = append(g.aggregates, aggregateOp{
		function: "SUM",
		field:    field,
		alias:    alias,
	})
	return g
}

// Avg adds an AVG aggregate on a field
func (g *GroupByQuery) Avg(field, alias string) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.aggregates = append(g.aggregates, aggregateOp{
		function: "AVG",
		field:    field,
		alias:    alias,
	})
	return g
}

// Min adds a MIN aggregate on a field
func (g *GroupByQuery) Min(field, alias string) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.aggregates = append(g.aggregates, aggregateOp{
		function: "MIN",
		field:    field,
		alias:    alias,
	})
	return g
}

// Max adds a MAX aggregate on a field
func (g *GroupByQuery) Max(field, alias string) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.aggregates = append(g.aggregates, aggregateOp{
		function: "MAX",
		field:    field,
		alias:    alias,
	})
	return g
}

// Having adds a HAVING clause to filter groups
func (g *GroupByQuery) Having(aggregate, operator string, value any) *GroupByQuery {
	if g.err != nil {
		return g
	}
	g.havingClauses = append(g.havingClauses, havingClause{
		aggregate: aggregate,
		operator:  operator,
		value:     value,
	})
	return g
}

// Execute runs the group by query and returns results
func (g *GroupByQuery) Execute() ([]*GroupedResult, error) {
	if g.err != nil {
		return nil, g.err
	}

	// Group items
	for _, item := range g.items {
		key := extractFieldValue(item, g.groupBy)
		if key == nil {
			continue
		}

		keyStr := fmt.Sprintf("%v", key)
		if group, exists := g.groups[keyStr]; exists {
			group.Count++
			group.Items = append(group.Items, item)
		} else {
			g.groups[keyStr] = &GroupedResult{
				Key:        key,
				Count:      1,
				Items:      []any{item},
				Aggregates: make(map[string]*AggregateResult),
			}
		}
	}

	// Calculate aggregates for each group
	for _, group := range g.groups {
		for _, agg := range g.aggregates {
			result := g.calculateAggregate(group.Items, agg)
			group.Aggregates[agg.alias] = result
		}
	}

	// Apply HAVING clauses
	filteredGroups := make([]*GroupedResult, 0)
	for _, group := range g.groups {
		if g.evaluateHaving(group) {
			filteredGroups = append(filteredGroups, group)
		}
	}

	return filteredGroups, nil
}

// calculateAggregate calculates a single aggregate for a group
func (g *GroupByQuery) calculateAggregate(items []any, agg aggregateOp) *AggregateResult {
	result := &AggregateResult{}

	switch agg.function {
	case "COUNT":
		result.Count = int64(len(items))

	case "SUM":
		var sum float64
		for _, item := range items {
			value, err := extractNumericValue(item, agg.field)
			if err == nil {
				sum += value
			}
		}
		result.Sum = sum

	case "AVG":
		var sum float64
		var count int
		for _, item := range items {
			value, err := extractNumericValue(item, agg.field)
			if err == nil {
				sum += value
				count++
			}
		}
		if count > 0 {
			result.Average = sum / float64(count)
		}

	case "MIN":
		var minValue any
		first := true
		for _, item := range items {
			value := extractFieldValue(item, agg.field)
			if value == nil {
				continue
			}
			if first {
				minValue = value
				first = false
			} else if compareValues(value, minValue) < 0 {
				minValue = value
			}
		}
		result.Min = minValue

	case "MAX":
		var maxValue any
		first := true
		for _, item := range items {
			value := extractFieldValue(item, agg.field)
			if value == nil {
				continue
			}
			if first {
				maxValue = value
				first = false
			} else if compareValues(value, maxValue) > 0 {
				maxValue = value
			}
		}
		result.Max = maxValue
	}

	return result
}

// evaluateHaving evaluates HAVING clauses for a group
func (g *GroupByQuery) evaluateHaving(group *GroupedResult) bool {
	for _, having := range g.havingClauses {
		// Parse aggregate function (e.g., "COUNT(*)", "SUM(price)")
		var aggValue float64
		var found bool

		// Simple parsing - in production, use a proper parser
		if having.aggregate == "COUNT(*)" {
			aggValue = float64(group.Count)
			found = true
		} else {
			// Look for alias in aggregates
			for alias, result := range group.Aggregates {
				if alias == having.aggregate {
					switch {
					case result.Count > 0:
						aggValue = float64(result.Count)
					case result.Sum != 0:
						aggValue = result.Sum
					case result.Average != 0:
						aggValue = result.Average
					default:
						// For MIN/MAX, try to convert to float
						if result.Min != nil {
							aggValue, _ = toFloat64(result.Min)
						} else if result.Max != nil {
							aggValue, _ = toFloat64(result.Max)
						}
					}
					found = true
					break
				}
			}
		}

		if !found {
			return false
		}

		// Evaluate condition
		compareValue, err := toFloat64(having.value)
		if err != nil {
			return false
		}

		switch having.operator {
		case "=":
			if aggValue != compareValue {
				return false
			}
		case ">":
			if aggValue <= compareValue {
				return false
			}
		case ">=":
			if aggValue < compareValue {
				return false
			}
		case "<":
			if aggValue >= compareValue {
				return false
			}
		case "<=":
			if aggValue > compareValue {
				return false
			}
		case "!=":
			if aggValue == compareValue {
				return false
			}
		}
	}

	return true
}

// getAllItems is a helper to retrieve all items for aggregate operations
func (q *Query) getAllItems() ([]any, error) {
	// Create a slice type based on model
	modelType := reflect.TypeOf(q.model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	sliceType := reflect.SliceOf(modelType)
	resultsPtr := reflect.New(sliceType)

	// Execute query
	err := q.All(resultsPtr.Interface())
	if err != nil {
		return nil, err
	}

	// Convert to []any
	results := resultsPtr.Elem()
	items := make([]any, results.Len())
	for i := 0; i < results.Len(); i++ {
		items[i] = results.Index(i).Interface()
	}

	return items, nil
}

// extractNumericValue extracts a numeric value from an item
func extractNumericValue(item any, field string) (float64, error) {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fieldValue := v.FieldByName(field)
	if !fieldValue.IsValid() {
		return 0, fmt.Errorf("field %s not found", field)
	}

	switch fieldValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fieldValue.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fieldValue.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return fieldValue.Float(), nil
	default:
		return 0, fmt.Errorf("field %s is not numeric", field)
	}
}

// extractFieldValue extracts any field value from an item
func extractFieldValue(item any, field string) any {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	fieldValue := v.FieldByName(field)
	if !fieldValue.IsValid() || fieldValue.IsZero() {
		return nil
	}

	return fieldValue.Interface()
}

// compareValues compares two values of the same type
func compareValues(a, b any) int {
	// Handle numeric types
	aFloat, aErr := toFloat64(a)
	bFloat, bErr := toFloat64(b)
	if aErr == nil && bErr == nil {
		if aFloat < bFloat {
			return -1
		} else if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// Handle strings
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		if aStr < bStr {
			return -1
		} else if aStr > bStr {
			return 1
		}
		return 0
	}

	// Default: convert to string and compare
	aStrVal := fmt.Sprintf("%v", a)
	bStrVal := fmt.Sprintf("%v", b)
	if aStrVal < bStrVal {
		return -1
	} else if aStrVal > bStrVal {
		return 1
	}
	return 0
}

// toFloat64 attempts to convert a value to float64
func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case float32:
		return float64(val), nil
	case float64:
		return val, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// CountDistinct counts unique values for a field
func (q *Query) CountDistinct(field string) (int64, error) {
	items, err := q.getAllItems()
	if err != nil {
		return 0, err
	}

	uniqueValues := make(map[string]bool)
	for _, item := range items {
		value := extractFieldValue(item, field)
		if value != nil {
			key := fmt.Sprintf("%v", value)
			uniqueValues[key] = true
		}
	}

	return int64(len(uniqueValues)), nil
}

// Having is deprecated - use GroupBy().Having() instead for proper aggregate filtering
// This method is kept for backward compatibility but does nothing
func (q *Query) Having(condition string, value any) core.Query {
	// Use GroupBy().Having() for actual functionality
	return q
}
