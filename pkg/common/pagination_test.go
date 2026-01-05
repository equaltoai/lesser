package common

import (
	"strings"
	"testing"
)

// TestValidatePaginationLimit tests the ValidatePaginationLimit function
func TestValidatePaginationLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "within valid range",
			input:    20,
			expected: 20,
		},
		{
			name:     "at minimum",
			input:    MinPaginationLimit,
			expected: MinPaginationLimit,
		},
		{
			name:     "at maximum",
			input:    MaxPaginationLimit,
			expected: MaxPaginationLimit,
		},
		{
			name:     "below minimum clamps to min",
			input:    0,
			expected: MinPaginationLimit,
		},
		{
			name:     "negative clamps to min",
			input:    -10,
			expected: MinPaginationLimit,
		},
		{
			name:     "above maximum clamps to max",
			input:    200,
			expected: MaxPaginationLimit,
		},
		{
			name:     "well above maximum clamps to max",
			input:    10000,
			expected: MaxPaginationLimit,
		},
		{
			name:     "mid-range value unchanged",
			input:    50,
			expected: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePaginationLimit(tt.input)
			if result != tt.expected {
				t.Errorf("ValidatePaginationLimit(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCalculateHasMore tests the CalculateHasMore function
func TestCalculateHasMore(t *testing.T) {
	tests := []struct {
		name           string
		itemCount      int
		requestedLimit int
		expected       bool
	}{
		{
			name:           "exactly at limit - has more",
			itemCount:      20,
			requestedLimit: 20,
			expected:       true,
		},
		{
			name:           "above limit - has more",
			itemCount:      25,
			requestedLimit: 20,
			expected:       true,
		},
		{
			name:           "below limit - no more",
			itemCount:      15,
			requestedLimit: 20,
			expected:       false,
		},
		{
			name:           "zero items - no more",
			itemCount:      0,
			requestedLimit: 20,
			expected:       false,
		},
		{
			name:           "one item with limit 1",
			itemCount:      1,
			requestedLimit: 1,
			expected:       true,
		},
		{
			name:           "empty result with limit 0",
			itemCount:      0,
			requestedLimit: 0,
			expected:       true, // 0 >= 0 is true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateHasMore(tt.itemCount, tt.requestedLimit)
			if result != tt.expected {
				t.Errorf("CalculateHasMore(%d, %d) = %v, want %v", tt.itemCount, tt.requestedLimit, result, tt.expected)
			}
		})
	}
}

// TestCalculateOffset tests the CalculateOffset function
func TestCalculateOffset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		limit    int
		expected int
	}{
		{
			name:     "page 1 with limit 20",
			page:     1,
			limit:    20,
			expected: 0,
		},
		{
			name:     "page 2 with limit 20",
			page:     2,
			limit:    20,
			expected: 20,
		},
		{
			name:     "page 3 with limit 20",
			page:     3,
			limit:    20,
			expected: 40,
		},
		{
			name:     "page 10 with limit 50",
			page:     10,
			limit:    50,
			expected: 450,
		},
		{
			name:     "page 0 treated as page 1",
			page:     0,
			limit:    20,
			expected: 0,
		},
		{
			name:     "negative page treated as page 1",
			page:     -5,
			limit:    20,
			expected: 0,
		},
		{
			name:     "large page number",
			page:     100,
			limit:    10,
			expected: 990,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateOffset(tt.page, tt.limit)
			if result != tt.expected {
				t.Errorf("CalculateOffset(%d, %d) = %d, want %d", tt.page, tt.limit, result, tt.expected)
			}
		})
	}
}

// TestCalculatePage tests the CalculatePage function
func TestCalculatePage(t *testing.T) {
	tests := []struct {
		name     string
		offset   int
		limit    int
		expected int
	}{
		{
			name:     "offset 0 is page 1",
			offset:   0,
			limit:    20,
			expected: 1,
		},
		{
			name:     "offset 20 with limit 20 is page 2",
			offset:   20,
			limit:    20,
			expected: 2,
		},
		{
			name:     "offset 40 with limit 20 is page 3",
			offset:   40,
			limit:    20,
			expected: 3,
		},
		{
			name:     "partial offset rounds down",
			offset:   25,
			limit:    20,
			expected: 2,
		},
		{
			name:     "zero limit uses default",
			offset:   40,
			limit:    0,
			expected: 3, // 40 / 20 + 1 = 3
		},
		{
			name:     "negative limit uses default",
			offset:   40,
			limit:    -10,
			expected: 3,
		},
		{
			name:     "large offset",
			offset:   990,
			limit:    10,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePage(tt.offset, tt.limit)
			if result != tt.expected {
				t.Errorf("CalculatePage(%d, %d) = %d, want %d", tt.offset, tt.limit, result, tt.expected)
			}
		})
	}
}

// TestBuildLinkHeader tests the BuildLinkHeader function
func TestBuildLinkHeader(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		params     PaginationParams
		hasNext    bool
		hasPrev    bool
		nextCursor string
		prevCursor string
		wantEmpty  bool
		wantNext   bool
		wantPrev   bool
	}{
		{
			name:       "both next and prev",
			baseURL:    "https://example.com/api/statuses",
			params:     PaginationParams{Limit: 20},
			hasNext:    true,
			hasPrev:    true,
			nextCursor: "abc123",
			prevCursor: "xyz789",
			wantNext:   true,
			wantPrev:   true,
		},
		{
			name:       "only next",
			baseURL:    "https://example.com/api/statuses",
			params:     PaginationParams{Limit: 20},
			hasNext:    true,
			hasPrev:    false,
			nextCursor: "abc123",
			wantNext:   true,
			wantPrev:   false,
		},
		{
			name:       "only prev",
			baseURL:    "https://example.com/api/statuses",
			params:     PaginationParams{Limit: 20},
			hasNext:    false,
			hasPrev:    true,
			prevCursor: "xyz789",
			wantNext:   false,
			wantPrev:   true,
		},
		{
			name:      "neither next nor prev",
			baseURL:   "https://example.com/api/statuses",
			params:    PaginationParams{Limit: 20},
			hasNext:   false,
			hasPrev:   false,
			wantEmpty: true,
		},
		{
			name:       "non-default limit included",
			baseURL:    "https://example.com/api/statuses",
			params:     PaginationParams{Limit: 50},
			hasNext:    true,
			hasPrev:    false,
			nextCursor: "abc123",
			wantNext:   true,
		},
		{
			name:       "default limit not included",
			baseURL:    "https://example.com/api/statuses",
			params:     PaginationParams{Limit: DefaultPaginationLimit},
			hasNext:    true,
			hasPrev:    false,
			nextCursor: "abc123",
			wantNext:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildLinkHeader(tt.baseURL, tt.params, tt.hasNext, tt.hasPrev, tt.nextCursor, tt.prevCursor)

			if tt.wantEmpty && result != "" {
				t.Errorf("BuildLinkHeader() = %v, want empty", result)
				return
			}

			if !tt.wantEmpty && result == "" {
				t.Errorf("BuildLinkHeader() returned empty, want non-empty")
				return
			}

			if tt.wantNext {
				if len(result) == 0 {
					t.Error("expected next link in header, but header is empty")
				}
				// Just check that next link is present
				if !containsNextRel(result) && tt.wantNext {
					t.Errorf("BuildLinkHeader() should contain next rel")
				}
			}

			if tt.wantPrev {
				if !containsPrevRel(result) && tt.wantPrev {
					t.Errorf("BuildLinkHeader() should contain prev rel")
				}
			}
		})
	}
}

// containsNextRel checks if the link header contains rel="next"
func containsNextRel(header string) bool {
	return strings.Contains(header, `rel="next"`)
}

// containsPrevRel checks if the link header contains rel="prev"
func containsPrevRel(header string) bool {
	return strings.Contains(header, `rel="prev"`)
}

// TestGetPaginationParamsFromRequest tests the GetPaginationParamsFromRequest function
func TestGetPaginationParamsFromRequest(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string][]string
		wantLimit   int
		wantOffset  int
		wantPage    int
		wantMaxID   string
		wantMinID   string
		wantSinceID string
		wantCursor  string
	}{
		{
			name:        "empty params uses defaults",
			queryParams: map[string][]string{},
			wantLimit:   DefaultPaginationLimit,
			wantOffset:  0,
			wantPage:    DefaultPaginationPage,
		},
		{
			name: "custom limit",
			queryParams: map[string][]string{
				"limit": {"50"},
			},
			wantLimit:  50,
			wantOffset: 0,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "limit clamped to max",
			queryParams: map[string][]string{
				"limit": {"200"},
			},
			wantLimit:  MaxPaginationLimit,
			wantOffset: 0,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "limit clamped to min",
			queryParams: map[string][]string{
				"limit": {"0"},
			},
			wantLimit:  MinPaginationLimit,
			wantOffset: 0,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "offset parameter",
			queryParams: map[string][]string{
				"offset": {"40"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 40,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "page calculates offset",
			queryParams: map[string][]string{
				"page": {"3"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 40, // (3-1) * 20
			wantPage:   3,
		},
		{
			name: "explicit offset takes precedence over page",
			queryParams: map[string][]string{
				"page":   {"5"},
				"offset": {"100"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 100,
			wantPage:   5,
		},
		{
			name: "cursor parameters",
			queryParams: map[string][]string{
				"max_id":   {"abc123"},
				"min_id":   {"xyz789"},
				"since_id": {"def456"},
				"cursor":   {"next_cursor"},
			},
			wantLimit:   DefaultPaginationLimit,
			wantOffset:  0,
			wantPage:    DefaultPaginationPage,
			wantMaxID:   "abc123",
			wantMinID:   "xyz789",
			wantSinceID: "def456",
			wantCursor:  "next_cursor",
		},
		{
			name: "invalid limit ignored",
			queryParams: map[string][]string{
				"limit": {"not-a-number"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 0,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "negative offset ignored",
			queryParams: map[string][]string{
				"offset": {"-10"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 0, // negative ignored, stays at 0
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "page 0 keeps default",
			queryParams: map[string][]string{
				"page": {"0"},
			},
			wantLimit:  DefaultPaginationLimit,
			wantOffset: 0,
			wantPage:   DefaultPaginationPage,
		},
		{
			name: "combined limit and page",
			queryParams: map[string][]string{
				"limit": {"50"},
				"page":  {"2"},
			},
			wantLimit:  50,
			wantOffset: 50, // (2-1) * 50
			wantPage:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPaginationParamsFromRequest(tt.queryParams)

			if result.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", result.Limit, tt.wantLimit)
			}
			if result.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", result.Offset, tt.wantOffset)
			}
			if result.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", result.Page, tt.wantPage)
			}
			if result.MaxID != tt.wantMaxID {
				t.Errorf("MaxID = %s, want %s", result.MaxID, tt.wantMaxID)
			}
			if result.MinID != tt.wantMinID {
				t.Errorf("MinID = %s, want %s", result.MinID, tt.wantMinID)
			}
			if result.SinceID != tt.wantSinceID {
				t.Errorf("SinceID = %s, want %s", result.SinceID, tt.wantSinceID)
			}
			if result.Cursor != tt.wantCursor {
				t.Errorf("Cursor = %s, want %s", result.Cursor, tt.wantCursor)
			}
		})
	}
}

// TestPaginationConstants tests that pagination constants have expected values
func TestPaginationConstants(t *testing.T) {
	// Verify constants have sensible values
	if DefaultPaginationLimit <= 0 {
		t.Errorf("DefaultPaginationLimit should be positive, got %d", DefaultPaginationLimit)
	}

	if MaxPaginationLimit <= DefaultPaginationLimit {
		t.Errorf("MaxPaginationLimit (%d) should be greater than DefaultPaginationLimit (%d)",
			MaxPaginationLimit, DefaultPaginationLimit)
	}

	if MinPaginationLimit <= 0 {
		t.Errorf("MinPaginationLimit should be positive, got %d", MinPaginationLimit)
	}

	if MinPaginationLimit > DefaultPaginationLimit {
		t.Errorf("MinPaginationLimit (%d) should be <= DefaultPaginationLimit (%d)",
			MinPaginationLimit, DefaultPaginationLimit)
	}

	if DefaultPaginationPage != 1 {
		t.Errorf("DefaultPaginationPage should be 1, got %d", DefaultPaginationPage)
	}
}
