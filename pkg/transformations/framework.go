package transformations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
)

// Transformer interface for data transformations
type Transformer[TSource, TTarget any] interface {
	Transform(ctx context.Context, source TSource) (TTarget, error)
	TransformList(ctx context.Context, sources []TSource) ([]TTarget, error)
}

// BaseTransformer provides common transformation functionality
type BaseTransformer[TSource, TTarget any] struct {
	TransformFunc func(context.Context, TSource) (TTarget, error)
}

// Transform performs a single transformation
func (bt *BaseTransformer[TSource, TTarget]) Transform(ctx context.Context, source TSource) (TTarget, error) {
	if bt.TransformFunc == nil {
		var zero TTarget
		return zero, errors.TransformFunctionNotSet()
	}
	return bt.TransformFunc(ctx, source)
}

// TransformList transforms a slice of source items
func (bt *BaseTransformer[TSource, TTarget]) TransformList(ctx context.Context, sources []TSource) ([]TTarget, error) {
	if err := common.ValidateSliceNotEmpty("sources", sources); err != nil {
		return []TTarget{}, nil
	}

	results := make([]TTarget, 0, len(sources))
	for _, source := range sources {
		transformed, err := bt.Transform(ctx, source)
		if err != nil {
			return nil, errors.TransformItemFailed(err)
		}
		results = append(results, transformed)
	}
	return results, nil
}

// APIResponseTransformer handles Mastodon API response transformations
type APIResponseTransformer[T any] struct {
	*BaseTransformer[T, models.Account]
	baseURL string
}

// NewAPIResponseTransformer creates a new API response transformer
func NewAPIResponseTransformer[T any](baseURL string, transformFunc func(context.Context, T) (models.Account, error)) *APIResponseTransformer[T] {
	return &APIResponseTransformer[T]{
		BaseTransformer: &BaseTransformer[T, models.Account]{
			TransformFunc: transformFunc,
		},
		baseURL: baseURL,
	}
}

// ActivityPubTransformer handles ActivityPub protocol transformations
type ActivityPubTransformer[T any] struct {
	*BaseTransformer[T, *activitypub.Actor]
}

// NewActivityPubTransformer creates a new ActivityPub transformer
func NewActivityPubTransformer[T any](transformFunc func(context.Context, T) (*activitypub.Actor, error)) *ActivityPubTransformer[T] {
	return &ActivityPubTransformer[T]{
		BaseTransformer: &BaseTransformer[T, *activitypub.Actor]{
			TransformFunc: transformFunc,
		},
	}
}

// StorageModelTransformer handles storage model transformations
type StorageModelTransformer[TSource, TTarget any] struct {
	*BaseTransformer[TSource, TTarget]
}

// NewStorageModelTransformer creates a new storage model transformer
func NewStorageModelTransformer[TSource, TTarget any](transformFunc func(context.Context, TSource) (TTarget, error)) *StorageModelTransformer[TSource, TTarget] {
	return &StorageModelTransformer[TSource, TTarget]{
		BaseTransformer: &BaseTransformer[TSource, TTarget]{
			TransformFunc: transformFunc,
		},
	}
}

// StatusResponseTransformer handles Mastodon Status API response transformations
type StatusResponseTransformer[T any] struct {
	*BaseTransformer[T, models.Status]
	baseURL string
}

// NewStatusResponseTransformer creates a new Status response transformer
func NewStatusResponseTransformer[T any](baseURL string, transformFunc func(context.Context, T) (models.Status, error)) *StatusResponseTransformer[T] {
	return &StatusResponseTransformer[T]{
		BaseTransformer: &BaseTransformer[T, models.Status]{
			TransformFunc: transformFunc,
		},
		baseURL: baseURL,
	}
}

// GraphQLTransformer handles GraphQL schema transformations
type GraphQLTransformer[T any] struct {
	*BaseTransformer[T, map[string]interface{}]
}

// NewGraphQLTransformer creates a new GraphQL transformer
func NewGraphQLTransformer[T any](transformFunc func(context.Context, T) (map[string]interface{}, error)) *GraphQLTransformer[T] {
	return &GraphQLTransformer[T]{
		BaseTransformer: &BaseTransformer[T, map[string]interface{}]{
			TransformFunc: transformFunc,
		},
	}
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	MaxID   string `json:"max_id,omitempty"`
	MinID   string `json:"min_id,omitempty"`
	SinceID string `json:"since_id,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

// Utility transformation functions

// TransformTimestamp converts a timestamp to a specific format
func TransformTimestamp(timestamp time.Time, format string) string {
	if timestamp.IsZero() {
		return ""
	}
	if err := common.ValidateRequiredParam("format", format); err != nil {
		format = time.RFC3339
	}
	return timestamp.Format(format)
}

// TransformIDList adds a prefix to a list of IDs
func TransformIDList(ids []string, prefix string) []string {
	if err := common.ValidateSliceNotEmpty("ids", ids); err != nil {
		return []string{}
	}

	result := make([]string, len(ids))
	for i, id := range ids {
		if prefix != "" && !strings.HasPrefix(id, prefix) {
			result[i] = prefix + id
		} else {
			result[i] = id
		}
	}
	return result
}

// TransformPaginationInfo converts pagination info to map format
func TransformPaginationInfo(info PaginationInfo) map[string]interface{} {
	result := make(map[string]interface{})

	if info.MaxID != "" {
		result["max_id"] = info.MaxID
	}
	if info.MinID != "" {
		result["min_id"] = info.MinID
	}
	if info.SinceID != "" {
		result["since_id"] = info.SinceID
	}
	if info.Limit > 0 {
		result["limit"] = info.Limit
	}
	if info.Offset > 0 {
		result["offset"] = info.Offset
	}

	return result
}

// TransformErrorResponse converts an error to a standardized response format
func TransformErrorResponse(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{
			"error": "unknown error",
		}
	}

	return map[string]interface{}{
		"error":      err.Error(),
		"error_type": fmt.Sprintf("%T", err),
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}
