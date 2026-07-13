package streaming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

func TestStreamEventLog_Enabled(t *testing.T) {
	db, _ := newStreamingMockDB(t)

	t.Run("disabled when env missing", func(t *testing.T) {
		t.Setenv("STREAM_EVENTS_TABLE_NAME", "")
		log := NewStreamEventLog(db, time.Hour)
		assert.False(t, log.Enabled())
	})

	t.Run("disabled when db missing", func(t *testing.T) {
		t.Setenv("STREAM_EVENTS_TABLE_NAME", "table")
		log := NewStreamEventLog(nil, time.Hour)
		assert.False(t, log.Enabled())
	})

	t.Run("enabled when configured", func(t *testing.T) {
		t.Setenv("STREAM_EVENTS_TABLE_NAME", "table")
		log := NewStreamEventLog(db, time.Hour)
		assert.True(t, log.Enabled())
		assert.Equal(t, "table", (streamEventLogRecord{}).TableName())
	})
}

func TestStreamEventLog_Append(t *testing.T) {
	t.Setenv("STREAM_EVENTS_TABLE_NAME", "table")

	t.Run("requires configuration", func(t *testing.T) {
		log := NewStreamEventLog(nil, time.Hour)
		_, err := log.Append(context.Background(), "public", "event", "data")
		require.Error(t, err)
	})

	t.Run("validates inputs", func(t *testing.T) {
		db, _ := newStreamingMockDB(t)
		log := NewStreamEventLog(db, time.Hour)

		_, err := log.Append(context.Background(), "", "event", "data")
		require.Error(t, err)

		_, err = log.Append(context.Background(), "public", "", "data")
		require.Error(t, err)
	})

	t.Run("creates record", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		q := new(dynamormMocks.MockQuery)

		expectedNow := time.Unix(10, 0).UTC()

		db.On("WithContext", mock.Anything).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Run(func(args mock.Arguments) {
			record, ok := args.Get(0).(*streamEventLogRecord)
			require.True(t, ok)
			assert.Equal(t, streamEventLogPKPrefix+"public", record.PK)
			assert.NotEmpty(t, record.SK)
			assert.Equal(t, "evt", record.Event)
			assert.Equal(t, "payload", record.Data)
			assert.Equal(t, expectedNow.UnixMilli(), record.CreatedAt)
			assert.Equal(t, expectedNow.Add(time.Hour).Unix(), record.TTL)
		}).Once()

		q.On("Create").Return(nil).Once()

		log := NewStreamEventLog(db, time.Hour)
		log.now = func() time.Time { return expectedNow }

		id, err := log.Append(context.Background(), "public", "evt", "payload")
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("create error", func(t *testing.T) {
		db, q := newStreamingMockDB(t)
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Create").Return(errors.New("boom")).Once()

		log := NewStreamEventLog(db, time.Hour)
		_, err := log.Append(context.Background(), "public", "evt", "payload")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "put stream event log item")
	})
}

func TestStreamEventLog_Query(t *testing.T) {
	t.Setenv("STREAM_EVENTS_TABLE_NAME", "table")

	db, q := newStreamingMockDB(t)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]streamEventLogRecord)
		*dest = []streamEventLogRecord{
			{SK: "", Event: "skip", Data: "x", CreatedAt: time.Unix(1, 0).UnixMilli()},
			{SK: "1", Event: "", Data: "skip", CreatedAt: time.Unix(1, 0).UnixMilli()},
			{SK: "2", Event: "evt", Data: "d2", CreatedAt: time.Unix(2, 0).UnixMilli()},
			{SK: "3", Event: "evt", Data: "d3", CreatedAt: 0},
		}
	}).Once()

	log := NewStreamEventLog(db, time.Hour)

	items, err := log.Query(context.Background(), "public", "", 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "2", items[0].ID)
	assert.Equal(t, "evt", items[0].Event)
	assert.Equal(t, "d2", items[0].Data)
	assert.True(t, items[0].CreatedAt.After(time.Time{}))

	assert.Equal(t, "3", items[1].ID)
	assert.True(t, items[1].CreatedAt.IsZero())
}
