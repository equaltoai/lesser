package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	dynamock "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityProcessor_StreamProcessingPaths(t *testing.T) {
	ctx := context.Background()

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	ap := &ActivityProcessor{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	// INSERT -> inbox direction
	err := ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-inbox",
		EventName: activityInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#1"),
				"SK":        events.NewStringAttribute("SK#1"),
				"direction": events.NewStringAttribute("inbox"),
				"username":  events.NewStringAttribute("alice"),
				"actor_id":  events.NewStringAttribute("https://remote.example/users/bob"),
				"type":      events.NewStringAttribute("Create"),
				"activity":  events.NewStringAttribute(`{}`),
			},
		},
	})
	require.NoError(t, err)

	// INSERT -> outbox direction (non-fanout type still writes outbox record)
	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-outbox",
		EventName: activityInsert,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#2"),
				"SK":        events.NewStringAttribute("SK#2"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"actor_id":  events.NewStringAttribute("https://example.com/users/alice"),
				"type":      events.NewStringAttribute("Like"),
				"activity":  events.NewStringAttribute(`{"id":"act-1","type":"Like","actor":"https://example.com/users/alice","object":"https://example.com/objects/1"}`),
			},
		},
	})
	require.NoError(t, err)

	// MODIFY -> metrics record
	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-modify",
		EventName: activityModify,
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#3"),
				"SK":        events.NewStringAttribute("SK#3"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"type":      events.NewStringAttribute("Update"),
				"activity":  events.NewStringAttribute(`{}`),
			},
		},
	})
	require.NoError(t, err)

	// REMOVE -> cleanup record (invalid JSON falls back to cleanup)
	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-remove",
		EventName: activityRemove,
		Change: events.DynamoDBStreamRecord{
			OldImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("ACTIVITY#4"),
				"SK":        events.NewStringAttribute("SK#4"),
				"direction": events.NewStringAttribute("outbox"),
				"username":  events.NewStringAttribute("alice"),
				"type":      events.NewStringAttribute("Delete"),
				"activity":  events.NewStringAttribute("{"),
			},
		},
	})
	require.NoError(t, err)

	// Unknown event types should be ignored.
	err = ap.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-unknown",
		EventName: "SOMETHING_ELSE",
	})
	require.NoError(t, err)

	// HandleStream should report retryable errors when record processing fails.
	err = ap.HandleStream(ctx, events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{EventID: "evt-missing-image", EventName: activityInsert},
		},
	})
	require.Error(t, err)

	mockQuery.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

