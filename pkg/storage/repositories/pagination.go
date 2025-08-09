package repositories

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// SearchSortOrder represents sorting options for search
type SearchSortOrder string

// Search sort order constants
const (
	// SearchSortRelevance sorts by relevance/score (default)
	SearchSortRelevance  SearchSortOrder = "relevance"
	// SearchSortTimeAsc sorts by time ascending (oldest first)
	SearchSortTimeAsc    SearchSortOrder = "time_asc"
	// SearchSortTimeDesc sorts by time descending (newest first)
	SearchSortTimeDesc   SearchSortOrder = "time_desc"
)

// PaginationOptions represents pagination parameters for search
type PaginationOptions struct {
	Cursor    string          `json:"cursor,omitempty"`     // Cursor for pagination
	Limit     int             `json:"limit"`                // Page size (default 20, max 50)
	SortOrder SearchSortOrder `json:"sort_order,omitempty"` // Sorting preference
}

// PaginationResult represents paginated search results
type PaginationResult struct {
	NextCursor   string `json:"next_cursor,omitempty"`   // Cursor for next page
	HasNextPage  bool   `json:"has_next_page"`           // Whether more results exist
	TotalScanned int    `json:"total_scanned,omitempty"` // Total items scanned (optional)
}

// CursorData represents the data stored in a pagination cursor
type CursorData struct {
	LastEvaluatedKey map[string]interface{} `json:"last_evaluated_key,omitempty"` // DynamORM's LastEvaluatedKey
	LastScore        float64                `json:"last_score,omitempty"`         // Last item's score for relevance sorting
	LastTimestamp    time.Time              `json:"last_timestamp,omitempty"`     // Last item's timestamp for time sorting
	LastID           string                 `json:"last_id,omitempty"`            // Last item's ID for tie-breaking
	SortOrder        SearchSortOrder        `json:"sort_order"`                   // Sort order used
}

// NewPaginationOptions creates pagination options with defaults
func NewPaginationOptions() *PaginationOptions {
	return &PaginationOptions{
		Limit:     20,
		SortOrder: SearchSortRelevance,
	}
}

// Validate validates and normalizes pagination options
func (p *PaginationOptions) Validate() error {
	// Set defaults
	if p.Limit <= 0 {
		p.Limit = 20
	}
	
	// Enforce maximum limit for search operations
	if p.Limit > 50 {
		p.Limit = 50
	}
	
	// Set default sort order
	if p.SortOrder == "" {
		p.SortOrder = SearchSortRelevance
	}
	
	// Validate sort order
	switch p.SortOrder {
	case SearchSortRelevance, SearchSortTimeAsc, SearchSortTimeDesc:
		// Valid
	default:
		return fmt.Errorf("invalid sort order: %s", p.SortOrder)
	}
	
	return nil
}

// EncodeCursor creates a cursor string from cursor data
func EncodeCursor(data *CursorData) string {
	if data == nil {
		return ""
	}
	
	// Handle empty cursor data
	if data.LastEvaluatedKey == nil && data.LastID == "" && data.LastScore == 0 && data.LastTimestamp.IsZero() {
		return ""
	}
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	
	return base64.URLEncoding.EncodeToString(jsonData)
}

// DecodeCursor parses a cursor string back to cursor data
func DecodeCursor(cursor string) (*CursorData, error) {
	if cursor == "" {
		return &CursorData{}, nil
	}
	
	jsonData, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}
	
	var data CursorData
	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor data: %w", err)
	}
	
	return &data, nil
}

// CreateNextCursor creates a cursor for the next page based on the last item
func CreateNextCursor(lastEvaluatedKey map[string]interface{}, lastScore float64, lastTimestamp time.Time, lastID string, sortOrder SearchSortOrder) string {
	data := &CursorData{
		LastEvaluatedKey: lastEvaluatedKey,
		LastScore:        lastScore,
		LastTimestamp:    lastTimestamp,
		LastID:           lastID,
		SortOrder:        sortOrder,
	}
	
	return EncodeCursor(data)
}

// CreatePaginationResult creates a pagination result with next cursor
func CreatePaginationResult(hasNextPage bool, nextCursor string, totalScanned int) *PaginationResult {
	return &PaginationResult{
		NextCursor:   nextCursor,
		HasNextPage:  hasNextPage,
		TotalScanned: totalScanned,
	}
}

// ShouldContinuePagination determines if pagination should continue based on results and limits
func ShouldContinuePagination(resultCount, requestedLimit, totalProcessed, maxScan int) bool {
	// Continue if we have more results than requested (indicates more data available)
	if resultCount > requestedLimit {
		return true
	}
	
	// Continue if we haven't reached the scan limit and got full batch (might have more)
	if totalProcessed < maxScan && resultCount == requestedLimit {
		return true
	}
	
	return false
}

// ApplyPaginationLimits applies limit to results and determines if there are more pages
func ApplyPaginationLimits[T any](results []T, requestedLimit int) ([]T, bool) {
	if len(results) <= requestedLimit {
		return results, false
	}
	
	// Return requested limit and indicate there are more results
	return results[:requestedLimit], true
}

// SortResults sorts results based on the specified sort order
func SortResults[T any](results []T, sortOrder SearchSortOrder, getScore func(T) float64, getTimestamp func(T) time.Time, getID func(T) string) {
	switch sortOrder {
	case SearchSortRelevance:
		sortByRelevance(results, getScore, getTimestamp)
	case SearchSortTimeAsc:
		sortByTimeAscending(results, getTimestamp, getID)
	case SearchSortTimeDesc:
		sortByTimeDescending(results, getTimestamp, getID)
	}
}

// sortByRelevance sorts by score descending, then by timestamp descending for ties
func sortByRelevance[T any](results []T, getScore func(T) float64, getTimestamp func(T) time.Time) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if shouldSwapByRelevance(results[i], results[j], getScore, getTimestamp) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// sortByTimeAscending sorts by timestamp ascending, then by ID for stable sort
func sortByTimeAscending[T any](results []T, getTimestamp func(T) time.Time, getID func(T) string) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if shouldSwapByTimeAscending(results[i], results[j], getTimestamp, getID) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// sortByTimeDescending sorts by timestamp descending, then by ID for stable sort
func sortByTimeDescending[T any](results []T, getTimestamp func(T) time.Time, getID func(T) string) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if shouldSwapByTimeDescending(results[i], results[j], getTimestamp, getID) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// shouldSwapByRelevance determines if two items should be swapped based on relevance
func shouldSwapByRelevance[T any](a, b T, getScore func(T) float64, getTimestamp func(T) time.Time) bool {
	scoreA, scoreB := getScore(a), getScore(b)
	if scoreA < scoreB {
		return true
	}
	if scoreA == scoreB {
		timeA, timeB := getTimestamp(a), getTimestamp(b)
		return timeA.Before(timeB)
	}
	return false
}

// shouldSwapByTimeAscending determines if two items should be swapped for ascending time sort
func shouldSwapByTimeAscending[T any](a, b T, getTimestamp func(T) time.Time, getID func(T) string) bool {
	timeA, timeB := getTimestamp(a), getTimestamp(b)
	if timeA.After(timeB) {
		return true
	}
	if timeA.Equal(timeB) {
		idA, idB := getID(a), getID(b)
		return idA > idB
	}
	return false
}

// shouldSwapByTimeDescending determines if two items should be swapped for descending time sort
func shouldSwapByTimeDescending[T any](a, b T, getTimestamp func(T) time.Time, getID func(T) string) bool {
	timeA, timeB := getTimestamp(a), getTimestamp(b)
	if timeA.Before(timeB) {
		return true
	}
	if timeA.Equal(timeB) {
		idA, idB := getID(a), getID(b)
		return idA > idB
	}
	return false
}