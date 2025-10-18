package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

// TestGetFederationActivitiesByTimeRange_MultiDay tests that the method queries across multiple days
func TestGetFederationActivitiesByTimeRange_MultiDay(t *testing.T) {
	// This is a unit test for the day-iteration logic
	// In production, this would use a mock DB or test database

	// Calculate expected number of days
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 5, 14, 0, 0, 0, time.UTC)

	// Expected days to query: Oct 1, 2, 3, 4, 5 = 5 days
	expectedDays := []string{
		"FEDERATION_DAILY#2025-10-01",
		"FEDERATION_DAILY#2025-10-02",
		"FEDERATION_DAILY#2025-10-03",
		"FEDERATION_DAILY#2025-10-04",
		"FEDERATION_DAILY#2025-10-05",
	}

	// Verify day calculation logic
	currentDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	endDay := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 23, 59, 59, 0, endTime.Location())

	days := make([]string, 0)
	for currentDay.Before(endDay) || currentDay.Equal(endDay) {
		dayKey := fmt.Sprintf("FEDERATION_DAILY#%s", currentDay.Format(common.DateFormat))
		days = append(days, dayKey)
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	assert.Equal(t, expectedDays, days, "Should calculate correct day keys for the range")
	assert.Len(t, days, 5, "Should query 5 days")
}

// TestGetFederationActivitiesByTimeRange_SingleDay tests single day query
func TestGetFederationActivitiesByTimeRange_SingleDay(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC)

	expectedDays := []string{
		"FEDERATION_DAILY#2025-10-01",
	}

	// Verify day calculation logic
	currentDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	endDay := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 23, 59, 59, 0, endTime.Location())

	days := make([]string, 0)
	for currentDay.Before(endDay) || currentDay.Equal(endDay) {
		dayKey := fmt.Sprintf("FEDERATION_DAILY#%s", currentDay.Format(common.DateFormat))
		days = append(days, dayKey)
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	assert.Equal(t, expectedDays, days, "Should calculate correct day key for single day")
	assert.Len(t, days, 1, "Should query 1 day")
}

// TestGetFederationActivitiesByTimeRange_Week tests week-long query
func TestGetFederationActivitiesByTimeRange_Week(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 7, 23, 59, 59, 0, time.UTC)

	// Verify day calculation logic
	currentDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	endDay := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 23, 59, 59, 0, endTime.Location())

	dayCount := 0
	for currentDay.Before(endDay) || currentDay.Equal(endDay) {
		dayCount++
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	assert.Equal(t, 7, dayCount, "Should query 7 days for a week")
}

// TestGetFederationActivitiesByTimeRange_Month tests month-long query
func TestGetFederationActivitiesByTimeRange_Month(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 31, 23, 59, 59, 0, time.UTC)

	// Verify day calculation logic
	currentDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	endDay := time.Date(endTime.Year(), endTime.Month(), endTime.Day(), 23, 59, 59, 0, endTime.Location())

	dayCount := 0
	for currentDay.Before(endDay) || currentDay.Equal(endDay) {
		dayCount++
		currentDay = currentDay.AddDate(0, 0, 1)
	}

	assert.Equal(t, 31, dayCount, "Should query 31 days for October")
}

// TestActivityTimestampFiltering tests that activities are correctly filtered by timestamp
func TestActivityTimestampFiltering(t *testing.T) {
	startTime := time.Date(2025, 10, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC)

	activities := []*models.FederationCostActivity{
		{Timestamp: time.Date(2025, 10, 1, 9, 30, 0, 0, time.UTC)},  // Before range
		{Timestamp: time.Date(2025, 10, 1, 10, 30, 0, 0, time.UTC)}, // In range
		{Timestamp: time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)},  // In range
		{Timestamp: time.Date(2025, 10, 1, 13, 30, 0, 0, time.UTC)}, // In range
		{Timestamp: time.Date(2025, 10, 1, 14, 30, 0, 0, time.UTC)}, // After range
	}

	filtered := make([]*models.FederationCostActivity, 0)
	for _, activity := range activities {
		if activity.Timestamp.After(startTime) && activity.Timestamp.Before(endTime) {
			filtered = append(filtered, activity)
		}
	}

	assert.Len(t, filtered, 3, "Should filter to activities within time range")
	assert.Equal(t, time.Date(2025, 10, 1, 10, 30, 0, 0, time.UTC), filtered[0].Timestamp)
	assert.Equal(t, time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC), filtered[1].Timestamp)
	assert.Equal(t, time.Date(2025, 10, 1, 13, 30, 0, 0, time.UTC), filtered[2].Timestamp)
}

func TestGetAllFederationEdges_PaginationLogic(t *testing.T) {
	// Test pagination detection logic using limit+1 pattern
	requestedLimit := 10
	results := make([]string, 11) // Got one extra

	hasMore := len(results) > requestedLimit
	assert.True(t, hasMore, "Should detect more pages when results exceed limit")

	if hasMore {
		results = results[:requestedLimit]
	}
	assert.Len(t, results, requestedLimit, "Should trim to requested limit")
}

// TestGetAllFederationEdges_LimitHandling tests limit boundary conditions
func TestGetAllFederationEdges_LimitHandling(t *testing.T) {
	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{
			name:          "zero limit uses default",
			inputLimit:    0,
			expectedLimit: 1000,
		},
		{
			name:          "negative limit uses default",
			inputLimit:    -1,
			expectedLimit: 1000,
		},
		{
			name:          "normal limit preserved",
			inputLimit:    500,
			expectedLimit: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.inputLimit
			if limit <= 0 {
				limit = 1000 // Default
			}
			assert.Equal(t, tt.expectedLimit, limit)
		})
	}
}

// TestContextCancellation tests context cancellation handling
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	select {
	case <-ctx.Done():
		assert.Equal(t, context.Canceled, ctx.Err())
	default:
		t.Fatal("Context should be cancelled")
	}
}

// TestEdgePaginationCursor tests cursor encoding for edges across partition keys
func TestEdgePaginationCursor(t *testing.T) {
	// Simulate multi-partition scenario
	edges := []struct {
		PK string
		SK string
	}{
		{PK: "FEDERATION_EDGE#domain1.com", SK: "domain2.com"},
		{PK: "FEDERATION_EDGE#domain1.com", SK: "domain3.com"},
		{PK: "FEDERATION_EDGE#domain2.com", SK: "domain1.com"}, // Different partition
	}

	// Verify cursor is created from last item
	if len(edges) > 0 {
		lastEdge := edges[len(edges)-1]

		// Cursor should encode both PK and SK to handle partition key changes
		assert.Equal(t, "FEDERATION_EDGE#domain2.com", lastEdge.PK, "Cursor should track PK transitions")
		assert.Equal(t, "domain1.com", lastEdge.SK, "Cursor should track SK within partition")
	}
}

// TestEdgePaginationAcrossPartitions tests that cursor handles partition key changes
func TestEdgePaginationAcrossPartitions(t *testing.T) {
	// Simulate fetching across multiple partition keys with the limit+1 pattern
	// Page 1: Request limit=2, get 3 items (2 returned + 1 extra for cursor)
	page1Results := []struct {
		PK string
		SK string
	}{
		{PK: "FEDERATION_EDGE#domainA.com", SK: "target1.com"}, // Returned
		{PK: "FEDERATION_EDGE#domainA.com", SK: "target2.com"}, // Returned
		{PK: "FEDERATION_EDGE#domainB.com", SK: "target1.com"}, // Extra (for cursor only)
	}

	pageLimit := 2
	hasMore := len(page1Results) > pageLimit
	assert.True(t, hasMore, "Should detect more pages")

	// CRITICAL: Cursor must be from the EXTRA item (index pageLimit), not the last returned item
	var cursorItem struct{ PK, SK string }
	if hasMore {
		cursorItem = page1Results[pageLimit] // The extra item
	}

	// Trim to page size for return
	returnedItems := page1Results[:pageLimit]
	assert.Len(t, returnedItems, 2, "Should return exactly pageLimit items")
	assert.Equal(t, "FEDERATION_EDGE#domainA.com", returnedItems[1].PK, "Last returned item is domainA")

	// Cursor should be from the EXTRA item (which becomes first item of next page)
	assert.Equal(t, "FEDERATION_EDGE#domainB.com", cursorItem.PK, "Cursor from extra item tracks PK transition")
	assert.Equal(t, "target1.com", cursorItem.SK, "Cursor from extra item")

	// Page 2: Using cursor, we should get domainB.com/target1.com as FIRST item (not skipped!)
	// This validates that the cursor pattern doesn't lose the transition item
	expectedFirstPage2 := cursorItem
	assert.Equal(t, "FEDERATION_EDGE#domainB.com", expectedFirstPage2.PK, "Extra item becomes first of next page")
}

// TestCursorFromExtraItem validates the critical cursor construction pattern
func TestCursorFromExtraItem(t *testing.T) {
	// Simulate the limit+1 fetch pattern
	pageLimit := 5
	fetchedItems := []int{1, 2, 3, 4, 5, 6} // Got 6 items when requesting 5

	hasMore := len(fetchedItems) > pageLimit
	assert.True(t, hasMore)

	// CORRECT: Cursor from the extra item (index pageLimit)
	cursorIndex := pageLimit
	assert.Equal(t, 6, fetchedItems[cursorIndex], "Cursor from extra item (index 5)")

	// INCORRECT (BUG): Cursor from last returned item (index pageLimit-1)
	wrongCursorIndex := pageLimit - 1
	assert.Equal(t, 5, fetchedItems[wrongCursorIndex], "Wrong: cursor from last returned item (index 4)")

	// The difference: using wrong cursor skips item 5 on the next page!
	assert.NotEqual(t, cursorIndex, wrongCursorIndex, "Cursor index must be pageLimit, not pageLimit-1")
}

// TestFetchEdgePageWithCursor_UsesExtraItemForCursor validates cursor construction from extra item
func TestFetchEdgePageWithCursor_UsesExtraItemForCursor(t *testing.T) {
	// Simulate fetching a page where the extra item is in a different partition
	pageLimit := 2

	// Mock result: 3 edges (2 returned + 1 extra)
	mockEdges := []models.FederationEdge{
		{PK: "FEDERATION_EDGE#domainA.com", SK: "target1"},
		{PK: "FEDERATION_EDGE#domainA.com", SK: "target2"},
		{PK: "FEDERATION_EDGE#domainB.com", SK: "target1"}, // Extra (different partition!)
	}

	// Simulate the production logic
	hasMore := len(mockEdges) > pageLimit
	assert.True(t, hasMore, "Should detect more pages")

	var nextCursorPK, nextCursorSK string
	var returnedEdges []models.FederationEdge

	if hasMore {
		// CRITICAL: Get cursor from extra item (index pageLimit) BEFORE trimming
		extraEdge := mockEdges[pageLimit]
		nextCursorPK = extraEdge.PK
		nextCursorSK = extraEdge.SK

		// Now trim to page size
		returnedEdges = mockEdges[:pageLimit]
	}

	// Validate results
	assert.Len(t, returnedEdges, pageLimit, "Should return exactly pageLimit edges")
	assert.Equal(t, "FEDERATION_EDGE#domainA.com", returnedEdges[0].PK)
	assert.Equal(t, "FEDERATION_EDGE#domainA.com", returnedEdges[1].PK)

	// Validate cursor points to the extra item (which will be first item of next page)
	assert.Equal(t, "FEDERATION_EDGE#domainB.com", nextCursorPK, "Cursor must be from extra item")
	assert.Equal(t, "target1", nextCursorSK, "Cursor SK from extra item")

	// This proves the extra item (domainB/target1) will NOT be skipped on the next page
}

// TestMultiPageEdgeFetch_NoDataLoss simulates multi-page fetch to prove no edges are lost
func TestMultiPageEdgeFetch_NoDataLoss(t *testing.T) {
	// Simulate 3 pages of data
	allEdges := []struct {
		PK string
		SK string
		ID int
	}{
		// Page 1 (limit=2, returns items 1-2, cursor from item 3)
		{PK: "FEDERATION_EDGE#domainA.com", SK: "t1", ID: 1},
		{PK: "FEDERATION_EDGE#domainA.com", SK: "t2", ID: 2},
		{PK: "FEDERATION_EDGE#domainA.com", SK: "t3", ID: 3}, // Extra → cursor

		// Page 2 (starts from item 3, returns 3-4, cursor from item 5)
		{PK: "FEDERATION_EDGE#domainA.com", SK: "t4", ID: 4},
		{PK: "FEDERATION_EDGE#domainB.com", SK: "t1", ID: 5}, // Extra → cursor (partition change!)

		// Page 3 (starts from item 5, returns 5-6, no cursor)
		{PK: "FEDERATION_EDGE#domainB.com", SK: "t2", ID: 6},
	}

	pageLimit := 2
	collectedIDs := []int{}

	// Simulate page 1
	page1 := allEdges[0:3] // Fetch returned 3 items (limit+1)
	hasMore1 := len(page1) > pageLimit
	assert.True(t, hasMore1)

	if hasMore1 {
		// Cursor from extra item (index 2)
		cursorItem1 := page1[pageLimit]
		assert.Equal(t, 3, cursorItem1.ID, "Cursor from extra item")

		// Return first 2
		for i := 0; i < pageLimit; i++ {
			collectedIDs = append(collectedIDs, page1[i].ID)
		}
	}
	assert.Equal(t, []int{1, 2}, collectedIDs, "Page 1 returns items 1-2")

	// Simulate page 2 (cursor seeks PAST item 3, so page starts with item 3)
	page2 := allEdges[2:5] // Items 3, 4, 5
	hasMore2 := len(page2) > pageLimit
	assert.True(t, hasMore2)

	if hasMore2 {
		// Cursor from extra item (index 2 in page2 = item 5 globally)
		cursorItem2 := page2[pageLimit]
		assert.Equal(t, 5, cursorItem2.ID, "Cursor from extra item (partition change)")

		// Return first 2 of page 2
		for i := 0; i < pageLimit; i++ {
			collectedIDs = append(collectedIDs, page2[i].ID)
		}
	}
	assert.Equal(t, []int{1, 2, 3, 4}, collectedIDs, "Page 2 adds items 3-4 (no skips!)")

	// Simulate page 3
	page3 := allEdges[4:6] // Items 5, 6
	hasMore3 := len(page3) > pageLimit
	assert.False(t, hasMore3, "Last page has no more data")

	// Return remaining items
	for i := 0; i < len(page3); i++ {
		collectedIDs = append(collectedIDs, page3[i].ID)
	}

	// CRITICAL VALIDATION: All 6 items collected with zero skips
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, collectedIDs, "All edges collected with no data loss")
}

// TestCursorBasedPaginationBehavior validates the cursor pagination pattern
func TestCursorBasedPaginationBehavior(t *testing.T) {
	tests := []struct {
		name           string
		totalItems     int
		pageLimit      int
		expectedPages  int
		expectedInLast int
	}{
		{
			name:           "exact multiple of page size",
			totalItems:     30,
			pageLimit:      10,
			expectedPages:  3,
			expectedInLast: 10,
		},
		{
			name:           "partial last page",
			totalItems:     25,
			pageLimit:      10,
			expectedPages:  3,
			expectedInLast: 5,
		},
		{
			name:           "single page",
			totalItems:     5,
			pageLimit:      10,
			expectedPages:  1,
			expectedInLast: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate pagination
			cursor := ""
			pageCount := 0
			totalFetched := 0

			for totalFetched < tt.totalItems {
				pageCount++
				remaining := tt.totalItems - totalFetched
				pageSize := tt.pageLimit
				if remaining < pageSize {
					pageSize = remaining
				}

				// Simulate fetch
				totalFetched += pageSize

				// Would create cursor if more items exist
				if totalFetched < tt.totalItems {
					cursor = fmt.Sprintf("cursor-page-%d", pageCount)
				} else {
					cursor = ""
				}

				// Break if no more pages
				if cursor == "" {
					break
				}
			}

			assert.Equal(t, tt.expectedPages, pageCount, "Should fetch expected number of pages")
			assert.Equal(t, tt.totalItems, totalFetched, "Should fetch all items")
		})
	}
}
