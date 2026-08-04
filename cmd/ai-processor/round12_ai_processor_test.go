package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	aiService "github.com/equaltoai/lesser/pkg/services/ai"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeAnalyzer struct {
	analysis   *ai.AIAnalysis
	err        error
	calls      int
	lastObject *ai.Content
}

func (f *fakeAnalyzer) AnalyzeContent(_ context.Context, content *ai.Content) (*ai.AIAnalysis, error) {
	f.calls++
	f.lastObject = content
	if f.err != nil {
		return nil, f.err
	}
	return f.analysis, nil
}

type fakeSaver struct {
	result   *aiService.SaveAnalysisResult
	err      error
	calls    int
	lastCmd  *aiService.SaveAnalysisCommand
	lastUser string
}

func (f *fakeSaver) SaveAnalysis(_ context.Context, cmd *aiService.SaveAnalysisCommand) (*aiService.SaveAnalysisResult, error) {
	f.calls++
	f.lastCmd = cmd
	if cmd != nil {
		f.lastUser = cmd.UserID
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &aiService.SaveAnalysisResult{Success: true}, nil
}

func TestAIProcessor_Helpers_Round12(t *testing.T) {
	ap := &AIProcessor{}
	require.True(t, ap.isAnalyzableType("Note"))
	require.False(t, ap.isAnalyzableType("Audio"))

	require.Equal(t, "spam", ap.determineCategory(&ai.AIAnalysis{SpamAnalysis: &ai.SpamAnalysis{SpamScore: 0.8}}))
	require.Equal(t, "hate_speech", ap.determineCategory(&ai.AIAnalysis{TextAnalysis: &ai.TextAnalysis{ToxicityScore: 0.8}}))
	require.Equal(t, "nsfw", ap.determineCategory(&ai.AIAnalysis{ImageAnalysis: &ai.ImageAnalysis{IsNSFW: true}}))
	require.Equal(t, "violence", ap.determineCategory(&ai.AIAnalysis{ImageAnalysis: &ai.ImageAnalysis{ViolenceScore: 0.8}}))
	require.Equal(t, "other", ap.determineCategory(&ai.AIAnalysis{}))

	require.Equal(t, "critical", ap.determineSeverity(&ai.AIAnalysis{OverallRisk: 0.95}))
	require.Equal(t, "high", ap.determineSeverity(&ai.AIAnalysis{OverallRisk: 0.75}))
	require.Equal(t, "medium", ap.determineSeverity(&ai.AIAnalysis{OverallRisk: 0.55}))
	require.Equal(t, "low", ap.determineSeverity(&ai.AIAnalysis{OverallRisk: 0.1}))
}

func TestAIProcessor_UnmarshalAndExtractContent_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	ap := &AIProcessor{logger: zap.NewNop()}
	record := events.DynamoDBEventRecord{Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}}}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		item := target.(*contentStreamItem)
		item.PK = "OBJECT#123"
		item.Type = "Note"
		item.Content = "hello"
		item.ActorID = "alice"
		item.Attachment = []contentStreamAttachment{
			{Type: "Image", URL: "https://example.com/img.png"},
			{Type: "Audio", URL: "https://example.com/audio.mp3"},
		}
		return nil
	}

	content, err := ap.extractContent(record)
	require.NoError(t, err)
	require.Equal(t, "123", content.ID)
	require.Equal(t, "Note", content.Type)
	require.Equal(t, "alice", content.AuthorID)
	require.Equal(t, []string{"https://example.com/img.png"}, content.MediaURLs)

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		item := target.(*contentStreamItem)
		item.PK = "BAD#123"
		item.Type = "Note"
		return nil
	}
	_, err = ap.extractContent(record)
	require.Error(t, err)

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		item := target.(*contentStreamItem)
		item.PK = "OBJECT#123"
		item.Type = "Audio"
		return nil
	}
	_, err = ap.extractContent(record)
	require.Error(t, err)

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error {
		return errors.New("boom")
	}
	_, err = ap.extractContent(record)
	require.Error(t, err)
}

func TestAIProcessor_IsAnalyzableRecord_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	ap := &AIProcessor{}
	require.False(t, ap.isAnalyzableRecord(events.DynamoDBEventRecord{}))

	record := events.DynamoDBEventRecord{Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}}}
	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return errors.New("boom") }
	require.False(t, ap.isAnalyzableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		item := target.(*analyzableStreamItem)
		item.PK = "OBJECT#1"
		item.Type = "Note"
		return nil
	}
	require.True(t, ap.isAnalyzableRecord(record))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		item := target.(*analyzableStreamItem)
		item.PK = "NOTOBJECT#1"
		item.Type = "Note"
		return nil
	}
	require.False(t, ap.isAnalyzableRecord(record))
}

func TestAIProcessor_ProcessRecord_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	analysis := &ai.AIAnalysis{
		ID:               "analysis-1",
		ObjectID:         "123",
		ObjectType:       "Note",
		ModerationAction: ai.ActionFlag,
		OverallRisk:      0.8,
		SpamAnalysis:     &ai.SpamAnalysis{SpamScore: 0.8},
		AnalyzedAt:       time.Now().UTC(),
		Version:          "1.0",
	}

	analyzer := &fakeAnalyzer{analysis: analysis}
	saver := &fakeSaver{}

	ap := &AIProcessor{
		db:         mockDB,
		aiAnalyzer: analyzer,
		aiService:  saver,
		logger:     zap.NewNop(),
	}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		switch v := target.(type) {
		case *analyzableStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			return nil
		case *contentStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			v.Content = "hello"
			v.ActorID = "alice"
			v.Attachment = []contentStreamAttachment{{Type: "Image", URL: "https://example.com/img.png"}}
			return nil
		default:
			return errors.New("unexpected target")
		}
	}

	requestID := "req"

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
	}

	require.NoError(t, ap.processRecord(context.Background(), requestID, record))
	require.Equal(t, 1, analyzer.calls)
	require.Equal(t, 1, saver.calls)
	require.Equal(t, "alice", saver.lastUser)
	require.NotNil(t, saver.lastCmd.Analysis)

	// Non-write event is ignored.
	require.NoError(t, ap.processRecord(context.Background(), requestID, events.DynamoDBEventRecord{EventName: "REMOVE"}))
}

func TestAIProcessor_ProcessRecord_ErrorBranches_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	requestID := "req"

	t.Run("skips non-analyzable", func(t *testing.T) {
		analyzer := &fakeAnalyzer{analysis: &ai.AIAnalysis{}}
		saver := &fakeSaver{}

		ap := &AIProcessor{
			aiAnalyzer: analyzer,
			aiService:  saver,
			logger:     zap.NewNop(),
		}

		record := events.DynamoDBEventRecord{EventName: "INSERT"}
		require.NoError(t, ap.processRecord(context.Background(), requestID, record))
		require.Equal(t, 0, analyzer.calls)
		require.Equal(t, 0, saver.calls)
	})

	t.Run("extract content error returns wrapped error", func(t *testing.T) {
		analyzer := &fakeAnalyzer{analysis: &ai.AIAnalysis{}}
		saver := &fakeSaver{}

		ap := &AIProcessor{
			aiAnalyzer: analyzer,
			aiService:  saver,
			logger:     zap.NewNop(),
		}

		unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
			switch v := target.(type) {
			case *analyzableStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				return nil
			case *contentStreamItem:
				return errors.New("boom")
			default:
				return errors.New("unexpected target")
			}
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
		}
		require.Error(t, ap.processRecord(context.Background(), requestID, record))
		require.Equal(t, 0, analyzer.calls)
		require.Equal(t, 0, saver.calls)
	})

	t.Run("analyzer error returns wrapped error", func(t *testing.T) {
		analyzer := &fakeAnalyzer{err: errors.New("boom")}
		saver := &fakeSaver{}

		ap := &AIProcessor{
			aiAnalyzer: analyzer,
			aiService:  saver,
			logger:     zap.NewNop(),
		}

		unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
			switch v := target.(type) {
			case *analyzableStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				return nil
			case *contentStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				return nil
			default:
				return errors.New("unexpected target")
			}
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
		}
		require.Error(t, ap.processRecord(context.Background(), requestID, record))
		require.Equal(t, 1, analyzer.calls)
		require.Equal(t, 0, saver.calls)
	})

	t.Run("save analysis error returns wrapped error", func(t *testing.T) {
		analyzer := &fakeAnalyzer{analysis: &ai.AIAnalysis{ModerationAction: ai.ActionNone}}
		saver := &fakeSaver{err: errors.New("boom")}

		ap := &AIProcessor{
			aiAnalyzer: analyzer,
			aiService:  saver,
			logger:     zap.NewNop(),
		}

		unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
			switch v := target.(type) {
			case *analyzableStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				return nil
			case *contentStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				v.ActorID = "alice"
				return nil
			default:
				return errors.New("unexpected target")
			}
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
		}
		require.Error(t, ap.processRecord(context.Background(), requestID, record))
		require.Equal(t, 1, analyzer.calls)
		require.Equal(t, 1, saver.calls)
	})

	t.Run("moderation create error is logged but does not fail batch", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(errors.New("boom"))

		analyzer := &fakeAnalyzer{analysis: &ai.AIAnalysis{
			ID:               "analysis-1",
			ObjectID:         "123",
			ObjectType:       "Note",
			ModerationAction: ai.ActionFlag,
			OverallRisk:      0.8,
			SpamAnalysis:     &ai.SpamAnalysis{SpamScore: 0.8},
			AnalyzedAt:       time.Now().UTC(),
			Version:          "1.0",
		}}
		saver := &fakeSaver{}

		ap := &AIProcessor{
			db:         mockDB,
			aiAnalyzer: analyzer,
			aiService:  saver,
			logger:     zap.NewNop(),
		}

		unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
			switch v := target.(type) {
			case *analyzableStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				return nil
			case *contentStreamItem:
				v.PK = "OBJECT#123"
				v.Type = "Note"
				v.ActorID = "alice"
				return nil
			default:
				return errors.New("unexpected target")
			}
		}

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
		}

		require.NoError(t, ap.processRecord(context.Background(), requestID, record))
		require.Equal(t, 1, analyzer.calls)
		require.Equal(t, 1, saver.calls)
	})
}

func TestAIProcessor_HandleDynamoDBRecord_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	ap := &AIProcessor{
		aiAnalyzer: &fakeAnalyzer{err: errors.New("boom")},
		aiService:  &fakeSaver{},
		logger:     zap.NewNop(),
	}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		switch v := target.(type) {
		case *analyzableStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			return nil
		case *contentStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			v.Content = "hello"
			v.ActorID = "alice"
			return nil
		default:
			return errors.New("unexpected target")
		}
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("OBJECT#123")}},
	}

	require.NoError(t, ap.HandleDynamoDBRecord(nil, record))
}

func TestAIProcessor_HandleDynamoDBRecord_PanicRecovery_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	ap := &AIProcessor{}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		switch v := target.(type) {
		case *analyzableStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			return nil
		case *contentStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			v.Content = "hello"
			v.ActorID = "alice"
			return nil
		default:
			return errors.New("unexpected target")
		}
	}

	record := events.DynamoDBEventRecord{
		EventID:   "evt-panic",
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("OBJECT#123"),
			},
		},
	}

	err := ap.HandleDynamoDBRecord(&apptheory.EventContext{RequestID: "req-panic"}, record)
	require.Error(t, err)
	require.Contains(t, err.Error(), "panic recovered")
	require.NotNil(t, ap.logger)
}

func TestAIProcessor_Entrypoint_Round12(t *testing.T) {
	origStart := lambdaStartFn
	origUnmarshal := unmarshalItemFn
	origAnalyzerFn := newContentAnalyzerFn
	origSaverFn := newAnalysisSaverFn
	t.Cleanup(func() { lambdaStartFn = origStart })
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })
	t.Cleanup(func() { newContentAnalyzerFn = origAnalyzerFn })
	t.Cleanup(func() { newAnalysisSaverFn = origSaverFn })

	analysis := &ai.AIAnalysis{
		ID:               "analysis-1",
		ObjectID:         "123",
		ObjectType:       "Note",
		ModerationAction: ai.ActionNone,
		OverallRisk:      0.1,
	}
	analyzer := &fakeAnalyzer{analysis: analysis}
	saver := &fakeSaver{}

	processor = &AIProcessor{
		aiAnalyzer: analyzer,
		aiService:  saver,
		logger:     zap.NewNop(),
	}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, target any) error {
		switch v := target.(type) {
		case *analyzableStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			return nil
		case *contentStreamItem:
			v.PK = "OBJECT#123"
			v.Type = "Note"
			v.ActorID = "alice"
			return nil
		default:
			return errors.New("unexpected target")
		}
	}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "1",
				EventName:      "INSERT",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
				Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{
					"PK": events.NewStringAttribute("OBJECT#123"),
				}},
			},
		}}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)
		resp, ok := respAny.(events.DynamoDBEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.True(t, called)
	require.Equal(t, 1, analyzer.calls)
	require.Equal(t, 1, saver.calls)

	newContentAnalyzerFn = func(*common.LambdaContext) (contentAnalyzer, error) { return analyzer, nil }
	newAnalysisSaverFn = func(storageCore.RepositoryStorage, core.DB, *zap.Logger) analysisSaver { return saver }
	p, err := NewSimplifiedAIProcessor(&common.LambdaContext{
		DynamoDB: new(mocks.MockDB),
		Config:   &config.Config{DynamoTableName: "test-table"},
		Logger:   zap.NewNop(),
		Repos:    testingmocks.NewMockRepositoryStorage(),
	})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestHandleAIProcessorStreamRecord_MissingProcessor_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	processor = nil
	require.Error(t, handleAIProcessorStreamRecord(nil, events.DynamoDBEventRecord{EventID: "1", EventName: "INSERT"}))
}

func TestInitializeAIProcessor_FailsClosedOnStorageError(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		newLambdaOptimizedClientFn = origNewClient
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
			Logger: zap.NewNop(),
		}
	}
	newLambdaOptimizedClientFn = func(context.Context, string) (core.DB, error) {
		return nil, errors.New("storage unavailable")
	}

	err := initializeAIProcessor()
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage client initialization failed")
}

func TestInitializeAIStorage_CreatesReposAndStoresClient(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	origNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewFactory
	})

	db := new(mocks.MockDB)
	expectedRepos := testingmocks.NewMockRepositoryStorage()
	var gotRegion, gotTable string
	newLambdaOptimizedClientFn = func(_ context.Context, region string) (core.DB, error) {
		gotRegion = region
		return db, nil
	}
	newRepositoryFactoryFn = func(gotDB core.DB, tableName string, _ *zap.Logger) (storageCore.RepositoryStorage, error) {
		require.Same(t, db, gotDB)
		gotTable = tableName
		return expectedRepos, nil
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "ai-table"},
		Logger: zap.NewNop(),
	}
	gotRepos, err := initializeAIStorage(ctx)
	require.NoError(t, err)
	require.Same(t, expectedRepos, gotRepos)
	require.Same(t, db, ctx.DynamoDB)
	require.Same(t, expectedRepos, ctx.Repos)
	require.Equal(t, "us-east-1", gotRegion)
	require.Equal(t, "ai-table", gotTable)
}

func TestNewSimplifiedAIProcessor_FailsClosedWithoutStorage(t *testing.T) {
	_, err := NewSimplifiedAIProcessor(&common.LambdaContext{
		Config: &config.Config{DynamoTableName: "test-table"},
		Logger: zap.NewNop(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "dynamodb client is not initialized")
}
