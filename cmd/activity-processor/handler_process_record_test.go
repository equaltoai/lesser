package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestActivityHandler_ProcessRecord_Branches(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()

	ctx := context.Background()

	handler := &ActivityHandler{
		Logger: zap.NewNop(),
	}

	// Missing PK -> entity type extraction fails.
	require.Error(t, handler.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-missing-pk",
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
	}))

	// Non-activity entity types are ignored.
	require.NoError(t, handler.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-non-activity",
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK": events.NewStringAttribute("user#1"),
		}},
	}))

	// Full activity record should parse and route.
	require.NoError(t, handler.processRecord(ctx, events.DynamoDBEventRecord{
		EventID:   "evt-activity",
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
			"PK":       events.NewStringAttribute("activity#1"),
			"SK":       events.NewStringAttribute("outbox#1"),
			"Username": events.NewStringAttribute("alice"),
			"Activity": events.NewStringAttribute(`{"id":"https://example.com/activities/1","type":"Update","actor":"https://example.com/users/alice","object":"https://example.com/objects/1","to":["https://remote.example/users/bob"]}`),
		}},
	}))

	// NewActivityHandler should be able to construct with a DB dependency.
	require.NotNil(t, NewActivityHandler(new(mocks.MockDB), "test-table"))
}
