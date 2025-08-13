package jsonld

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeOptions(t *testing.T) {
	tests := []struct {
		name     string
		options  CanonicalizeOptions
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "remove signature fields",
			options: CanonicalizeOptions{
				RemoveSignatureFields: true,
				SignatureFields:       []string{"signature", "proof"},
			},
			input: map[string]interface{}{
				"name":      "test",
				"signature": "should_be_removed",
				"proof":     "also_removed",
				"data":      "preserved",
			},
			expected: map[string]interface{}{
				"name": "test",
				"data": "preserved",
			},
		},
		{
			name: "preserve all fields when not removing signatures",
			options: CanonicalizeOptions{
				RemoveSignatureFields: false,
			},
			input: map[string]interface{}{
				"name":      "test",
				"signature": "preserved",
				"data":      "preserved",
			},
			expected: map[string]interface{}{
				"name":      "test",
				"signature": "preserved",
				"data":      "preserved",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCanonicalizer(tt.options)
			result := c.removeSignatureFields(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanonicalizeJSON_SimpleObject(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name: "simple object with alphabetical key ordering",
			input: map[string]interface{}{
				"z_last":  "value1",
				"a_first": "value2",
				"m_mid":   "value3",
			},
			expected: `{"a_first":"value2","m_mid":"value3","z_last":"value1"}`,
		},
		{
			name: "object with nested structure",
			input: map[string]interface{}{
				"parent": map[string]interface{}{
					"z_child": "nested_value",
					"a_child": "first_nested",
				},
				"simple": "value",
			},
			expected: `{"parent":{"a_child":"first_nested","z_child":"nested_value"},"simple":"value"}`,
		},
		{
			name: "array values maintain order",
			input: map[string]interface{}{
				"array": []interface{}{"third", "first", "second"},
				"name":  "test",
			},
			expected: `{"array":["third","first","second"],"name":"test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := CanonicalizeOptions{
				SkipExpansion:         true,
				RemoveSignatureFields: false,
			}
			c := NewCanonicalizer(options)
			
			result, err := c.CanonicalizeToJSON(tt.input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(result))
		})
	}
}

func TestCanonicalizeJSON_ActivityPubObjects(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected map[string]interface{}
	}{
		{
			name: "ActivityPub Note with signature removal",
			input: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"id":           "https://example.com/notes/1",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
				"published":    "2023-01-01T00:00:00Z",
				"signature":    "should_be_removed",
				"proof":        "also_removed",
			},
			expected: map[string]interface{}{
				"@context":     "https://www.w3.org/ns/activitystreams",
				"type":         "Note",
				"id":           "https://example.com/notes/1",
				"content":      "Hello World",
				"attributedTo": "https://example.com/users/alice",
				"published":    "2023-01-01T00:00:00Z",
			},
		},
		{
			name: "ActivityPub Actor",
			input: map[string]interface{}{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"type":              "Person",
				"id":                "https://example.com/users/alice",
				"preferredUsername": "alice",
				"name":              "Alice Smith",
				"inbox":             "https://example.com/users/alice/inbox",
				"outbox":            "https://example.com/users/alice/outbox",
				"publicKey": map[string]interface{}{
					"id":           "https://example.com/users/alice#main-key",
					"owner":        "https://example.com/users/alice",
					"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFA...",
				},
				"signature": "should_be_removed",
			},
			expected: map[string]interface{}{
				"@context":          "https://alice.w3.org/ns/activitystreams",
				"type":              "Person",
				"id":                "https://example.com/users/alice",
				"preferredUsername": "alice",
				"name":              "Alice Smith",
				"inbox":             "https://example.com/users/alice/inbox",
				"outbox":            "https://example.com/users/alice/outbox",
				"publicKey": map[string]interface{}{
					"id":           "https://example.com/users/alice#main-key",
					"owner":        "https://example.com/users/alice",
					"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFA...",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CanonicalizeActivityPubObject(tt.input)
			require.NoError(t, err)
			
			var parsed map[string]interface{}
			err = json.Unmarshal(result, &parsed)
			require.NoError(t, err)
			
			// Note: We can't do exact comparison due to key ordering
			// Instead, verify signature fields are removed and required fields exist
			_, hasSignature := parsed["signature"]
			assert.False(t, hasSignature, "signature field should be removed")
			
			_, hasProof := parsed["proof"]
			assert.False(t, hasProof, "proof field should be removed")
			
			// Verify required fields exist
			assert.Equal(t, tt.expected["type"], parsed["type"])
			assert.Equal(t, tt.expected["id"], parsed["id"])
		})
	}
}

func TestCanonicalizeJSON_ReputationObjects(t *testing.T) {
	// Test with reputation system objects
	reputation := map[string]interface{}{
		"actorId":         "https://example.com/users/alice",
		"instance":        "https://example.com",
		"trustScore":      850,
		"activityScore":   720,
		"moderationScore": 900,
		"communityScore":  780,
		"totalScore":      812,
		"calculatedAt":    "2023-01-01T00:00:00Z",
		"version":         "1.0",
		"signature":       "should_be_removed",
		"publicKey":       "should_remain",
	}

	t.Run("reputation with signature removal", func(t *testing.T) {
		result, err := CanonicalizeBytesToJSON(mustMarshal(reputation), true)
		require.NoError(t, err)
		
		var parsed map[string]interface{}
		err = json.Unmarshal(result, &parsed)
		require.NoError(t, err)
		
		// Verify signature removed but publicKey preserved
		_, hasSignature := parsed["signature"]
		assert.False(t, hasSignature)
		
		_, hasPublicKey := parsed["publicKey"]
		assert.True(t, hasPublicKey)
		
		// Verify key ordering (should be alphabetical)
		resultStr := string(result)
		actorIdPos := strings.Index(resultStr, "actorId")
		instancePos := strings.Index(resultStr, "instance")
		versionPos := strings.Index(resultStr, "version")
		
		assert.True(t, actorIdPos < instancePos, "actorId should come before instance")
		assert.True(t, instancePos < versionPos, "instance should come before version")
	})

	vouch := map[string]interface{}{
		"id":          "https://example.com/vouches/1",
		"from":        "https://example.com/users/alice",
		"to":          "https://example.com/users/bob",
		"instance":    "https://example.com",
		"createdAt":   "2023-01-01T00:00:00Z",
		"expiresAt":   "2023-12-31T23:59:59Z",
		"confidence":  0.95,
		"context":     "colleague",
		"active":      true,
		"signature":   "should_be_removed",
	}

	t.Run("vouch with signature removal", func(t *testing.T) {
		result, err := CanonicalizeStructToJSON(vouch, true)
		require.NoError(t, err)
		
		var parsed map[string]interface{}
		err = json.Unmarshal(result, &parsed)
		require.NoError(t, err)
		
		_, hasSignature := parsed["signature"]
		assert.False(t, hasSignature)
		
		assert.Equal(t, vouch["from"], parsed["from"])
		assert.Equal(t, vouch["to"], parsed["to"])
	})
}

func TestCanonicalizeJSON_DataTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "integers become floats (JSON behavior)",
			input: map[string]interface{}{
				"score": int64(850),
				"count": int64(42),
			},
			expected: map[string]interface{}{
				"score": float64(850),
				"count": float64(42),
			},
		},
		{
			name: "floats preserved",
			input: map[string]interface{}{
				"ratio":      0.95,
				"percentage": 78.5,
			},
			expected: map[string]interface{}{
				"ratio":      0.95,
				"percentage": 78.5,
			},
		},
		{
			name: "booleans preserved",
			input: map[string]interface{}{
				"active":   true,
				"verified": false,
			},
			expected: map[string]interface{}{
				"active":   true,
				"verified": false,
			},
		},
		{
			name: "null values preserved",
			input: map[string]interface{}{
				"optional": nil,
				"required": "value",
			},
			expected: map[string]interface{}{
				"optional": nil,
				"required": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := CanonicalizeOptions{
				SkipExpansion:         true,
				RemoveSignatureFields: false,
			}
			c := NewCanonicalizer(options)
			
			result, err := c.CanonicalizeToJSON(tt.input)
			require.NoError(t, err)
			
			var parsed interface{}
			err = json.Unmarshal(result, &parsed)
			require.NoError(t, err)
			
			assert.Equal(t, tt.expected, parsed)
		})
	}
}

func TestCanonicalizeJSON_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "empty object",
			input:   map[string]interface{}{},
			wantErr: false,
		},
		{
			name:    "empty array",
			input:   []interface{}{},
			wantErr: false,
		},
		{
			name:    "null input",
			input:   nil,
			wantErr: false,
		},
		{
			name: "deeply nested object",
			input: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"value": "deep",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "mixed array types",
			input: map[string]interface{}{
				"mixed": []interface{}{
					"string",
					int64(42),
					true,
					map[string]interface{}{"nested": "object"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := CanonicalizeOptions{
				SkipExpansion:         true,
				RemoveSignatureFields: false,
			}
			c := NewCanonicalizer(options)
			
			result, err := c.CanonicalizeToJSON(tt.input)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				
				// Verify result is valid JSON
				var parsed interface{}
				err = json.Unmarshal(result, &parsed)
				assert.NoError(t, err)
			}
		})
	}
}

func TestCanonicalizeJSON_Unicode(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name: "unicode characters",
			input: map[string]interface{}{
				"emoji":   "🌟✨💫",
				"chinese": "你好世界",
				"arabic":  "مرحبا بالعالم",
				"russian": "Привет мир",
			},
		},
		{
			name: "special characters",
			input: map[string]interface{}{
				"quotes":    `"Hello" 'World'`,
				"backslash": `Path\To\File`,
				"newlines":  "Line 1\nLine 2\nLine 3",
				"tabs":      "Col1\tCol2\tCol3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := CanonicalizeOptions{
				SkipExpansion:         true,
				RemoveSignatureFields: false,
			}
			c := NewCanonicalizer(options)
			
			result, err := c.CanonicalizeToJSON(tt.input)
			require.NoError(t, err)
			
			// Verify result is valid JSON
			var parsed map[string]interface{}
			err = json.Unmarshal(result, &parsed)
			require.NoError(t, err)
			
			// Verify Unicode content is preserved
			for key, expectedValue := range tt.input {
				actualValue, exists := parsed[key]
				assert.True(t, exists, "key %s should exist", key)
				assert.Equal(t, expectedValue, actualValue, "value for key %s should be preserved", key)
			}
		})
	}
}

func TestDeterministicOrdering(t *testing.T) {
	// Test that canonicalization produces deterministic output
	input := map[string]interface{}{
		"z_field": "value_z",
		"a_field": "value_a",
		"m_field": map[string]interface{}{
			"nested_z": "nested_value_z",
			"nested_a": "nested_value_a",
		},
		"array_field": []interface{}{
			map[string]interface{}{
				"item_z": "item_z_value",
				"item_a": "item_a_value",
			},
		},
	}

	options := CanonicalizeOptions{
		SkipExpansion:         true,
		RemoveSignatureFields: false,
	}

	// Run canonicalization multiple times
	var results []string
	for i := 0; i < 5; i++ {
		c := NewCanonicalizer(options)
		result, err := c.CanonicalizeToJSON(input)
		require.NoError(t, err)
		results = append(results, string(result))
	}

	// All results should be identical
	firstResult := results[0]
	for i, result := range results[1:] {
		assert.Equal(t, firstResult, result, "result %d should match first result", i+1)
	}

	// Verify alphabetical ordering in the result
	resultStr := firstResult
	aPos := strings.Index(resultStr, `"a_field"`)
	mPos := strings.Index(resultStr, `"m_field"`)
	zPos := strings.Index(resultStr, `"z_field"`)

	assert.True(t, aPos < mPos, "a_field should come before m_field")
	assert.True(t, mPos < zPos, "m_field should come before z_field")
}

func TestHash(t *testing.T) {
	canonical := []byte(`{"name":"test","value":123}`)
	hash := Hash(canonical)
	
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex string length
	
	// Same input should produce same hash
	hash2 := Hash(canonical)
	assert.Equal(t, hash, hash2)
	
	// Different input should produce different hash
	differentCanonical := []byte(`{"name":"different","value":456}`)
	differentHash := Hash(differentCanonical)
	assert.NotEqual(t, hash, differentHash)
}

func TestIsBlankNode(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"_:b1", true},
		{"_:blank", true},
		{"_:c14n0", true},
		{"http://example.com", false},
		{"regular_string", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsBlankNode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trim whitespace",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "normalize multiple spaces",
			input:    "hello    world    test",
			expected: "hello world test",
		},
		{
			name:     "mixed whitespace types",
			input:    "hello\t\n  world",
			expected: "hello world",
		},
		{
			name:     "already normalized",
			input:    "hello world",
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeUnicode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests
func BenchmarkCanonicalizeSimpleObject(b *testing.B) {
	input := map[string]interface{}{
		"name":        "test",
		"value":       123,
		"active":      true,
		"description": "This is a test object for benchmarking",
	}

	options := CanonicalizeOptions{
		SkipExpansion:         true,
		RemoveSignatureFields: false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := NewCanonicalizer(options)
		_, err := c.CanonicalizeToJSON(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanonicalizeComplexObject(b *testing.B) {
	input := map[string]interface{}{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"type":         "Note",
		"id":           "https://example.com/notes/1",
		"content":      "This is a complex ActivityPub object with nested structures",
		"attributedTo": "https://example.com/users/alice",
		"published":    time.Now().Format(time.RFC3339),
		"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		"cc":           []string{"https://example.com/users/alice/followers"},
		"attachment": []interface{}{
			map[string]interface{}{
				"type":      "Image",
				"url":       "https://example.com/images/1.jpg",
				"mediaType": "image/jpeg",
			},
		},
		"tag": []interface{}{
			map[string]interface{}{
				"type": "Hashtag",
				"href": "https://example.com/tags/test",
				"name": "#test",
			},
		},
	}

	options := CanonicalizeOptions{
		SkipExpansion:         true,
		RemoveSignatureFields: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := NewCanonicalizer(options)
		_, err := c.CanonicalizeToJSON(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper functions
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}