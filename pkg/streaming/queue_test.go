package streaming

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func newStreamingMockDB(t *testing.T) (*dynamormMocks.MockDB, *dynamormMocks.MockQuery) {
	t.Helper()

	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	return db, q
}

func TestDynamoStreamQueue_QueueEvent_ValidatesTargetID(t *testing.T) {
	db, _ := newStreamingMockDB(t)
	queue := NewDynamoStreamQueue(db, "ignored", zap.NewNop()).(*dynamoStreamQueue)

	err := queue.QueueEventForUser(context.Background(), "", "type", map[string]interface{}{"a": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "userID cannot be empty")
}

func TestDynamoStreamQueue_QueueEventForTargets_CreatesDynamoRecords(t *testing.T) {
	db, q := newStreamingMockDB(t)
	queue := NewDynamoStreamQueue(db, "ignored", zap.NewNop()).(*dynamoStreamQueue)

	var got *models.StreamingEvent
	db.On("Model", mock.Anything).Return(q).Run(func(args mock.Arguments) {
		if e, ok := args.Get(0).(*models.StreamingEvent); ok {
			got = e
		}
	}).Once()

	q.On("Create").Return(nil).Once()

	err := queue.QueueEventForUser(context.Background(), "u1", "status.created", map[string]interface{}{"x": 1})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "user", got.TargetType)
	assert.Equal(t, "u1", got.TargetID)
	assert.Equal(t, "status.created", got.EventType)
	assert.True(t, strings.HasPrefix(got.PK, "STREAM_EVENT#"))
	assert.Equal(t, "EVENT", got.SK)
	assert.True(t, got.TTL > time.Now().Unix())
	assert.Equal(t, 1, got.Payload["x"])

	// Cover remaining wrapper methods.
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Create").Return(nil).Maybe()

	require.NoError(t, queue.QueueEventForStream(context.Background(), "public", "t", map[string]interface{}{}))
	require.NoError(t, queue.QueueEventForConversation(context.Background(), "c1", "t", map[string]interface{}{}))
	require.NoError(t, queue.QueueEventForFollowers(context.Background(), "u2", "t", map[string]interface{}{}))
}

func TestDynamoStreamQueue_QueueEvent_CreateError(t *testing.T) {
	db, q := newStreamingMockDB(t)
	queue := NewDynamoStreamQueue(db, "ignored", zap.NewNop()).(*dynamoStreamQueue)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Create").Return(errors.New("dynamo down")).Once()

	err := queue.QueueEventForStream(context.Background(), "public", "t", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to queue event")
}
