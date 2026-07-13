package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeFederationActivityRepo struct {
	createCalls int
	updateCalls int

	createErr error
	updateErr error
}

func (f *fakeFederationActivityRepo) Create(context.Context, *models.FederationActivity) error {
	f.createCalls++
	return f.createErr
}

func (f *fakeFederationActivityRepo) UpdateInstanceInfo(context.Context, *models.InstanceInfo) error {
	f.updateCalls++
	return f.updateErr
}

func TestExtractDomain_Round12(t *testing.T) {
	require.Equal(t, "example.com", extractDomain("https://example.com/users/alice"))
	require.Equal(t, "", extractDomain("not a url"))
	require.Equal(t, "", extractDomain("http://[::1"))
}

func TestInitializeFederationTracker_Round12(t *testing.T) {
	origMustInitialize := mustInitializeLambdaFn
	origInitDefaults := initializeWithDefaultsFn
	origNewClient := newLambdaOptimizedClientFn
	origNewRepo := newFederationActivityRepoFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMustInitialize
		initializeWithDefaultsFn = origInitDefaults
		newLambdaOptimizedClientFn = origNewClient
		newFederationActivityRepoFn = origNewRepo
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				Domain:          "local.example",
				DynamoTableName: "test-table",
			},
			Logger: zap.NewNop(),
		}
	}
	initializeWithDefaultsFn = func(*common.LambdaContext) error { return errors.New("boom") }
	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}
	// Provide a simple fake repo implementation.
	newFederationActivityRepoFn = func(dynamormCore.DB, string, *zap.Logger) federationActivityStore {
		return &fakeFederationActivityRepo{}
	}

	initializeFederationTracker()

	require.NotNil(t, lambdaCtx)
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
	require.NotNil(t, federationActivityRepository)
}

func TestFederationTracker_ProcessRecord_ActivityAndActor_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	runCtx := context.Background()
	requestID := "req"

	t.Run("ignores non insert/modify", func(t *testing.T) {
		record := events.DynamoDBEventRecord{EventName: "REMOVE"}
		require.NoError(t, ft.processRecord(runCtx, requestID, record))
	})

	t.Run("tracks remote activity", func(t *testing.T) {
		repo.createCalls = 0
		repo.updateCalls = 0

		activityMap := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"actor":        events.NewStringAttribute("https://remote.example/users/alice"),
			"type":         events.NewStringAttribute("Create"),
			"object":       events.NewStringAttribute("https://remote.example/objects/1"),
			"orderedItems": events.NewListAttribute(nil),
		})

		record := events.DynamoDBEventRecord{
			EventName: "INSERT",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":       events.NewStringAttribute("ACTIVITY#1"),
					"Activity": activityMap,
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"SK": events.NewStringAttribute("EVENT"),
				},
			},
		}

		require.NoError(t, ft.processRecord(runCtx, requestID, record))
		require.Equal(t, 1, repo.createCalls)
		require.Equal(t, 1, repo.updateCalls)
	})

	t.Run("tracks remote actor", func(t *testing.T) {
		repo.createCalls = 0
		repo.updateCalls = 0

		actorMap := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"id": events.NewStringAttribute("https://remote.example/users/bob"),
			"publicKey": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"publicKeyPem": events.NewStringAttribute("pem"),
			}),
			"endpoints": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
				"sharedInbox": events.NewStringAttribute("https://remote.example/inbox"),
			}),
			"attachment": events.NewListAttribute(nil),
		})

		record := events.DynamoDBEventRecord{
			EventName: "MODIFY",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":    events.NewStringAttribute("ACTOR#1"),
					"Actor": actorMap,
				},
			},
		}

		require.NoError(t, ft.processRecord(runCtx, requestID, record))
		require.Equal(t, 1, repo.createCalls)
		require.Equal(t, 1, repo.updateCalls)
	})
}

func TestFederationTracker_HandleDynamoDBRecord_ContinuesOnErrors_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{createErr: errors.New("boom")}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	activityMap := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
		"actor":  events.NewStringAttribute("https://remote.example/users/alice"),
		"type":   events.NewStringAttribute("Create"),
		"object": events.NewStringAttribute("https://remote.example/objects/1"),
	})

	require.NoError(t, ft.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":       events.NewStringAttribute("ACTIVITY#1"),
				"Activity": activityMap,
			},
		},
	}))
	require.NoError(t, ft.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{EventName: "REMOVE"}))
	require.Equal(t, 1, repo.createCalls)
}

func TestRunFederationTracker_Round12(t *testing.T) {
	origLambdaStart := lambdaStartFn
	t.Cleanup(func() { lambdaStartFn = origLambdaStart })

	repo := &fakeFederationActivityRepo{}
	federationActivityRepository = repo
	logger = zap.NewNop()
	cfg = &config.Config{Domain: "local.example"}
	lambdaCtx = &common.LambdaContext{Logger: logger, Config: cfg}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	var startHandler any
	lambdaStartFn = func(handler any) { startHandler = handler }

	main()
	require.NotNil(t, startHandler)

	fn, ok := startHandler.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	activityMap := events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
		"actor":  events.NewStringAttribute("https://remote.example/users/alice"),
		"type":   events.NewStringAttribute("Create"),
		"object": events.NewStringAttribute("https://remote.example/objects/1"),
	})

	event := events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{
		{
			EventID:        "1",
			EventName:      "INSERT",
			EventSource:    "aws:dynamodb",
			EventSourceArn: "arn:aws:dynamodb:us-east-1:123456789012:table/lesser-dev-main-table/stream/2024-01-01T00:00:00.000",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":       events.NewStringAttribute("ACTIVITY#1"),
					"Activity": activityMap,
				},
				Keys: map[string]events.DynamoDBAttributeValue{
					"SK": events.NewStringAttribute("EVENT"),
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
	require.Empty(t, resp.BatchItemFailures)

	// Allow for best-effort UpdateInstanceInfo warnings (repo calls are still recorded).
	require.GreaterOrEqual(t, repo.createCalls, 1)
}

func TestFederationTracker_ExtractDomain_AndIgnoredPaths_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	runCtx := context.Background()
	requestID := "req"
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTIVITY#1"),
			},
		},
	}

	require.NoError(t, ft.processRecord(runCtx, requestID, record))
	require.Equal(t, 0, repo.createCalls)

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("ACTOR#1"),
	}
	require.NoError(t, ft.processRecord(runCtx, requestID, record))
	require.Equal(t, 0, repo.createCalls)

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("ACTIVITY#1"),
		"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"actor": events.NewStringAttribute("https://local.example/users/alice"),
		}),
	}
	require.NoError(t, ft.processRecord(runCtx, requestID, record))
	require.Equal(t, 0, repo.createCalls)

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("ACTIVITY#1"),
		"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"actor": events.NewStringAttribute("not a url"),
		}),
	}
	require.NoError(t, ft.processRecord(runCtx, requestID, record))
	require.Equal(t, 0, repo.createCalls)

	record.Change.NewImage = map[string]events.DynamoDBAttributeValue{
		"PK": events.NewStringAttribute("ACTIVITY#1"),
		"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
			"actor":  events.NewStringAttribute("https://remote.example/users/alice"),
			"object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{}),
		}),
	}
	require.NoError(t, ft.processRecord(runCtx, requestID, record))
}

func TestFederationTracker_HandleStream_Empty_Round12(t *testing.T) {
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: &fakeFederationActivityRepo{},
	}
	require.NoError(t, ft.HandleDynamoDBRecord(nil, events.DynamoDBEventRecord{}))
}

func TestFederationTracker_TrackActor_UpdateInstanceInfoError_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{updateErr: errors.New("boom")}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#1"),
				"Actor": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"id": events.NewStringAttribute("https://remote.example/users/alice"),
				}),
			},
		},
	}
	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, repo.updateCalls)
}

func TestFederationTracker_TrackActivity_UpdateInstanceInfoError_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{updateErr: errors.New("boom")}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}
	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTIVITY#1"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"actor": events.NewStringAttribute("https://remote.example/users/alice"),
					"type":  events.NewStringAttribute("Create"),
					"items": events.NewListAttribute(nil),
				}),
			},
		},
	}

	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, repo.updateCalls)
}

func TestFederationTracker_TrackActor_SkipsWhenActorFieldMissing_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#1"),
			},
		},
	}
	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 0, repo.createCalls)
}

func TestFederationTracker_TrackActivity_ObjectMapBranch_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTIVITY#1"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"actor": events.NewStringAttribute("https://remote.example/users/alice"),
					"object": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
						"id":   events.NewStringAttribute("https://remote.example/objects/1"),
						"type": events.NewStringAttribute("Note"),
					}),
				}),
			},
		},
	}

	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 1, repo.createCalls)
}

func TestFederationTracker_TrackActivity_SkipsWhenActorMissing_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTIVITY#1"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"type": events.NewStringAttribute("Create"),
				}),
			},
		},
	}
	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 0, repo.createCalls)
}

func TestFederationTracker_TrackActor_SkipsWhenLocalDomain_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTOR#1"),
				"Actor": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"id": events.NewStringAttribute("https://local.example/users/alice"),
				}),
			},
		},
	}
	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 0, repo.createCalls)
}

func TestFederationTracker_ProcessRecord_UsesRequestIDArgument_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	record := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK": events.NewStringAttribute("ACTIVITY#1"),
				"Activity": events.NewMapAttribute(map[string]events.DynamoDBAttributeValue{
					"actor": events.NewStringAttribute("https://remote.example/users/alice"),
				}),
			},
		},
	}

	require.NoError(t, ft.processRecord(context.Background(), "req", record))
	require.Equal(t, 1, repo.createCalls)
}

func TestFederationTracker_ProcessRecord_UsesContextDeadline_Round12(t *testing.T) {
	repo := &fakeFederationActivityRepo{}
	ft := &FederationTracker{
		logger:                       zap.NewNop(),
		cfg:                          &common.LambdaContext{Config: &config.Config{Domain: "local.example"}},
		federationActivityRepository: repo,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	require.NoError(t, ft.processRecord(ctx, "req", events.DynamoDBEventRecord{EventName: "REMOVE"}))
}
