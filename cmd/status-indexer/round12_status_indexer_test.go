package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeLikeCounter struct {
	likes     int64
	boosts    int64
	likesErr  error
	boostsErr error
}

func (f *fakeLikeCounter) GetLikeCount(context.Context, string) (int64, error)  { return f.likes, f.likesErr }
func (f *fakeLikeCounter) GetBoostCount(context.Context, string) (int64, error) { return f.boosts, f.boostsErr }

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
	q.On("Update", mock.Anything).Return(nil)
	q.On("Index", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("Limit", mock.Anything).Return(q)
	q.On("All", mock.Anything).Return(nil)
	q.On("First", mock.Anything).Return(errors.New("not found"))

	return db, q
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
		db:      db,
		logger:  zap.NewNop(),
		likeRepo: &fakeLikeCounter{likes: 2, boosts: 1},
	}

	score, likes, boosts, replies, err := si.calculateEngagement(context.Background(), "status-1")
	require.NoError(t, err)
	require.Equal(t, 2, likes)
	require.Equal(t, 1, boosts)
	require.Equal(t, 2, replies)
	require.Equal(t, float64(2)*1.0+float64(1)*2.0+float64(2)*1.5, score)

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
		q.On("Update", mock.Anything).Return(nil)

		embed := &fakeEmbeddingGenerator{embedding: []float32{0.1, 0.2}}
		si := &StatusIndexer{
			db:       db,
			logger:   zap.NewNop(),
			domain:   "example.com",
			aiService: embed,
			likeRepo: &fakeLikeCounter{likes: 11, boosts: 0},
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
		q.On("Update", mock.Anything).Return(errors.New("boom"))

		embed := &fakeEmbeddingGenerator{err: errors.New("boom")}
		si := &StatusIndexer{
			db:       db,
			logger:   zap.NewNop(),
			domain:   "example.com",
			aiService: embed,
			likeRepo: &fakeLikeCounter{likesErr: errors.New("boom"), boostsErr: errors.New("boom")},
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
				"PK": events.NewStringAttribute("OBJECT#1"),
				"SK": events.NewStringAttribute("METADATA"),
			},
		},
	}
	require.NoError(t, si.processRecord(context.Background(), record))

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"Object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"type": events.NewStringAttribute("Image"),
		}),
	}
	require.NoError(t, si.processRecord(context.Background(), record))

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"Object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"type":         events.NewStringAttribute("Note"),
			"id":           events.NewStringAttribute("status-1"),
			"content":      events.NewStringAttribute("Hello world"),
			"attributedTo": events.NewStringAttribute("https://example.com/users/alice"),
			"published":    events.NewStringAttribute(time.Now().UTC().Format(time.RFC3339)),
		}),
	}
	require.NoError(t, si.processRecord(context.Background(), record))
}

func TestHandleStatusIndexerStream_Round12(t *testing.T) {
	origProcessor := processor
	t.Cleanup(func() { processor = origProcessor })

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))

	processor = nil
	require.Error(t, handleStatusIndexerStream(ctx))

	processor = &StatusIndexer{logger: zap.NewNop(), likeRepo: &fakeLikeCounter{}}
	ctx.Request.TriggerType = lift.TriggerEventBus
	ctx.Request.Records = []any{"bad-record"}
	require.Error(t, handleStatusIndexerStream(ctx))

	ctx.Request.Records = []any{}
	require.NoError(t, handleStatusIndexerStream(ctx))
}

func TestStatusIndexer_Main_WiresLambdaStart_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origLoadAWS := loadAWSConfigFn
	origNewAI := newAIServiceFn
	origNewIndexer := newStatusIndexerFn
	origStart := lambdaStartFn
	origProcessor := processor
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		loadAWSConfigFn = origLoadAWS
		newAIServiceFn = origNewAI
		newStatusIndexerFn = origNewIndexer
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
			Logger:  zap.NewNop(),
			DynamoDB: mockDB,
		}
	}
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newAIServiceFn = func(aws.Config, *ai.AIConfig) embeddingGenerator { return nil }
	newStatusIndexerFn = func(db dynamormCore.DB, tableName, domain string, _ embeddingGenerator, logger *zap.Logger) *StatusIndexer {
		return &StatusIndexer{db: db, tableName: tableName, domain: domain, logger: logger, likeRepo: &fakeLikeCounter{}}
	}

	called := false
	lambdaStartFn = func(any) { called = true }

	main()
	require.True(t, called)
}
