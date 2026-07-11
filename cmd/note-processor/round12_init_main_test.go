package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/ai"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
)

type fakeDB struct{}

func (f *fakeDB) Model(_ any) core.Query { return nil }

func (f *fakeDB) Migrate() error                        { return nil }
func (f *fakeDB) AutoMigrate(_ ...any) error            { return nil }
func (f *fakeDB) Close() error                          { return nil }
func (f *fakeDB) WithContext(_ context.Context) core.DB { return f }

func TestNewNoteProcessor_InitializationBranches(t *testing.T) {
	origGetClient := dynamormGetClientFn
	origBedrockClient := newBedrockClientFn
	origStreamerClient := newStreamerClientFn
	t.Cleanup(func() {
		dynamormGetClientFn = origGetClient
		newBedrockClientFn = origBedrockClient
		newStreamerClientFn = origStreamerClient
	})

	dynamormGetClientFn = func(context.Context) (core.DB, error) { return &fakeDB{}, nil }

	t.Run("skips websocket init without aws services", func(t *testing.T) {
		newBedrockClientFn = func(context.Context, *zap.Logger) (*ai.BedrockClient, error) {
			return &ai.BedrockClient{}, nil
		}

		newStreamerClientFn = func(_ context.Context, _ string, _ ...streamer.Option) (streamer.Client, error) {
			require.Fail(t, "expected websocket init to be skipped")
			return nil, nil
		}

		proc := NewNoteProcessor(&common.LambdaContext{
			Config: &config.Config{
				Domain:            "example.com",
				DynamoTableName:   "table",
				WebSocketEndpoint: "wss://example.com/ws",
			},
			Logger:      zap.NewNop(),
			AWSServices: nil,
		})

		require.NotNil(t, proc)
		require.Equal(t, "wss://example.com/ws", proc.wsEndpoint)
		require.Nil(t, proc.wsClient)
		require.NotEmpty(t, proc.baseURL)
	})

	t.Run("warns on bedrock and websocket init errors", func(t *testing.T) {
		newBedrockClientFn = func(context.Context, *zap.Logger) (*ai.BedrockClient, error) {
			return nil, errors.New("bedrock init failed")
		}

		newStreamerClientFn = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
			return nil, errors.New("streamer init failed")
		}

		proc := NewNoteProcessor(&common.LambdaContext{
			Config: &config.Config{
				Domain:            "example.com",
				DynamoTableName:   "table",
				WebSocketEndpoint: "wss://example.com/ws",
			},
			Logger: zap.NewNop(),
			AWSServices: &awsInit.AWSServices{
				Config: aws.Config{},
			},
		})

		require.NotNil(t, proc)
		require.Nil(t, proc.bedrockClient)
		require.Nil(t, proc.wsClient)
		require.Equal(t, "wss://example.com/ws", proc.wsEndpoint)
	})

	t.Run("initializes websocket client when configured", func(t *testing.T) {
		newBedrockClientFn = func(context.Context, *zap.Logger) (*ai.BedrockClient, error) {
			return &ai.BedrockClient{}, nil
		}

		newStreamerClientFn = func(context.Context, string, ...streamer.Option) (streamer.Client, error) {
			return &fakeStreamerClient{}, nil
		}

		proc := NewNoteProcessor(&common.LambdaContext{
			Config: &config.Config{
				Domain:            "example.com",
				DynamoTableName:   "table",
				WebSocketEndpoint: "wss://example.com/ws",
			},
			Logger: zap.NewNop(),
			AWSServices: &awsInit.AWSServices{
				Config: aws.Config{},
			},
		})

		require.NotNil(t, proc)
		require.NotNil(t, proc.wsClient)
	})
}

func TestMain_InvokesLambdaStart(t *testing.T) {
	origStart := lambdaStartFn
	origLambdaCtx := lambdaCtx
	origProcessor := processor
	t.Cleanup(func() { lambdaStartFn = origStart })
	t.Cleanup(func() { lambdaCtx = origLambdaCtx })
	t.Cleanup(func() { processor = origProcessor })

	lambdaCtx = &common.LambdaContext{Config: &config.Config{DynamoTableName: "test-table"}}

	noteRepo := testingmocks.NewMockCommunityNoteRepository()
	noteRepo.On("GetCommunityNote", mock.Anything, "n1").Return(nil, errors.New("missing"))
	processor = &NoteProcessor{
		logger:            zap.NewNop(),
		communityNoteRepo: noteRepo,
	}

	called := false
	lambdaStartFn = func(h any) {
		called = true
		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "evt-1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000",
				Change: events.DynamoDBStreamRecord{
					SequenceNumber: "evt-1",
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("NOTE#n1"),
						"SK": events.NewStringAttribute("METADATA"),
					},
				},
			},
		}}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Len(t, resp.BatchItemFailures, 1)
		require.Equal(t, "evt-1", resp.BatchItemFailures[0].ItemIdentifier)
	}

	main()
	require.True(t, called)
}
