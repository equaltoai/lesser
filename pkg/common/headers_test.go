package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCORSHeaders(t *testing.T) {
	headers := GetCORSHeaders()

	require.NotNil(t, headers)
	assert.Equal(t, "*", headers["Access-Control-Allow-Origin"])
	assert.Contains(t, headers["Access-Control-Allow-Headers"], "Authorization")
	assert.Contains(t, headers["Access-Control-Allow-Headers"], "Content-Type")
	assert.Contains(t, headers["Access-Control-Allow-Methods"], "GET")
	assert.Contains(t, headers["Access-Control-Allow-Methods"], "POST")
	assert.Contains(t, headers["Access-Control-Allow-Methods"], "PUT")
	assert.Contains(t, headers["Access-Control-Allow-Methods"], "DELETE")
	assert.Contains(t, headers["Access-Control-Allow-Methods"], "OPTIONS")
}

func TestGetAPIHeaders(t *testing.T) {
	headers := GetAPIHeaders()

	require.NotNil(t, headers)
	// Should include CORS headers
	assert.Equal(t, "*", headers["Access-Control-Allow-Origin"])
	// Should include Content-Type
	assert.Equal(t, "application/json", headers["Content-Type"])
}

func TestAddLinkHeader(t *testing.T) {
	tests := []struct {
		name           string
		baseURL        string
		endpoint       string
		cursor         string
		params         map[string]string
		expectedKey    string
		expectedHasKey bool
		expectedValue  string
	}{
		{
			name:           "empty cursor does nothing",
			baseURL:        "https://example.com",
			endpoint:       "/api/v1/statuses",
			cursor:         "",
			params:         nil,
			expectedKey:    "Link",
			expectedHasKey: false,
		},
		{
			name:           "with cursor builds correct URL",
			baseURL:        "https://example.com",
			endpoint:       "/api/v1/statuses",
			cursor:         "12345",
			params:         nil,
			expectedKey:    "Link",
			expectedHasKey: true,
			expectedValue:  `<https://example.com/api/v1/statuses?max_id=12345>; rel="next"`,
		},
		{
			name:           "with cursor and params",
			baseURL:        "https://example.com",
			endpoint:       "/api/v1/timelines/home",
			cursor:         "67890",
			params:         map[string]string{"limit": "20"},
			expectedKey:    "Link",
			expectedHasKey: true,
		},
		{
			name:     "empty param values skipped",
			baseURL:  "https://example.com",
			endpoint: "/api/v1/statuses",
			cursor:   "12345",
			params: map[string]string{
				"limit":  "20",
				"empty":  "",
				"zero":   "0",
				"falsey": StringFalse,
			},
			expectedKey:    "Link",
			expectedHasKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(map[string]string)
			AddLinkHeader(headers, tt.baseURL, tt.endpoint, tt.cursor, tt.params)

			if tt.expectedHasKey {
				assert.Contains(t, headers, tt.expectedKey)
				if tt.expectedValue != "" {
					assert.Equal(t, tt.expectedValue, headers[tt.expectedKey])
				}
				// Verify it contains the cursor
				assert.Contains(t, headers[tt.expectedKey], tt.cursor)
				assert.Contains(t, headers[tt.expectedKey], `rel="next"`)
			} else {
				assert.NotContains(t, headers, tt.expectedKey)
			}
		})
	}
}

func TestAddPaginationHeaders(t *testing.T) {
	t.Run("adds Link header", func(t *testing.T) {
		headers := make(map[string]string)
		AddPaginationHeaders(headers, "https://example.com", "/api/v1/statuses", "12345", nil, 0)

		assert.Contains(t, headers, "Link")
		assert.Contains(t, headers["Link"], "12345")
	})

	t.Run("adds X-Total-Count when totalCount > 0", func(t *testing.T) {
		headers := make(map[string]string)
		AddPaginationHeaders(headers, "https://example.com", "/api/v1/statuses", "12345", nil, 100)

		assert.Equal(t, "100", headers["X-Total-Count"])
	})

	t.Run("no X-Total-Count when totalCount is 0", func(t *testing.T) {
		headers := make(map[string]string)
		AddPaginationHeaders(headers, "https://example.com", "/api/v1/statuses", "12345", nil, 0)

		assert.NotContains(t, headers, "X-Total-Count")
	})

	t.Run("with params and totalCount", func(t *testing.T) {
		headers := make(map[string]string)
		params := map[string]string{"limit": "20", "only_media": "true"}
		AddPaginationHeaders(headers, "https://example.com", "/api/v1/timelines/home", "99999", params, 500)

		assert.Contains(t, headers, "Link")
		assert.Equal(t, "500", headers["X-Total-Count"])
	})
}
