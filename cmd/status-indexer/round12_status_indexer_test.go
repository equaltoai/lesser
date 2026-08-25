package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	ttmodel "github.com/theory-cloud/tabletheory/v3/pkg/model"
	"go.uber.org/zap"
)

type fakeLikeCounter struct {
	likes     int64
	boosts    int64
	likesErr  error
	boostsErr error
}

func (f *fakeLikeCounter) GetLikeCount(context.Context, string) (int64, error) {
	return f.likes, f.likesErr
}
func (f *fakeLikeCounter) GetBoostCount(context.Context, string) (int64, error) {
	return f.boosts, f.boostsErr
}

type fakeEmbeddingGenerator struct {
	embedding []float32
	err       error
	calls     int
}

func (f *fakeEmbeddingGenerator) GenerateEmbedding(context.Context, string) ([]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.embedding, nil
}

func newMockDB(t *testing.T) (*dynamormmocks.MockDB, *dynamormmocks.MockQuery) {
	t.Helper()

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)

	q.On("Create").Return(nil)
	q.On("CreateOrUpdate").Return(nil)
	q.On("Update", mock.Anything).Return(nil)
	q.On("Index", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)
	q.On("All", mock.Anything).Return(nil)
	q.On("First", mock.Anything).Return(errors.New("not found"))

	return db, q
}

func requireStatusModelBindsToMainTable(t *testing.T, modelValue any) {
	t.Helper()

	registry := ttmodel.NewRegistry()
	require.NoError(t, registry.Register(modelValue))
	metadata, err := registry.GetMetadata(modelValue)
	require.NoError(t, err)
	require.Equal(t, models.MainTableName, metadata.TableName)
	require.NotEqual(t, "s", metadata.TableName)
}

func TestStatusIndexerSideIndexModels_BindToMainTable_Round12(t *testing.T) {
	requireStatusModelBindsToMainTable(t, &statusWordIndex{})
	requireStatusModelBindsToMainTable(t, &statusTagIndex{})
	requireStatusModelBindsToMainTable(t, &statusAuthorIndex{})
	requireStatusModelBindsToMainTable(t, &statusTrendingHashtag{})
}

func TestNewStatusIndexer_Round12(t *testing.T) {
	db, _ := newMockDB(t)
	logger := zap.NewNop()
	embed := &fakeEmbeddingGenerator{embedding: []float32{0.1, 0.2}}

	si := NewStatusIndexer(db, "lesser-dev", "example.com", embed, logger)

	require.Same(t, db, si.db)
	require.Equal(t, "lesser-dev", si.tableName)
	require.Equal(t, "example.com", si.domain)
	require.Same(t, logger, si.logger)
	require.Same(t, embed, si.aiService)
	require.NotNil(t, si.likeRepo)
	require.NotNil(t, si.hashtagsRepo)
	require.NotNil(t, si.objectRepo)
}

func TestResolveStatusIndexerDB_Round12(t *testing.T) {
	db, _ := newMockDB(t)

	existingCtx := &common.LambdaContext{DynamoDB: db}
	got, err := resolveStatusIndexerDB(existingCtx, &config.Config{Region: "us-east-1"}, nil)
	require.NoError(t, err)
	require.Same(t, db, got)

	_, err = resolveStatusIndexerDB(nil, nil, zap.NewNop())
	require.ErrorContains(t, err, "lambda config is required")

	_, err = resolveStatusIndexerDB(&common.LambdaContext{}, &config.Config{}, zap.NewNop())
	require.ErrorContains(t, err, "AWS region is required")

	origNewLambdaClient := newLambdaClientFn
	t.Cleanup(func() { newLambdaClientFn = origNewLambdaClient })

	newLambdaClientFn = func(ctx context.Context, region string) (dynamormCore.DB, error) {
		require.NotNil(t, ctx)
		require.Equal(t, "us-west-2", region)
		return db, nil
	}

	fallbackCtx := &common.LambdaContext{}
	got, err = resolveStatusIndexerDB(fallbackCtx, &config.Config{Region: " us-west-2 "}, zap.NewNop())
	require.NoError(t, err)
	require.Same(t, db, got)
	require.Same(t, db, fallbackCtx.DynamoDB)
}

func TestStatusIndexer_Helpers_Round12(t *testing.T) {
	si := &StatusIndexer{domain: "example.com", logger: zap.NewNop()}

	require.True(t, si.shouldProcessEvent(events.DynamoDBEventRecord{EventName: "INSERT"}))
	require.True(t, si.shouldProcessEvent(events.DynamoDBEventRecord{EventName: "MODIFY"}))
	require.False(t, si.shouldProcessEvent(events.DynamoDBEventRecord{EventName: "REMOVE"}))

	require.False(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{}))
	require.False(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewNumberAttribute("123")},
		},
	}))
	require.False(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("NOT_OBJECT")},
		},
	}))
	require.False(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("OBJECT#1"),
			},
		},
	}))
	require.True(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("OBJECT#1"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}))
	require.True(t, si.isObjectMetadataRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("object#1"),
				"SK": events.NewStringAttribute("object#1"),
			},
		},
	}))

	_, err := si.extractObjectFromRecord(events.DynamoDBEventRecord{})
	require.Error(t, err)

	_, err = si.extractObjectFromRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
	})
	require.Error(t, err)

	_, err = si.extractObjectFromRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Object": events.NewStringAttribute("nope"),
			},
		},
	})
	require.Error(t, err)

	flatObjectMap, err := si.extractObjectFromRecord(events.DynamoDBEventRecord{
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"type":         events.NewStringAttribute("Note"),
				"id":           events.NewStringAttribute("status-flat"),
				"content":      events.NewStringAttribute("flat record"),
				"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "status-flat", flatObjectMap["id"].String())

	objectMap := map[string]events.DynamoDBAttributeValue{
		"type":         events.NewStringAttribute("Note"),
		"id":           events.NewStringAttribute("status-1"),
		"content":      events.NewStringAttribute("Hello world #Go!"),
		"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
		"published":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
	}

	require.True(t, si.isIndexableObjectType(objectMap))
	require.False(t, si.isIndexableObjectType(map[string]events.DynamoDBAttributeValue{}))
	require.False(t, si.isIndexableObjectType(map[string]events.DynamoDBAttributeValue{
		"type": events.NewNumberAttribute("1"),
	}))

	details := si.extractObjectDetails(objectMap)
	require.Equal(t, "status-1", details.objectID)
	require.Equal(t, "Hello world #Go!", details.content)
	require.Equal(t, "https://example.com/users/alice", details.authorID)
	require.Equal(t, "alice", details.authorUsername)
	require.False(t, details.publicAddressed)

	publicObjectMap := map[string]events.DynamoDBAttributeValue{
		"type":         events.NewStringAttribute("Note"),
		"id":           events.NewStringAttribute("status-public"),
		"content":      events.NewStringAttribute("public content"),
		"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
		"to":           events.NewListAttribute([]events.DynamoDBAttributeValue{events.NewStringAttribute(activitypub.PublicAddress)}),
	}
	publicDetails := si.extractObjectDetails(publicObjectMap)
	require.True(t, publicDetails.publicAddressed)
	require.True(t, publicDetails.audienceObserved)
	require.True(t, si.isSearchIndexableVisibility(publicDetails))
	ccObjectMap := map[string]events.DynamoDBAttributeValue{
		"cc": events.NewStringSetAttribute([]string{activitypub.PublicAddress}),
	}
	ccPublic, ccObserved := si.publicSearchAddressing(ccObjectMap)
	require.True(t, ccPublic)
	require.True(t, ccObserved)
	emptyPublic, emptyObserved := si.publicSearchAddressing(map[string]events.DynamoDBAttributeValue{})
	require.False(t, emptyPublic)
	require.False(t, emptyObserved)
	require.True(t, si.isSearchIndexableVisibility(statusDetails{visibility: models.VisibilityPublic}))
	require.True(t, si.isSearchIndexableVisibility(statusDetails{visibility: models.VisibilityUnlisted}))
	require.False(t, si.isSearchIndexableVisibility(statusDetails{visibility: models.VisibilityPrivate, publicAddressed: true}))
	require.False(t, si.isSearchIndexableVisibility(statusDetails{visibility: models.VisibilityDirect}))
	require.False(t, si.isSearchIndexableVisibility(statusDetails{
		visibility:       models.VisibilityPublic,
		audienceObserved: true,
		publicAddressed:  false,
	}))
	require.False(t, si.isSearchIndexableVisibility(statusDetails{}))
	require.Equal(t, []string{"one"}, dynamoStringValues(events.NewStringAttribute("one")))
	require.ElementsMatch(t, []string{"one", "two"}, dynamoStringValues(events.NewStringSetAttribute([]string{"one", "two"})))
	require.Equal(t, []string{"one"}, dynamoStringValues(events.NewListAttribute([]events.DynamoDBAttributeValue{
		events.NewStringAttribute("one"),
		events.NewNumberAttribute("2"),
	})))
	require.Nil(t, dynamoStringValues(events.NewNumberAttribute("1")))

	require.Equal(t, "", si.extractUsernameFromAuthorID(""))
	require.Equal(t, "bob", si.extractUsernameFromAuthorID("https://remote.example/users/bob"))

	words := si.extractSignificantWords("the quick brown fox jumps over the quick fox")
	require.Contains(t, words, "quick")
	require.NotContains(t, words, "the")

	limited := si.extractSignificantWords("one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree")
	require.Len(t, limited, 20)

	tags := si.extractHashtags("hello #Go! #foo_bar #? #MixedCase")
	require.Equal(t, []string{"go", "foo_bar", "mixedcase"}, tags)

	require.True(t, si.isStopWord("the"))
	require.False(t, si.isStopWord("lesser"))

	require.Equal(t, "example.com", si.getDomain())
	require.Equal(t, "localhost", (&StatusIndexer{}).getDomain())
	require.Equal(t, "https://example.com/statuses/status-1", si.statusURL("status-1"))
	require.Equal(t, "https://remote.example/objects/status-1", si.statusURL("https://remote.example/objects/status-1"))

	require.Equal(t, "unknown", getRequestID(nil))
	require.Equal(t, "unknown", getRequestID(context.Background()))
	require.Equal(t, "req", getRequestID(context.WithValue(context.Background(), requestIDKey, "req")))
	require.Equal(t, "req2", getRequestID(context.WithValue(context.Background(), "request_id", "req2")))
}

func TestStatusIndexer_calculateEngagementAndReplies_Round12(t *testing.T) {
	db, q := newMockDB(t)

	q.ExpectedCalls = nil
	q.On("Index", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)
	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.Object)
		*out = []models.Object{{}, {}}
	}).Return(nil)

	si := &StatusIndexer{
		db:       db,
		logger:   zap.NewNop(),
		likeRepo: &fakeLikeCounter{likes: 2, boosts: 1},
	}

	score, likes, boosts, replies, err := si.calculateEngagement(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, 2, likes)
	require.Equal(t, 1, boosts)
	require.Equal(t, 2, replies)
	require.Equal(t, float64(2)*1.0+float64(1)*2.0+float64(2)*1.5, score)
	q.AssertCalled(t, "Index", "gsi6")

	q.ExpectedCalls = nil
	q.On("Index", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)
	q.On("All", mock.Anything).Return(errors.New("boom"))

	_, _, _, replies, err = si.calculateEngagement(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, 0, replies)
}

func TestStatusIndexer_updateTrendingHashtag_Branches_Round12(t *testing.T) {
	t.Run("update existing", func(t *testing.T) {
		db, q := newMockDB(t)
		q.ExpectedCalls = nil
		q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
		q.On("First", mock.Anything).Return(nil)
		q.On("Update", mock.Anything).Return(nil)

		si := &StatusIndexer{db: db, logger: zap.NewNop()}
		require.NoError(t, si.updateTrendingHashtag(context.Background(), "go", 10))
	})

	t.Run("create new", func(t *testing.T) {
		db, q := newMockDB(t)
		q.ExpectedCalls = nil
		q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
		q.On("First", mock.Anything).Return(errors.New("not found"))
		q.On("Create").Return(nil)

		si := &StatusIndexer{db: db, logger: zap.NewNop()}
		require.NoError(t, si.updateTrendingHashtag(context.Background(), "go", 10))
	})
}

func TestStatusIndexer_processStatusEvent_Round12(t *testing.T) {
	published := time.Now().UTC()

	t.Run("embedding and trending paths", func(t *testing.T) {
		db, q := newMockDB(t)
		q.ExpectedCalls = nil
		q.On("Index", mock.Anything).Return(q)
		q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
		q.On("Limit", mock.Anything).Return(q)
		q.On("All", mock.Anything).Return(nil)
		q.On("First", mock.Anything).Return(errors.New("not found"))
		q.On("Create").Return(nil)
		q.On("CreateOrUpdate").Return(nil)
		q.On("Update", mock.Anything).Return(nil)

		embed := &fakeEmbeddingGenerator{embedding: []float32{0.1, 0.2}}
		si := &StatusIndexer{
			db:        db,
			logger:    zap.NewNop(),
			domain:    "example.com",
			aiService: embed,
			likeRepo:  &fakeLikeCounter{likes: 11, boosts: 0},
		}

		ctx := context.WithValue(context.Background(), requestIDKey, "req")
		require.NoError(t, si.processStatusEvent(ctx, "status-1", "Hello world #Go!", "author-1", "alice", published))
		require.Equal(t, 1, embed.calls)
	})

	t.Run("embedding errors and db create failures are non-fatal", func(t *testing.T) {
		db, q := newMockDB(t)
		q.ExpectedCalls = nil
		q.On("Index", mock.Anything).Return(q)
		q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
		q.On("Limit", mock.Anything).Return(q)
		q.On("All", mock.Anything).Return(nil)
		q.On("First", mock.Anything).Return(errors.New("not found"))
		q.On("Create").Return(errors.New("boom"))
		q.On("CreateOrUpdate").Return(errors.New("boom"))
		q.On("Update", mock.Anything).Return(errors.New("boom"))

		embed := &fakeEmbeddingGenerator{err: errors.New("boom")}
		si := &StatusIndexer{
			db:        db,
			logger:    zap.NewNop(),
			domain:    "example.com",
			aiService: embed,
			likeRepo:  &fakeLikeCounter{likesErr: errors.New("boom"), boostsErr: errors.New("boom")},
		}

		ctx := context.WithValue(context.Background(), requestIDKey, "req")
		require.NoError(t, si.processStatusEvent(ctx, "status-1", "Hello world #Go!", "author-1", "alice", published))
		require.Equal(t, 1, embed.calls)
	})
}

func TestStatusIndexer_processRecord_Round12(t *testing.T) {
	db, _ := newMockDB(t)

	si := &StatusIndexer{
		db:       db,
		logger:   zap.NewNop(),
		likeRepo: &fakeLikeCounter{},
	}

	require.NoError(t, si.processRecord(context.Background(), events.DynamoDBEventRecord{EventName: "REMOVE"}))

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("object#status-1"),
				"SK": events.NewStringAttribute("object#status-1"),
			},
		},
	}
	require.NoError(t, si.processRecord(context.Background(), record))

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"type": events.NewStringAttribute("Image"),
	}
	require.NoError(t, si.processRecord(context.Background(), record))

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"type":         events.NewStringAttribute("Note"),
		"id":           events.NewStringAttribute("status-1"),
		"content":      events.NewStringAttribute("Hello world"),
		"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
		"published":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
		"visibility":   events.NewStringAttribute(models.VisibilityPublic),
		"to":           events.NewListAttribute([]events.DynamoDBAttributeValue{events.NewStringAttribute(activitypub.PublicAddress)}),
	}
	require.NoError(t, si.processRecord(context.Background(), record))

	legacyRecord := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("OBJECT#legacy"),
				"SK": events.NewStringAttribute("METADATA"),
			},
			NewImage: map[string]events.DynamoDBAttributeValue{
				"Object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"type":         events.NewStringAttribute("Note"),
					"id":           events.NewStringAttribute("status-legacy"),
					"content":      events.NewStringAttribute("Hello legacy"),
					"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
					"published":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
					"visibility":   events.NewStringAttribute(models.VisibilityPublic),
				}),
			},
		},
	}
	require.NoError(t, si.processRecord(context.Background(), legacyRecord))
}

func TestStatusIndexer_processRecord_SkipsNonPublicObjects(t *testing.T) {
	db, _ := newMockDB(t)
	embed := &fakeEmbeddingGenerator{embedding: []float32{0.1, 0.2}}

	si := &StatusIndexer{
		db:        db,
		logger:    zap.NewNop(),
		aiService: embed,
		likeRepo:  &fakeLikeCounter{},
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			Keys: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("object#status-private"),
				"SK": events.NewStringAttribute("object#status-private"),
			},
			NewImage: map[string]events.DynamoDBAttributeValue{
				"type":         events.NewStringAttribute("Note"),
				"id":           events.NewStringAttribute("status-private"),
				"content":      events.NewStringAttribute("private #secret content"),
				"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
				"visibility":   events.NewStringAttribute(models.VisibilityDirect),
				"to":           events.NewListAttribute([]events.DynamoDBAttributeValue{events.NewStringAttribute("https://example.com/users/bob")}),
			},
		},
	}

	require.NoError(t, si.processRecord(context.Background(), record))
	require.Equal(t, 0, embed.calls)
}

func TestHandleStatusIndexerStreamRecord_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	processor = nil
	require.Error(t, handleStatusIndexerStreamRecord(nil, events.DynamoDBEventRecord{}))

	processor = &StatusIndexer{logger: zap.NewNop(), likeRepo: &fakeLikeCounter{}}
	require.NoError(t, handleStatusIndexerStreamRecord(&apptheory.EventContext{RequestID: "req"}, events.DynamoDBEventRecord{EventName: "REMOVE"}))
}

func TestStatusIndexer_Main_WiresLambdaStart_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origLoadAWS := loadAWSConfigFn
	origNewAI := newAIServiceFn
	origNewIndexer := newStatusIndexerFn
	origNewClient := newLambdaClientFn
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		loadAWSConfigFn = origLoadAWS
		newAIServiceFn = origNewAI
		newStatusIndexerFn = origNewIndexer
		newLambdaClientFn = origNewClient
		lambdaStartFn = origStart
		processor = origProcessor
	})

	mockDB := new(dynamormmocks.MockDB)
	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Domain:          "example.com",
				Region:          "us-east-1",
				DynamoTableName: "test-table",
			},
			Logger:   zap.NewNop(),
			DynamoDB: mockDB,
		}
	}
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newAIServiceFn = func(aws.Config, *ai.Config) embeddingGenerator { return nil }
	newStatusIndexerFn = func(db dynamormCore.DB, tableName, domain string, _ embeddingGenerator, logger *zap.Logger) *StatusIndexer {
		return &StatusIndexer{db: db, tableName: tableName, domain: domain, logger: logger, likeRepo: &fakeLikeCounter{}}
	}

	called := false
	lambdaStartFn = func(h any) {
		called = true
		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
			{
				EventID:        "1",
				EventName:      "REMOVE",
				EventSource:    "aws:dynamodb",
				EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000",
				Change:         events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{}},
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

	main()
	require.True(t, called)
}

func TestStatusIndexer_Main_FallsBackToManualDynamoClient_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origLoadAWS := loadAWSConfigFn
	origNewAI := newAIServiceFn
	origNewIndexer := newStatusIndexerFn
	origNewClient := newLambdaClientFn
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		loadAWSConfigFn = origLoadAWS
		newAIServiceFn = origNewAI
		newStatusIndexerFn = origNewIndexer
		newLambdaClientFn = origNewClient
		lambdaStartFn = origStart
		processor = origProcessor
	})

	mockDB := new(dynamormmocks.MockDB)
	fakeCtx := &common.LambdaContext{
		Config: &config.Config{
			Domain:          "example.com",
			Region:          "us-east-1",
			DynamoTableName: "test-table",
		},
		Logger:   zap.NewNop(),
		DynamoDB: nil,
	}
	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext { return fakeCtx }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newAIServiceFn = func(aws.Config, *ai.Config) embeddingGenerator { return nil }
	newLambdaClientFn = func(context.Context, string) (dynamormCore.DB, error) { return mockDB, nil }

	var gotDB dynamormCore.DB
	newStatusIndexerFn = func(db dynamormCore.DB, tableName, domain string, _ embeddingGenerator, logger *zap.Logger) *StatusIndexer {
		gotDB = db
		return &StatusIndexer{db: db, tableName: tableName, domain: domain, logger: logger, likeRepo: &fakeLikeCounter{}}
	}

	called := false
	lambdaStartFn = func(h any) {
		called = true
	}

	main()
	require.True(t, called)
	require.Same(t, mockDB, gotDB)
	require.Same(t, mockDB, fakeCtx.DynamoDB)
}
