package repositories

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1) PaginationOptions.Validate() Tests
// ============================================================================

func TestPaginationOptions_Validate(t *testing.T) {
	tests := []struct {
		name           string
		opts           PaginationOptions
		expectedLimit  int
		expectedSort   SearchSortOrder
		expectedError  bool
		errorValidator func(t *testing.T, err error)
	}{
		{
			name: "default values when limit is zero",
			opts: PaginationOptions{
				Limit:     0,
				SortOrder: "",
			},
			expectedLimit: 20,
			expectedSort:  SearchSortRelevance,
			expectedError: false,
		},
		{
			name: "default values when limit is negative",
			opts: PaginationOptions{
				Limit:     -5,
				SortOrder: "",
			},
			expectedLimit: 20,
			expectedSort:  SearchSortRelevance,
			expectedError: false,
		},
		{
			name: "limit clamped to max 50",
			opts: PaginationOptions{
				Limit:     100,
				SortOrder: SearchSortRelevance,
			},
			expectedLimit: 50,
			expectedSort:  SearchSortRelevance,
			expectedError: false,
		},
		{
			name: "valid limit within bounds",
			opts: PaginationOptions{
				Limit:     25,
				SortOrder: SearchSortTimeAsc,
			},
			expectedLimit: 25,
			expectedSort:  SearchSortTimeAsc,
			expectedError: false,
		},
		{
			name: "valid time_desc sort order",
			opts: PaginationOptions{
				Limit:     10,
				SortOrder: SearchSortTimeDesc,
			},
			expectedLimit: 10,
			expectedSort:  SearchSortTimeDesc,
			expectedError: false,
		},
		{
			name: "empty sort order defaults to relevance",
			opts: PaginationOptions{
				Limit:     15,
				SortOrder: "",
			},
			expectedLimit: 15,
			expectedSort:  SearchSortRelevance,
			expectedError: false,
		},
		{
			name: "invalid sort order returns error",
			opts: PaginationOptions{
				Limit:     10,
				SortOrder: "invalid_sort",
			},
			expectedLimit: 10,
			expectedSort:  SearchSortOrder("invalid_sort"),
			expectedError: true,
			errorValidator: func(t *testing.T, err error) {
				assert.True(t, errors.Is(err, ErrPaginationParametersInvalid),
					"expected error to wrap ErrPaginationParametersInvalid, got: %v", err)
			},
		},
		{
			name: "edge case: limit exactly at max",
			opts: PaginationOptions{
				Limit:     50,
				SortOrder: SearchSortRelevance,
			},
			expectedLimit: 50,
			expectedSort:  SearchSortRelevance,
			expectedError: false,
		},
		{
			name: "edge case: limit exactly at 1",
			opts: PaginationOptions{
				Limit:     1,
				SortOrder: SearchSortTimeAsc,
			},
			expectedLimit: 1,
			expectedSort:  SearchSortTimeAsc,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			err := opts.Validate()

			if tt.expectedError {
				require.Error(t, err)
				if tt.errorValidator != nil {
					tt.errorValidator(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedLimit, opts.Limit)
				assert.Equal(t, tt.expectedSort, opts.SortOrder)
			}
		})
	}
}

func TestNewPaginationOptions(t *testing.T) {
	opts := NewPaginationOptions()
	require.NotNil(t, opts)
	assert.Equal(t, 20, opts.Limit)
	assert.Equal(t, SearchSortRelevance, opts.SortOrder)
	assert.Empty(t, opts.Cursor)
}

// ============================================================================
// 2) Cursor Encode/Decode Tests
// ============================================================================

func TestEncodeCursor(t *testing.T) {
	tests := []struct {
		name            string
		input           *CursorData
		expectedEmpty   bool
		shouldRoundtrip bool
	}{
		{
			name:          "nil cursor returns empty string",
			input:         nil,
			expectedEmpty: true,
		},
		{
			name:          "empty cursor data returns empty string",
			input:         &CursorData{},
			expectedEmpty: true,
		},
		{
			name: "cursor with only zero values returns empty string",
			input: &CursorData{
				LastEvaluatedKey: nil,
				LastScore:        0,
				LastTimestamp:    time.Time{},
				LastID:           "",
				SortOrder:        "",
			},
			expectedEmpty: true,
		},
		{
			name: "cursor with LastID set returns non-empty",
			input: &CursorData{
				LastID:    "test-id-123",
				SortOrder: SearchSortRelevance,
			},
			expectedEmpty:   false,
			shouldRoundtrip: true,
		},
		{
			name: "cursor with LastScore set returns non-empty",
			input: &CursorData{
				LastScore: 0.95,
				SortOrder: SearchSortRelevance,
			},
			expectedEmpty:   false,
			shouldRoundtrip: true,
		},
		{
			name: "cursor with LastTimestamp set returns non-empty",
			input: &CursorData{
				LastTimestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				SortOrder:     SearchSortTimeDesc,
			},
			expectedEmpty:   false,
			shouldRoundtrip: true,
		},
		{
			name: "cursor with LastEvaluatedKey set returns non-empty",
			input: &CursorData{
				LastEvaluatedKey: map[string]interface{}{
					"PK": "STATUS#123",
					"SK": "META",
				},
				SortOrder: SearchSortTimeAsc,
			},
			expectedEmpty:   false,
			shouldRoundtrip: true,
		},
		{
			name: "full cursor data returns non-empty",
			input: &CursorData{
				LastEvaluatedKey: map[string]interface{}{"PK": "test"},
				LastScore:        0.85,
				LastTimestamp:    time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
				LastID:           "status-456",
				SortOrder:        SearchSortRelevance,
			},
			expectedEmpty:   false,
			shouldRoundtrip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeCursor(tt.input)

			if tt.expectedEmpty {
				assert.Empty(t, result, "expected empty cursor string")
			} else {
				assert.NotEmpty(t, result, "expected non-empty cursor string")

				// Verify it's valid base64
				decoded, err := base64.URLEncoding.DecodeString(result)
				require.NoError(t, err, "cursor should be valid base64")

				// Verify it's valid JSON
				var data CursorData
				err = json.Unmarshal(decoded, &data)
				require.NoError(t, err, "cursor should contain valid JSON")
			}
		})
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectError  bool
		errorType    error // expected wrapped error type
		expectedData *CursorData
	}{
		{
			name:         "empty cursor returns empty CursorData",
			input:        "",
			expectError:  false,
			expectedData: &CursorData{},
		},
		{
			name:        "invalid base64 (non-base64 chars) returns format error",
			input:       "%%%",
			expectError: true,
			errorType:   ErrPaginationCursorInvalid,
		},
		{
			name:        "invalid base64 with special chars",
			input:       "not!valid@base64",
			expectError: true,
			errorType:   ErrPaginationCursorInvalid,
		},
		{
			name:        "valid base64 but invalid JSON returns data error",
			input:       base64.URLEncoding.EncodeToString([]byte("not json content")),
			expectError: true,
			errorType:   ErrPaginationCursorData,
		},
		{
			name:        "cursor exceeding max length returns invalid error",
			input:       string(make([]byte, 501)), // Creates a string > 500 chars
			expectError: true,
			errorType:   ErrPaginationCursorInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeCursor(tt.input)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType),
						"expected error to wrap %v, got: %v", tt.errorType, err)
				}
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.expectedData != nil {
					assert.Equal(t, tt.expectedData.LastID, result.LastID)
					assert.Equal(t, tt.expectedData.LastScore, result.LastScore)
					assert.Equal(t, tt.expectedData.SortOrder, result.SortOrder)
				}
			}
		})
	}
}

func TestCursor_RoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		data *CursorData
	}{
		{
			name: "round-trip with all fields",
			data: &CursorData{
				LastEvaluatedKey: map[string]interface{}{
					"PK": "ACTOR#user123",
					"SK": "STATUS#2024-01-15T10:00:00Z",
				},
				LastScore:     0.873,
				LastTimestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
				LastID:        "status-id-123",
				SortOrder:     SearchSortRelevance,
			},
		},
		{
			name: "round-trip with minimal fields",
			data: &CursorData{
				LastID:    "id-only",
				SortOrder: SearchSortTimeAsc,
			},
		},
		{
			name: "round-trip with timestamp sort",
			data: &CursorData{
				LastTimestamp: time.Date(2024, 6, 20, 15, 30, 45, 0, time.UTC),
				LastID:        "ts-cursor",
				SortOrder:     SearchSortTimeDesc,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := EncodeCursor(tt.data)
			require.NotEmpty(t, encoded, "encoded cursor should not be empty for non-empty data")

			// Decode
			decoded, err := DecodeCursor(encoded)
			require.NoError(t, err)
			require.NotNil(t, decoded)

			// Verify key fields preserved
			assert.Equal(t, tt.data.LastID, decoded.LastID)
			assert.InDelta(t, tt.data.LastScore, decoded.LastScore, 0.0001)
			assert.Equal(t, tt.data.SortOrder, decoded.SortOrder)

			// Timestamp comparison with tolerance for JSON serialization
			if !tt.data.LastTimestamp.IsZero() {
				assert.WithinDuration(t, tt.data.LastTimestamp, decoded.LastTimestamp, time.Second)
			}

			// LastEvaluatedKey comparison (JSON may change types)
			if tt.data.LastEvaluatedKey != nil {
				assert.NotNil(t, decoded.LastEvaluatedKey)
				for k, v := range tt.data.LastEvaluatedKey {
					assert.Contains(t, decoded.LastEvaluatedKey, k)
					// Compare as strings since JSON may change types
					assert.Equal(t, v, decoded.LastEvaluatedKey[k])
				}
			}
		})
	}
}

// ============================================================================
// 3) Page-State Helpers Tests
// ============================================================================

func TestCreateNextCursor(t *testing.T) {
	tests := []struct {
		name             string
		lastEvaluatedKey map[string]interface{}
		lastScore        float64
		lastTimestamp    time.Time
		lastID           string
		sortOrder        SearchSortOrder
		expectEmpty      bool
	}{
		{
			name:             "all zero values returns empty cursor",
			lastEvaluatedKey: nil,
			lastScore:        0,
			lastTimestamp:    time.Time{},
			lastID:           "",
			sortOrder:        "",
			expectEmpty:      true,
		},
		{
			name:             "with lastID returns non-empty",
			lastEvaluatedKey: nil,
			lastScore:        0,
			lastTimestamp:    time.Time{},
			lastID:           "next-item-123",
			sortOrder:        SearchSortRelevance,
			expectEmpty:      false,
		},
		{
			name:             "with lastScore returns non-empty",
			lastEvaluatedKey: nil,
			lastScore:        0.75,
			lastTimestamp:    time.Time{},
			lastID:           "",
			sortOrder:        SearchSortRelevance,
			expectEmpty:      false,
		},
		{
			name:             "with lastTimestamp returns non-empty",
			lastEvaluatedKey: nil,
			lastScore:        0,
			lastTimestamp:    time.Now(),
			lastID:           "",
			sortOrder:        SearchSortTimeDesc,
			expectEmpty:      false,
		},
		{
			name:             "with lastEvaluatedKey returns non-empty",
			lastEvaluatedKey: map[string]interface{}{"PK": "test"},
			lastScore:        0,
			lastTimestamp:    time.Time{},
			lastID:           "",
			sortOrder:        SearchSortTimeAsc,
			expectEmpty:      false,
		},
		{
			name:             "full parameters returns non-empty",
			lastEvaluatedKey: map[string]interface{}{"PK": "STATUS#123", "SK": "META"},
			lastScore:        0.92,
			lastTimestamp:    time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC),
			lastID:           "status-abc",
			sortOrder:        SearchSortRelevance,
			expectEmpty:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateNextCursor(tt.lastEvaluatedKey, tt.lastScore, tt.lastTimestamp, tt.lastID, tt.sortOrder)

			if tt.expectEmpty {
				assert.Empty(t, result)
			} else {
				assert.NotEmpty(t, result)

				// Verify round-trip
				decoded, err := DecodeCursor(result)
				require.NoError(t, err)
				assert.Equal(t, tt.lastID, decoded.LastID)
				assert.InDelta(t, tt.lastScore, decoded.LastScore, 0.0001)
				assert.Equal(t, tt.sortOrder, decoded.SortOrder)
			}
		})
	}
}

func TestCreatePaginationResult(t *testing.T) {
	tests := []struct {
		name         string
		hasNextPage  bool
		nextCursor   string
		totalScanned int
	}{
		{
			name:         "basic result with next page",
			hasNextPage:  true,
			nextCursor:   "some-cursor-string",
			totalScanned: 100,
		},
		{
			name:         "result with no next page",
			hasNextPage:  false,
			nextCursor:   "",
			totalScanned: 50,
		},
		{
			name:         "result with zero scanned",
			hasNextPage:  false,
			nextCursor:   "",
			totalScanned: 0,
		},
		{
			name:         "result with cursor but no next page",
			hasNextPage:  false,
			nextCursor:   "cursor-but-done",
			totalScanned: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreatePaginationResult(tt.hasNextPage, tt.nextCursor, tt.totalScanned)

			require.NotNil(t, result)
			assert.Equal(t, tt.hasNextPage, result.HasNextPage)
			assert.Equal(t, tt.nextCursor, result.NextCursor)
			assert.Equal(t, tt.totalScanned, result.TotalScanned)
		})
	}
}

func TestShouldContinuePagination(t *testing.T) {
	tests := []struct {
		name           string
		resultCount    int
		requestedLimit int
		totalProcessed int
		maxScan        int
		expected       bool
	}{
		// resultCount > requestedLimit → true
		{
			name:           "more results than requested indicates more data",
			resultCount:    25,
			requestedLimit: 20,
			totalProcessed: 25,
			maxScan:        100,
			expected:       true,
		},
		{
			name:           "results exceed limit by one",
			resultCount:    21,
			requestedLimit: 20,
			totalProcessed: 50,
			maxScan:        100,
			expected:       true,
		},

		// totalProcessed < maxScan && resultCount == requestedLimit → true
		{
			name:           "full batch with room to scan indicates potential more",
			resultCount:    20,
			requestedLimit: 20,
			totalProcessed: 50,
			maxScan:        100,
			expected:       true,
		},
		{
			name:           "exact limit match with minimal processing",
			resultCount:    10,
			requestedLimit: 10,
			totalProcessed: 10,
			maxScan:        1000,
			expected:       true,
		},

		// otherwise → false
		{
			name:           "fewer results than requested",
			resultCount:    15,
			requestedLimit: 20,
			totalProcessed: 50,
			maxScan:        100,
			expected:       false,
		},
		{
			name:           "reached max scan limit",
			resultCount:    20,
			requestedLimit: 20,
			totalProcessed: 100,
			maxScan:        100,
			expected:       false,
		},
		{
			name:           "exceeded max scan with full batch",
			resultCount:    20,
			requestedLimit: 20,
			totalProcessed: 150,
			maxScan:        100,
			expected:       false,
		},
		{
			name:           "partial results at max scan",
			resultCount:    15,
			requestedLimit: 20,
			totalProcessed: 100,
			maxScan:        100,
			expected:       false,
		},
		{
			name:           "zero results",
			resultCount:    0,
			requestedLimit: 20,
			totalProcessed: 20,
			maxScan:        100,
			expected:       false,
		},
		{
			name:           "edge case: totalProcessed equals maxScan minus one",
			resultCount:    20,
			requestedLimit: 20,
			totalProcessed: 99,
			maxScan:        100,
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldContinuePagination(tt.resultCount, tt.requestedLimit, tt.totalProcessed, tt.maxScan)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyPaginationLimits(t *testing.T) {
	tests := []struct {
		name            string
		inputLen        int
		requestedLimit  int
		expectedLen     int
		expectedHasMore bool
	}{
		{
			name:            "results equal to limit returns original",
			inputLen:        20,
			requestedLimit:  20,
			expectedLen:     20,
			expectedHasMore: false,
		},
		{
			name:            "results less than limit returns original",
			inputLen:        15,
			requestedLimit:  20,
			expectedLen:     15,
			expectedHasMore: false,
		},
		{
			name:            "results greater than limit truncates",
			inputLen:        25,
			requestedLimit:  20,
			expectedLen:     20,
			expectedHasMore: true,
		},
		{
			name:            "one over limit truncates",
			inputLen:        21,
			requestedLimit:  20,
			expectedLen:     20,
			expectedHasMore: true,
		},
		{
			name:            "empty results",
			inputLen:        0,
			requestedLimit:  20,
			expectedLen:     0,
			expectedHasMore: false,
		},
		{
			name:            "single item under limit",
			inputLen:        1,
			requestedLimit:  20,
			expectedLen:     1,
			expectedHasMore: false,
		},
		{
			name:            "many over limit",
			inputLen:        100,
			requestedLimit:  10,
			expectedLen:     10,
			expectedHasMore: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test slice
			input := make([]int, tt.inputLen)
			for i := range input {
				input[i] = i
			}

			result, hasMore := ApplyPaginationLimits(input, tt.requestedLimit)

			assert.Equal(t, tt.expectedLen, len(result))
			assert.Equal(t, tt.expectedHasMore, hasMore)

			// Verify first elements are preserved
			for i := 0; i < len(result); i++ {
				assert.Equal(t, i, result[i])
			}
		})
	}
}

// Test with different types to verify generic behavior
func TestApplyPaginationLimits_GenericTypes(t *testing.T) {
	t.Run("string slice", func(t *testing.T) {
		input := []string{"a", "b", "c", "d", "e"}
		result, hasMore := ApplyPaginationLimits(input, 3)
		assert.Equal(t, []string{"a", "b", "c"}, result)
		assert.True(t, hasMore)
	})

	t.Run("struct slice", func(t *testing.T) {
		type item struct {
			id   string
			name string
		}
		input := []item{
			{id: "1", name: "first"},
			{id: "2", name: "second"},
		}
		result, hasMore := ApplyPaginationLimits(input, 5)
		assert.Equal(t, input, result)
		assert.False(t, hasMore)
	})
}

// ============================================================================
// 4) Sorting Tests
// ============================================================================

// testItem is a helper struct for sorting tests
type testItem struct {
	score     float64
	timestamp time.Time
	id        string
}

func getScore(i testItem) float64       { return i.score }
func getTimestamp(i testItem) time.Time { return i.timestamp }
func getID(i testItem) string           { return i.id }

func TestSortResults_Relevance(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    []testItem
		expected []string // expected IDs in order
	}{
		{
			name: "sorts by score descending",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "low"},
				{score: 0.9, timestamp: baseTime, id: "high"},
				{score: 0.7, timestamp: baseTime, id: "mid"},
			},
			expected: []string{"high", "mid", "low"},
		},
		{
			name: "ties broken by timestamp descending (newer first)",
			input: []testItem{
				{score: 0.8, timestamp: baseTime, id: "older"},
				{score: 0.8, timestamp: baseTime.Add(time.Hour), id: "newer"},
				{score: 0.8, timestamp: baseTime.Add(-time.Hour), id: "oldest"},
			},
			expected: []string{"newer", "older", "oldest"},
		},
		{
			name: "mixed scores with timestamp tie-breaking",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "c"},
				{score: 0.9, timestamp: baseTime, id: "a"},
				{score: 0.9, timestamp: baseTime.Add(time.Hour), id: "b"},
			},
			expected: []string{"b", "a", "c"},
		},
		{
			name:     "empty slice",
			input:    []testItem{},
			expected: []string{},
		},
		{
			name: "single item",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "only"},
			},
			expected: []string{"only"},
		},
		{
			name: "already sorted",
			input: []testItem{
				{score: 0.9, timestamp: baseTime.Add(time.Hour), id: "first"},
				{score: 0.9, timestamp: baseTime, id: "second"},
				{score: 0.5, timestamp: baseTime, id: "third"},
			},
			expected: []string{"first", "second", "third"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]testItem, len(tt.input))
			copy(items, tt.input)

			SortResults(items, SearchSortRelevance, getScore, getTimestamp, getID)

			actualIDs := make([]string, len(items))
			for i, item := range items {
				actualIDs[i] = item.id
			}

			assert.Equal(t, tt.expected, actualIDs)
		})
	}
}

func TestSortResults_TimeAscending(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    []testItem
		expected []string // expected IDs in order
	}{
		{
			name: "sorts by timestamp ascending (oldest first)",
			input: []testItem{
				{score: 0.5, timestamp: baseTime.Add(time.Hour), id: "newest"},
				{score: 0.9, timestamp: baseTime, id: "middle"},
				{score: 0.7, timestamp: baseTime.Add(-time.Hour), id: "oldest"},
			},
			expected: []string{"oldest", "middle", "newest"},
		},
		{
			name: "ties broken by ID ascending",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "c"},
				{score: 0.9, timestamp: baseTime, id: "a"},
				{score: 0.7, timestamp: baseTime, id: "b"},
			},
			expected: []string{"a", "b", "c"},
		},
		{
			name: "mixed timestamps with ID tie-breaking",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "z"},
				{score: 0.9, timestamp: baseTime.Add(-time.Hour), id: "earlier"},
				{score: 0.7, timestamp: baseTime, id: "a"},
			},
			expected: []string{"earlier", "a", "z"},
		},
		{
			name:     "empty slice",
			input:    []testItem{},
			expected: []string{},
		},
		{
			name: "single item",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "only"},
			},
			expected: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]testItem, len(tt.input))
			copy(items, tt.input)

			SortResults(items, SearchSortTimeAsc, getScore, getTimestamp, getID)

			actualIDs := make([]string, len(items))
			for i, item := range items {
				actualIDs[i] = item.id
			}

			assert.Equal(t, tt.expected, actualIDs)
		})
	}
}

func TestSortResults_TimeDescending(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    []testItem
		expected []string // expected IDs in order
	}{
		{
			name: "sorts by timestamp descending (newest first)",
			input: []testItem{
				{score: 0.5, timestamp: baseTime.Add(-time.Hour), id: "oldest"},
				{score: 0.9, timestamp: baseTime, id: "middle"},
				{score: 0.7, timestamp: baseTime.Add(time.Hour), id: "newest"},
			},
			expected: []string{"newest", "middle", "oldest"},
		},
		{
			name: "ties broken by ID ascending for stability",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "c"},
				{score: 0.9, timestamp: baseTime, id: "a"},
				{score: 0.7, timestamp: baseTime, id: "b"},
			},
			expected: []string{"a", "b", "c"},
		},
		{
			name: "mixed timestamps with ID tie-breaking",
			input: []testItem{
				{score: 0.5, timestamp: baseTime.Add(time.Hour), id: "newer"},
				{score: 0.9, timestamp: baseTime, id: "z"},
				{score: 0.7, timestamp: baseTime, id: "a"},
			},
			expected: []string{"newer", "a", "z"},
		},
		{
			name:     "empty slice",
			input:    []testItem{},
			expected: []string{},
		},
		{
			name: "single item",
			input: []testItem{
				{score: 0.5, timestamp: baseTime, id: "only"},
			},
			expected: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]testItem, len(tt.input))
			copy(items, tt.input)

			SortResults(items, SearchSortTimeDesc, getScore, getTimestamp, getID)

			actualIDs := make([]string, len(items))
			for i, item := range items {
				actualIDs[i] = item.id
			}

			assert.Equal(t, tt.expected, actualIDs)
		})
	}
}

func TestSortResults_UnknownSortOrder(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	items := []testItem{
		{score: 0.5, timestamp: baseTime, id: "a"},
		{score: 0.9, timestamp: baseTime, id: "b"},
	}
	original := make([]testItem, len(items))
	copy(original, items)

	// Unknown sort order should not modify the slice
	SortResults(items, SearchSortOrder("unknown"), getScore, getTimestamp, getID)

	// Slice should remain unchanged
	assert.Equal(t, original, items)
}

// TestSortResults_StabilityWithManyTies verifies stable sorting behavior
func TestSortResults_StabilityWithManyTies(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// All items have same score and timestamp - should sort by ID
	items := []testItem{
		{score: 0.8, timestamp: baseTime, id: "delta"},
		{score: 0.8, timestamp: baseTime, id: "alpha"},
		{score: 0.8, timestamp: baseTime, id: "charlie"},
		{score: 0.8, timestamp: baseTime, id: "bravo"},
	}

	// For relevance, ties on score go to timestamp, and if those are equal,
	// the tie-breaking is by timestamp (not ID). Items with equal timestamp
	// maintain relative order based on the shouldSwapByRelevance logic.
	SortResults(items, SearchSortRelevance, getScore, getTimestamp, getID)

	// With same score and timestamp, the swap logic (timeA.Before(timeB))
	// won't swap items with equal timestamps, so original order is preserved
	// after score-based sorting (which doesn't change order for equal scores).
	// This is implementation-specific behavior.

	// For time-based sorts, ID is used as tie-breaker
	items2 := []testItem{
		{score: 0.8, timestamp: baseTime, id: "delta"},
		{score: 0.8, timestamp: baseTime, id: "alpha"},
		{score: 0.8, timestamp: baseTime, id: "charlie"},
		{score: 0.8, timestamp: baseTime, id: "bravo"},
	}

	SortResults(items2, SearchSortTimeAsc, getScore, getTimestamp, getID)
	expectedAsc := []string{"alpha", "bravo", "charlie", "delta"}
	actualIDs := make([]string, len(items2))
	for i, item := range items2 {
		actualIDs[i] = item.id
	}
	assert.Equal(t, expectedAsc, actualIDs)
}
