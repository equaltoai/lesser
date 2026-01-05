package common

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantErr  bool
		contains string
	}{
		{
			name:     "simple struct",
			input:    struct{ Name string }{"test"},
			wantErr:  false,
			contains: "test",
		},
		{
			name:     "map",
			input:    map[string]string{"key": "value"},
			wantErr:  false,
			contains: "value",
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			wantErr:  false,
			contains: "[1,2,3]",
		},
		{
			name:    "channel causes error",
			input:   make(chan int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Marshal(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, string(result), tt.contains)
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	t.Run("valid JSON to struct", func(t *testing.T) {
		type TestStruct struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		var result TestStruct
		err := Unmarshal([]byte(`{"name":"test","age":25}`), &result)
		require.NoError(t, err)
		assert.Equal(t, "test", result.Name)
		assert.Equal(t, 25, result.Age)
	})

	t.Run("valid JSON to map", func(t *testing.T) {
		var result map[string]any
		err := Unmarshal([]byte(`{"key":"value","num":42}`), &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		var result map[string]any
		err := Unmarshal([]byte(`{invalid}`), &result)
		assert.Error(t, err)
	})
}

func TestMarshalString(t *testing.T) {
	t.Run("struct to string", func(t *testing.T) {
		input := struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}{"123", "test"}
		result, err := MarshalString(input)
		require.NoError(t, err)
		assert.Contains(t, result, `"id":"123"`)
		assert.Contains(t, result, `"name":"test"`)
	})

	t.Run("error case", func(t *testing.T) {
		result, err := MarshalString(make(chan int))
		assert.Error(t, err)
		assert.Empty(t, result)
	})
}

func TestNewEncoder(t *testing.T) {
	t.Run("creates encoder with EscapeHTML disabled", func(t *testing.T) {
		buf := &bytes.Buffer{}
		encoder := NewEncoder(buf)
		require.NotNil(t, encoder)

		// Encode something with HTML characters to verify escaping is disabled
		err := encoder.Encode(map[string]string{"url": "https://example.com?a=1&b=2"})
		require.NoError(t, err)

		// HTML escaping disabled means & should NOT become \u0026
		result := buf.String()
		assert.Contains(t, result, "&")
		assert.NotContains(t, result, "\\u0026")
	})
}

func TestNewDecoder(t *testing.T) {
	t.Run("creates working decoder", func(t *testing.T) {
		input := `{"name":"test","value":123}`
		reader := strings.NewReader(input)
		decoder := NewDecoder(reader)
		require.NotNil(t, decoder)

		var result map[string]any
		err := decoder.Decode(&result)
		require.NoError(t, err)
		assert.Equal(t, "test", result["name"])
	})

	t.Run("decoder handles multiple objects", func(t *testing.T) {
		input := `{"a":1}
{"b":2}`
		reader := strings.NewReader(input)
		decoder := NewDecoder(reader)

		var result1 map[string]any
		err := decoder.Decode(&result1)
		require.NoError(t, err)
		assert.Equal(t, float64(1), result1["a"])

		var result2 map[string]any
		err = decoder.Decode(&result2)
		require.NoError(t, err)
		assert.Equal(t, float64(2), result2["b"])
	})
}
