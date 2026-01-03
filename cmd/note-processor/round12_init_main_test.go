package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/ai"
	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/pay-theory/lift/pkg/streamer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeDB struct{}

func (f *fakeDB) Model(_ any) core.Query { return nil }

func (f *fakeDB) Transaction(fn func(tx *core.Tx) error) error {
	if fn == nil {
		return nil
	}
	tx := &core.Tx{}
	tx.SetDB(f)
	return fn(tx)
}

func (f *fakeDB) Migrate() error                    { return nil }
func (f *fakeDB) AutoMigrate(_ ...any) error        { return nil }
func (f *fakeDB) Close() error                      { return nil }
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

		newStreamerClientFn = func(_ context.Context, _ streamer.ClientConfig) (*streamer.AWSClient, error) {
			require.Fail(t, "expected websocket init to be skipped")
			return nil, nil
		}

		proc := NewNoteProcessor(&common.LambdaContext{
			Config: &config.Config{
				Domain:            "example.com",
				DynamoTableName:   "table",
				WebSocketEndpoint: "wss://example.com/ws",
			},
			Logger:     zap.NewNop(),
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

		newStreamerClientFn = func(context.Context, streamer.ClientConfig) (*streamer.AWSClient, error) {
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

		newStreamerClientFn = func(context.Context, streamer.ClientConfig) (*streamer.AWSClient, error) {
			return &streamer.AWSClient{}, nil
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

func TestHandleDynamoDBStream_RecordsDecodeError(t *testing.T) {
	origLambdaCtx := lambdaCtx
	origProcessor := processor
	t.Cleanup(func() {
		lambdaCtx = origLambdaCtx
		processor = origProcessor
	})

	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}
	processor = &NoteProcessor{logger: zap.NewNop()}

	req := lift.NewRequest(&adapters.Request{TriggerType: lift.TriggerSQS})
	ctx := lift.NewContext(context.Background(), req)

	err := handleDynamoDBStream(ctx)
	require.Error(t, err)
}

func TestHandleDynamoDBStream_SuccessAndFailure(t *testing.T) {
	origLambdaCtx := lambdaCtx
	origProcessor := processor
	t.Cleanup(func() {
		lambdaCtx = origLambdaCtx
		processor = origProcessor
	})

	lambdaCtx = &common.LambdaContext{Logger: zap.NewNop()}

	t.Run("success with empty batch", func(t *testing.T) {
		processor = &NoteProcessor{logger: zap.NewNop()}

		req := lift.NewRequest(&adapters.Request{TriggerType: lift.TriggerEventBus})
		ctx := lift.NewContext(context.Background(), req)

		require.NoError(t, handleDynamoDBStream(ctx))
	})

	t.Run("propagates partial batch failure", func(t *testing.T) {
		noteRepo := testingmocks.NewMockCommunityNoteRepository()
		noteRepo.On("GetCommunityNote", mock.Anything, "n1").Return((*storage.CommunityNote)(nil), errors.New("missing"))

		processor = &NoteProcessor{
			logger:           zap.NewNop(),
			communityNoteRepo: noteRepo,
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("NOTE#n1"),
					"SK": events.NewStringAttribute("METADATA"),
				},
			},
		}

		req := lift.NewRequest(&adapters.Request{
			TriggerType: lift.TriggerEventBus,
			Records:     []any{record},
		})
		ctx := lift.NewContext(context.Background(), req)

		err := handleDynamoDBStream(ctx)
		require.Error(t, err)
		require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeSQSProcessingFailed))
	})
}

func TestMain_InvokesLambdaStart(t *testing.T) {
	origStart := lambdaStartFn
	t.Cleanup(func() { lambdaStartFn = origStart })

	var called bool
	lambdaStartFn = func(_ interface{}) { called = true }

	main()
	require.True(t, called)
}
