package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormURLEncoded(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "simple key=value",
			body:     "key=value",
			expected: map[string]string{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "multiple keys",
			body:     "name=john&age=30",
			expected: map[string]string{"name": "john", "age": "30"},
			wantErr:  false,
		},
		{
			name:     "URL encoded values",
			body:     "message=hello%20world&url=https%3A%2F%2Fexample.com",
			expected: map[string]string{"message": "hello world", "url": "https://example.com"},
			wantErr:  false,
		},
		{
			name:     "empty string",
			body:     "",
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "special characters",
			body:     "text=%3Cscript%3Ealert%28%27xss%27%29%3C%2Fscript%3E",
			expected: map[string]string{"text": "<script>alert('xss')</script>"},
			wantErr:  false,
		},
		{
			name:     "plus signs as spaces",
			body:     "query=hello+world",
			expected: map[string]string{"query": "hello world"},
			wantErr:  false,
		},
		{
			name:     "equals in value",
			body:     "equation=a%3Db",
			expected: map[string]string{"equation": "a=b"},
			wantErr:  false,
		},
		{
			name:     "empty value",
			body:     "empty=",
			expected: map[string]string{"empty": ""},
			wantErr:  false,
		},
		{
			name:     "multiple values for same key takes first",
			body:     "key=first&key=second",
			expected: map[string]string{"key": "first"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFormURLEncoded(tt.body)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseMultipartForm(t *testing.T) {
	t.Run("missing boundary returns error", func(t *testing.T) {
		_, err := ParseMultipartForm("body", "multipart/form-data")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "boundary")
	})

	t.Run("invalid content type returns error", func(t *testing.T) {
		_, err := ParseMultipartForm("body", "invalid;;;;")
		assert.Error(t, err)
	})

	t.Run("valid multipart form", func(t *testing.T) {
		boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
		contentType := "multipart/form-data; boundary=" + boundary

		body := "------WebKitFormBoundary7MA4YWxkTrZu0gW\r\n" +
			"Content-Disposition: form-data; name=\"field1\"\r\n\r\n" +
			"value1\r\n" +
			"------WebKitFormBoundary7MA4YWxkTrZu0gW\r\n" +
			"Content-Disposition: form-data; name=\"field2\"\r\n\r\n" +
			"value2\r\n" +
			"------WebKitFormBoundary7MA4YWxkTrZu0gW--\r\n"

		result, err := ParseMultipartForm(body, contentType)
		require.NoError(t, err)
		assert.Equal(t, "value1", result["field1"])
		assert.Equal(t, "value2", result["field2"])
	})

	t.Run("empty form body", func(t *testing.T) {
		boundary := "----boundary"
		contentType := "multipart/form-data; boundary=" + boundary

		body := "------boundary--\r\n"

		result, err := ParseMultipartForm(body, contentType)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}
