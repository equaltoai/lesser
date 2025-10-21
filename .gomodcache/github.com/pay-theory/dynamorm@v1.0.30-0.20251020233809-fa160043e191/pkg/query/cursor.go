package query

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Cursor represents pagination state for DynamoDB queries
type Cursor struct {
	LastEvaluatedKey map[string]any `json:"lastKey"`
	IndexName        string         `json:"index,omitempty"`
	SortDirection    string         `json:"sort,omitempty"`
}

// Note: PaginatedResult is defined in types.go

// EncodeCursor encodes a DynamoDB LastEvaluatedKey into a base64 cursor string
func EncodeCursor(lastKey map[string]types.AttributeValue, indexName string, sortDirection string) (string, error) {
	if len(lastKey) == 0 {
		return "", nil
	}

	// Convert AttributeValues to JSON-friendly format
	jsonKey := make(map[string]any)
	for k, v := range lastKey {
		jsonValue, err := attributeValueToJSON(v)
		if err != nil {
			return "", fmt.Errorf("failed to convert attribute %s: %w", k, err)
		}
		jsonKey[k] = jsonValue
	}

	cursor := Cursor{
		LastEvaluatedKey: jsonKey,
		IndexName:        indexName,
		SortDirection:    sortDirection,
	}

	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor: %w", err)
	}

	return base64.URLEncoding.EncodeToString(data), nil
}

// DecodeCursor decodes a base64 cursor string into a Cursor
func DecodeCursor(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	return &cursor, nil
}

// ToAttributeValues converts the cursor's LastEvaluatedKey back to DynamoDB AttributeValues
func (c *Cursor) ToAttributeValues() (map[string]types.AttributeValue, error) {
	if c == nil || len(c.LastEvaluatedKey) == 0 {
		return nil, nil
	}

	result := make(map[string]types.AttributeValue)
	for k, v := range c.LastEvaluatedKey {
		av, err := jsonToAttributeValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert attribute %s: %w", k, err)
		}
		result[k] = av
	}

	return result, nil
}

// attributeValueToJSON converts a DynamoDB AttributeValue to a JSON-friendly format
func attributeValueToJSON(av types.AttributeValue) (any, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return map[string]any{"S": v.Value}, nil
	case *types.AttributeValueMemberN:
		return map[string]any{"N": v.Value}, nil
	case *types.AttributeValueMemberB:
		return map[string]any{"B": base64.StdEncoding.EncodeToString(v.Value)}, nil
	case *types.AttributeValueMemberBOOL:
		return map[string]any{"BOOL": v.Value}, nil
	case *types.AttributeValueMemberNULL:
		return map[string]any{"NULL": true}, nil
	case *types.AttributeValueMemberL:
		list := make([]any, len(v.Value))
		for i, item := range v.Value {
			jsonItem, err := attributeValueToJSON(item)
			if err != nil {
				return nil, err
			}
			list[i] = jsonItem
		}
		return map[string]any{"L": list}, nil
	case *types.AttributeValueMemberM:
		m := make(map[string]any)
		for k, val := range v.Value {
			jsonVal, err := attributeValueToJSON(val)
			if err != nil {
				return nil, err
			}
			m[k] = jsonVal
		}
		return map[string]any{"M": m}, nil
	case *types.AttributeValueMemberSS:
		return map[string]any{"SS": v.Value}, nil
	case *types.AttributeValueMemberNS:
		return map[string]any{"NS": v.Value}, nil
	case *types.AttributeValueMemberBS:
		encoded := make([]string, len(v.Value))
		for i, b := range v.Value {
			encoded[i] = base64.StdEncoding.EncodeToString(b)
		}
		return map[string]any{"BS": encoded}, nil
	default:
		return nil, fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

// jsonToAttributeValue converts a JSON-friendly format back to DynamoDB AttributeValue
func jsonToAttributeValue(v any) (types.AttributeValue, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", v)
	}

	// String
	if val, ok := m["S"]; ok {
		strVal, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("s value must be string")
		}
		return &types.AttributeValueMemberS{Value: strVal}, nil
	}

	// Number
	if val, ok := m["N"]; ok {
		strVal, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("n value must be string")
		}
		return &types.AttributeValueMemberN{Value: strVal}, nil
	}

	// Binary
	if val, ok := m["B"]; ok {
		strVal, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("b value must be string")
		}
		decoded, err := base64.StdEncoding.DecodeString(strVal)
		if err != nil {
			return nil, fmt.Errorf("failed to decode binary: %w", err)
		}
		return &types.AttributeValueMemberB{Value: decoded}, nil
	}

	// Boolean
	if val, ok := m["BOOL"]; ok {
		boolVal, ok := val.(bool)
		if !ok {
			return nil, fmt.Errorf("bool value must be bool")
		}
		return &types.AttributeValueMemberBOOL{Value: boolVal}, nil
	}

	// Null
	if _, ok := m["NULL"]; ok {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// List
	if val, ok := m["L"]; ok {
		listVal, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("l value must be []any")
		}
		list := make([]types.AttributeValue, len(listVal))
		for i, item := range listVal {
			av, err := jsonToAttributeValue(item)
			if err != nil {
				return nil, err
			}
			list[i] = av
		}
		return &types.AttributeValueMemberL{Value: list}, nil
	}

	// Map
	if val, ok := m["M"]; ok {
		mapVal, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("m value must be map[string]any")
		}
		avMap := make(map[string]types.AttributeValue)
		for k, v := range mapVal {
			av, err := jsonToAttributeValue(v)
			if err != nil {
				return nil, err
			}
			avMap[k] = av
		}
		return &types.AttributeValueMemberM{Value: avMap}, nil
	}

	// String Set
	if val, ok := m["SS"]; ok {
		listVal, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("ss value must be []any")
		}
		strSet := make([]string, len(listVal))
		for i, item := range listVal {
			strVal, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("ss items must be strings")
			}
			strSet[i] = strVal
		}
		return &types.AttributeValueMemberSS{Value: strSet}, nil
	}

	// Number Set
	if val, ok := m["NS"]; ok {
		listVal, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("ns value must be []any")
		}
		numSet := make([]string, len(listVal))
		for i, item := range listVal {
			strVal, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("ns items must be strings")
			}
			numSet[i] = strVal
		}
		return &types.AttributeValueMemberNS{Value: numSet}, nil
	}

	// Binary Set
	if val, ok := m["BS"]; ok {
		listVal, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("bs value must be []any")
		}
		binSet := make([][]byte, len(listVal))
		for i, item := range listVal {
			strVal, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("bs items must be strings")
			}
			decoded, err := base64.StdEncoding.DecodeString(strVal)
			if err != nil {
				return nil, fmt.Errorf("failed to decode binary: %w", err)
			}
			binSet[i] = decoded
		}
		return &types.AttributeValueMemberBS{Value: binSet}, nil
	}

	return nil, fmt.Errorf("unknown attribute value format: %v", m)
}
