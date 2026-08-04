package streaming

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/oklog/ulid/v2"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

const (
	streamEventLogPKPrefix = "STREAM#"
)

// StreamEventLogItem represents a single persisted SSE event for a given stream.
type StreamEventLogItem struct {
	ID        string
	Event     string
	Data      string
	CreatedAt time.Time
}

type streamEventLogRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	Event     string `theorydb:"attr:event"`
	Data      string `theorydb:"attr:data"`
	CreatedAt int64  `theorydb:"attr:createdAt"`
	TTL       int64  `theorydb:"ttl,attr:ttl"`
}

func (streamEventLogRecord) TableName() string {
	return config.GetStreamEventsTableName()
}

// StreamEventLog provides append/query operations for SSE stream event persistence.
type StreamEventLog struct {
	db  core.DB
	ttl time.Duration
	now func() time.Time
}

// NewStreamEventLog creates a new StreamEventLog with the provided DynamORM DB and TTL.
func NewStreamEventLog(db core.DB, ttl time.Duration) *StreamEventLog {
	return &StreamEventLog{
		db:  db,
		ttl: ttl,
		now: time.Now,
	}
}

// Enabled reports whether the StreamEventLog is configured and usable.
func (l *StreamEventLog) Enabled() bool {
	return l != nil && l.db != nil && config.GetStreamEventsTableName() != ""
}

// Append writes a new event into the stream log for the given stream.
func (l *StreamEventLog) Append(ctx context.Context, streamName, eventType, data string) (string, error) {
	if !l.Enabled() {
		return "", fmt.Errorf("stream event log not configured")
	}
	if streamName == "" {
		return "", fmt.Errorf("streamName cannot be empty")
	}
	if eventType == "" {
		return "", fmt.Errorf("eventType cannot be empty")
	}

	now := l.now().UTC()
	id, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate ulid: %w", err)
	}

	record := &streamEventLogRecord{
		PK:        streamEventLogPKPrefix + streamName,
		SK:        id.String(),
		Event:     eventType,
		Data:      data,
		CreatedAt: now.UnixMilli(),
		TTL:       now.Add(l.ttl).Unix(),
	}

	if err := l.db.WithContext(ctx).Model(record).Create(); err != nil {
		return "", fmt.Errorf("put stream event log item: %w", err)
	}

	return id.String(), nil
}

// Query returns up to limit events for the stream after afterID, ordered ascending by ID.
func (l *StreamEventLog) Query(ctx context.Context, streamName, afterID string, limit int32) ([]StreamEventLogItem, error) {
	if !l.Enabled() {
		return nil, fmt.Errorf("stream event log not configured")
	}
	if streamName == "" {
		return nil, fmt.Errorf("streamName cannot be empty")
	}
	if limit <= 0 {
		limit = 25
	}

	query := l.db.WithContext(ctx).Model(&streamEventLogRecord{}).
		Where("PK", "=", streamEventLogPKPrefix+streamName)

	if afterID != "" {
		query = query.Where("SK", ">", afterID)
	}

	var records []streamEventLogRecord
	if err := query.OrderBy("SK", "ASC").Limit(int(limit)).All(&records); err != nil {
		return nil, fmt.Errorf("query stream event log: %w", err)
	}

	items := make([]StreamEventLogItem, 0, len(records))
	for _, record := range records {
		if record.SK == "" || record.Event == "" {
			continue
		}
		createdAt := time.Time{}
		if record.CreatedAt > 0 {
			createdAt = time.UnixMilli(record.CreatedAt).UTC()
		}

		items = append(items, StreamEventLogItem{
			ID:        record.SK,
			Event:     record.Event,
			Data:      record.Data,
			CreatedAt: createdAt,
		})
	}

	return items, nil
}
