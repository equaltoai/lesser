package cost

import (
	"context"
)

// contextKey is the type for context keys
type contextKey string

// trackerKey is the context key for the cost tracker
const trackerKey contextKey = "cost_tracker"

// WithTracker attaches a cost tracker to the context
func WithTracker(ctx context.Context, tracker *Tracker) context.Context {
	return context.WithValue(ctx, trackerKey, tracker)
}

// FromContext retrieves the cost tracker from the context
// Returns nil if no tracker is found
func FromContext(ctx context.Context) *Tracker {
	if tracker, ok := ctx.Value(trackerKey).(*Tracker); ok {
		return tracker
	}
	return nil
}

// Track is a convenience function that tracks an operation cost if a tracker exists in the context
// It's safe to call even if no tracker is present
func Track(ctx context.Context, fn func(*Tracker)) {
	if tracker := FromContext(ctx); tracker != nil {
		fn(tracker)
	}
}

// TrackDynamoReadContext tracks DynamoDB reads from context
func TrackDynamoReadContext(ctx context.Context, items int) {
	Track(ctx, func(t *Tracker) {
		t.TrackDynamoRead(items)
	})
}

// TrackDynamoWriteContext tracks DynamoDB writes from context
func TrackDynamoWriteContext(ctx context.Context, items int) {
	Track(ctx, func(t *Tracker) {
		t.TrackDynamoWrite(items)
	})
}

// TrackS3GetContext tracks S3 GET operations from context
func TrackS3GetContext(ctx context.Context, count int) {
	Track(ctx, func(t *Tracker) {
		t.TrackS3Get(count)
	})
}

// TrackS3PutContext tracks S3 PUT operations from context
func TrackS3PutContext(ctx context.Context, count int) {
	Track(ctx, func(t *Tracker) {
		t.TrackS3Put(count)
	})
}

// TrackDataTransferContext tracks data transfer from context
func TrackDataTransferContext(ctx context.Context, bytes int64) {
	Track(ctx, func(t *Tracker) {
		t.TrackDataTransfer(bytes)
	})
}

// TrackOpenSearchQueryContext tracks OpenSearch queries from context
func TrackOpenSearchQueryContext(ctx context.Context, count int) {
	Track(ctx, func(t *Tracker) {
		t.TrackOpenSearchQuery(count)
	})
}

// TrackOpenSearchIndexContext tracks OpenSearch indexing from context
func TrackOpenSearchIndexContext(ctx context.Context, count int) {
	Track(ctx, func(t *Tracker) {
		t.TrackOpenSearchIndex(count)
	})
}
