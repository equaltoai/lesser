package graph

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// PaginationOptions represents GraphQL pagination parameters
type PaginationOptions struct {
	First  *int          `json:"first,omitempty"`
	After  *model.Cursor `json:"after,omitempty"`
	Last   *int          `json:"last,omitempty"`
	Before *model.Cursor `json:"before,omitempty"`
}

// CursorData represents cursor data for GraphQL pagination
type CursorData struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Score     float64   `json:"score,omitempty"`
}

// ParsePaginationArgs parses and validates GraphQL pagination arguments
func ParsePaginationArgs(first *int, after *model.Cursor, last *int, before *model.Cursor) (*PaginationOptions, error) {
	opts := &PaginationOptions{
		First:  first,
		After:  after,
		Last:   last,
		Before: before,
	}

	// Validate that first/after or last/before are used together (not mixed)
	if (first != nil || after != nil) && (last != nil || before != nil) {
		return nil, ErrPaginationMixedParams
	}

	// Set defaults and limits for forward pagination
	if first != nil {
		if *first <= 0 {
			return nil, ErrFirstMustBePositive
		}
		if *first > 100 {
			*first = 100 // Enforce maximum
		}
	} else if last == nil && first == nil {
		// Default to forward pagination with 20 items
		defaultFirst := 20
		opts.First = &defaultFirst
	}

	// Set defaults and limits for backward pagination
	if last != nil {
		if *last <= 0 {
			return nil, ErrLastMustBePositive
		}
		if *last > 100 {
			*last = 100 // Enforce maximum
		}
	}

	return opts, nil
}

// EncodeGraphQLCursor creates an opaque cursor from cursor data
func EncodeGraphQLCursor(data *CursorData) model.Cursor {
	if data == nil {
		return ""
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	return model.Cursor(base64.URLEncoding.EncodeToString(jsonData))
}

// DecodeGraphQLCursor parses a cursor back to cursor data
func DecodeGraphQLCursor(cursor model.Cursor) (*CursorData, error) {
	if err := common.ValidateRequiredParam("cursor", string(cursor)); err != nil {
		return nil, nil
	}

	jsonData, err := base64.URLEncoding.DecodeString(string(cursor))
	if err != nil {
		return nil, ErrInvalidCursorFormatWithContext(err)
	}

	var data CursorData
	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, ErrInvalidCursorDataWithContext(err)
	}

	return &data, nil
}

// BuildPageInfo creates PageInfo for GraphQL connections
func BuildPageInfo(
	edges []interface{},
	hasPreviousPage bool,
	hasNextPage bool,
	getCursor func(interface{}) model.Cursor,
) *model.PageInfo {
	pageInfo := &model.PageInfo{
		HasPreviousPage: hasPreviousPage,
		HasNextPage:     hasNextPage,
	}

	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor := getCursor(edges[0])
		endCursor := getCursor(edges[len(edges)-1])
		pageInfo.StartCursor = &startCursor
		pageInfo.EndCursor = &endCursor
	}

	return pageInfo
}

// ConvertToDynamORMPagination converts GraphQL pagination to DynamORM pagination
func ConvertToDynamORMPagination(opts *PaginationOptions) (*repositories.PaginationOptions, error) {
	dynamormOpts := repositories.NewPaginationOptions()

	// Handle forward pagination (first/after)
	if opts.First != nil {
		dynamormOpts.Limit = *opts.First + 1 // Request one extra to determine hasNextPage

		if opts.After != nil {
			cursorData, err := DecodeGraphQLCursor(*opts.After)
			if err != nil {
				return nil, ErrInvalidAfterCursorWithContext(err)
			}

			// Convert to DynamORM cursor format
			if cursorData != nil {
				dynamormOpts.Cursor = repositories.EncodeCursor(&repositories.CursorData{
					LastID:        cursorData.ID,
					LastTimestamp: cursorData.Timestamp,
					LastScore:     cursorData.Score,
				})
			}
		}
	}

	// Handle backward pagination (last/before)
	if opts.Last != nil {
		dynamormOpts.Limit = *opts.Last + 1                     // Request one extra to determine hasPreviousPage
		dynamormOpts.SortOrder = repositories.SearchSortTimeAsc // Reverse for backward pagination

		if opts.Before != nil {
			cursorData, err := DecodeGraphQLCursor(*opts.Before)
			if err != nil {
				return nil, ErrInvalidBeforeCursorWithContext(err)
			}

			// Convert to DynamORM cursor format
			if cursorData != nil {
				dynamormOpts.Cursor = repositories.EncodeCursor(&repositories.CursorData{
					LastID:        cursorData.ID,
					LastTimestamp: cursorData.Timestamp,
					LastScore:     cursorData.Score,
				})
			}
		}
	}

	return dynamormOpts, nil
}

// ApplyPaginationToResults applies pagination logic to results and returns proper hasNextPage/hasPreviousPage
func ApplyPaginationToResults[T any](
	results []T,
	opts *PaginationOptions,
	getID func(T) string, //nolint:revive // unused but required for consistent API
	getTimestamp func(T) time.Time, //nolint:revive // unused but required for consistent API
	getScore func(T) float64, //nolint:revive // unused but required for consistent API
) ([]T, bool, bool, error) {

	hasNextPage := false
	hasPreviousPage := false

	// Handle forward pagination (first/after)
	if opts.First != nil {
		requestedLimit := *opts.First

		// Check if we have more results than requested
		if err := common.ValidateIntRange("results_length", len(results), requestedLimit+1, 1000); err == nil {
			hasNextPage = true
			results = results[:requestedLimit] // Take only requested amount
		}

		// If we used after cursor, we have a previous page
		if opts.After != nil && *opts.After != "" {
			hasPreviousPage = true
		}

		return results, hasPreviousPage, hasNextPage, nil
	}

	// Handle backward pagination (last/before)
	if opts.Last != nil {
		requestedLimit := *opts.Last

		// Check if we have more results than requested
		if err := common.ValidateIntRange("results_length", len(results), requestedLimit+1, 1000); err == nil {
			hasPreviousPage = true
			results = results[len(results)-requestedLimit:] // Take last N items
		}

		// If we used before cursor, we have a next page
		if opts.Before != nil && *opts.Before != "" {
			hasNextPage = true
		}

		// Reverse results for backward pagination since we queried in reverse
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}

		return results, hasPreviousPage, hasNextPage, nil
	}

	// Default case (shouldn't happen with validation)
	return results, false, false, nil
}

// CreateObjectEdges creates ObjectEdge array from objects
func CreateObjectEdges[T any](
	items []T,
	convertToObject func(T) (*model.Object, error),
	getID func(T) string,
	getTimestamp func(T) time.Time,
	getScore func(T) float64,
) ([]*model.ObjectEdge, error) {

	edges := make([]*model.ObjectEdge, len(items))

	for i, item := range items {
		object, err := convertToObject(item)
		if err != nil {
			return nil, ErrFailedToConvertItem(i, err)
		}

		cursor := EncodeGraphQLCursor(&CursorData{
			ID:        getID(item),
			Timestamp: getTimestamp(item),
			Score:     getScore(item),
		})

		edges[i] = &model.ObjectEdge{
			Node:   object,
			Cursor: cursor,
		}
	}

	return edges, nil
}

// CreateNotificationEdges creates NotificationEdge array from notifications
func CreateNotificationEdges[T any](
	items []T,
	convertToNotification func(T) (*model.Notification, error),
	getID func(T) string,
	getTimestamp func(T) time.Time,
) ([]*model.NotificationEdge, error) {

	edges := make([]*model.NotificationEdge, len(items))

	for i, item := range items {
		notification, err := convertToNotification(item)
		if err != nil {
			return nil, ErrFailedToConvertNotification(i, err)
		}

		cursor := EncodeGraphQLCursor(&CursorData{
			ID:        getID(item),
			Timestamp: getTimestamp(item),
		})

		edges[i] = &model.NotificationEdge{
			Node:   notification,
			Cursor: cursor,
		}
	}

	return edges, nil
}

// CreateHashtagEdges creates HashtagEdge array from hashtags
func CreateHashtagEdges[T any](
	items []T,
	convertToHashtag func(T) (*model.Hashtag, error),
	getID func(T) string,
	getTimestamp func(T) time.Time,
) ([]*model.HashtagEdge, error) {

	edges := make([]*model.HashtagEdge, len(items))

	for i, item := range items {
		hashtag, err := convertToHashtag(item)
		if err != nil {
			return nil, ErrFailedToConvertHashtag(i, err)
		}

		cursor := EncodeGraphQLCursor(&CursorData{
			ID:        getID(item),
			Timestamp: getTimestamp(item),
		})

		edges[i] = &model.HashtagEdge{
			Node:   hashtag,
			Cursor: cursor,
		}
	}

	return edges, nil
}

// CreateSeveredRelationshipEdges creates SeveredRelationshipEdge array
func CreateSeveredRelationshipEdges[T any](
	items []T,
	convertToSeveredRelationship func(T) (*model.SeveredRelationship, error),
	getID func(T) string,
	getTimestamp func(T) time.Time,
) ([]*model.SeveredRelationshipEdge, error) {

	edges := make([]*model.SeveredRelationshipEdge, len(items))

	for i, item := range items {
		relationship, err := convertToSeveredRelationship(item)
		if err != nil {
			return nil, ErrFailedToConvertSeveredRelationship(i, err)
		}

		cursor := EncodeGraphQLCursor(&CursorData{
			ID:        getID(item),
			Timestamp: getTimestamp(item),
		})

		edges[i] = &model.SeveredRelationshipEdge{
			Node:   relationship,
			Cursor: cursor,
		}
	}

	return edges, nil
}

// CreateAffectedRelationshipEdges creates AffectedRelationshipEdge array
func CreateAffectedRelationshipEdges[T any](
	items []T,
	convertToAffectedRelationship func(T) (*model.AffectedRelationship, error),
	getID func(T) string,
	getTimestamp func(T) time.Time,
) ([]*model.AffectedRelationshipEdge, error) {

	edges := make([]*model.AffectedRelationshipEdge, len(items))

	for i, item := range items {
		relationship, err := convertToAffectedRelationship(item)
		if err != nil {
			return nil, ErrFailedToConvertAffectedRelationship(i, err)
		}

		cursor := EncodeGraphQLCursor(&CursorData{
			ID:        getID(item),
			Timestamp: getTimestamp(item),
		})

		edges[i] = &model.AffectedRelationshipEdge{
			Node:   relationship,
			Cursor: cursor,
		}
	}

	return edges, nil
}
