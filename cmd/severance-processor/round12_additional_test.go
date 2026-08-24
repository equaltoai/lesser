package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services"
	severanceService "github.com/equaltoai/lesser/pkg/services/severance"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	storageInterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeSeveranceService struct {
	calls int
	err   error
}

func (f *fakeSeveranceService) DetectSeverance(context.Context, string, models.SeveranceReason, int, int, string) (*severanceService.SeveredRelationship, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &severanceService.SeveredRelationship{}, nil
}

type fakeRepoStorage struct {
	*testingmocks.MockRepositoryStorage
	rel storageInterfaces.ConcreteRelationshipRepository
}

func (f *fakeRepoStorage) Relationship() storageInterfaces.ConcreteRelationshipRepository {
	return f.rel
}

type fakeRegistry struct {
	storage storageCore.RepositoryStorage
	sev     severanceDetector
}

func (f *fakeRegistry) Severance() severanceDetector              { return f.sev }
func (f *fakeRegistry) GetStorage() storageCore.RepositoryStorage { return f.storage }

func TestInitializeSeveranceProcessor_SetsGlobalsAndAdapter(t *testing.T) {
	originalMustInitialize := mustInitializeLambdaFn
	originalNewRegistry := newRegistryFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInitialize
		newRegistryFn = originalNewRegistry
	})

	storage := testingmocks.NewMockRepositoryStorage()
	storage.On("Severance").Return(nil).Maybe()

	fakeLambdaCtx := &common.LambdaContext{
		Config: &config.Config{
			Domain: "https://example.com",
		},
		Logger: zap.NewNop(),
		Repos:  storage,
	}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext { return fakeLambdaCtx }
	newRegistryFn = services.NewRegistry

	require.NoError(t, initializeSeveranceProcessor())
	require.NotNil(t, cfg)
	require.NotNil(t, logger)
	require.NotNil(t, registry)
	require.NotNil(t, processor)

	adapter := servicesRegistryAdapter{Registry: registry}
	require.Equal(t, registry.GetStorage(), adapter.GetStorage())
	require.Nil(t, adapter.Severance())
}

func TestInitializeSeveranceProcessor_RegistryError(t *testing.T) {
	originalMustInitialize := mustInitializeLambdaFn
	originalNewRegistry := newRegistryFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = originalMustInitialize
		newRegistryFn = originalNewRegistry
	})

	storage := testingmocks.NewMockRepositoryStorage()
	fakeLambdaCtx := &common.LambdaContext{
		Config: &config.Config{Domain: "https://example.com"},
		Logger: zap.NewNop(),
		Repos:  storage,
	}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext { return fakeLambdaCtx }
	newRegistryFn = func(...services.RegistryOption) (*services.Registry, error) {
		return nil, errors.New("boom")
	}

	require.Error(t, initializeSeveranceProcessor())
}

func TestSeveranceHelpers_Getters(t *testing.T) {
	image := map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IsHealthy": events.NewBooleanAttribute(false),
	}

	require.Equal(t, "remote.example", getStringValue(image, "Domain"))
	require.Equal(t, "", getStringValue(image, "Missing"))
	require.False(t, getBoolValue(image, "IsHealthy", true))
	require.True(t, getBoolValue(image, "Missing", true))
}

func TestSeveranceProcessor_CountAffectedRelationships_Branches(t *testing.T) {
	p := &SeveranceProcessor{logger: zap.NewNop(), registry: &fakeRegistry{}}
	followers, following := p.countAffectedRelationships(context.Background(), "remote.example")
	require.Equal(t, 0, followers)
	require.Equal(t, 0, following)

	baseStorage := testingmocks.NewMockRepositoryStorage()
	p.registry = &fakeRegistry{storage: &fakeRepoStorage{MockRepositoryStorage: baseStorage, rel: nil}}
	followers, following = p.countAffectedRelationships(context.Background(), "remote.example")
	require.Equal(t, 0, followers)
	require.Equal(t, 0, following)

	relRepo := testingmocks.NewMockRelationshipRepository()
	relRepo.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(0, 0, errors.New("boom"))
	p.registry = &fakeRegistry{storage: &fakeRepoStorage{MockRepositoryStorage: baseStorage, rel: relRepo}}
	followers, following = p.countAffectedRelationships(context.Background(), "remote.example")
	require.Equal(t, 0, followers)
	require.Equal(t, 0, following)

	relRepo2 := testingmocks.NewMockRelationshipRepository()
	relRepo2.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(3, 4, nil)
	p.registry = &fakeRegistry{storage: &fakeRepoStorage{MockRepositoryStorage: baseStorage, rel: relRepo2}}
	followers, following = p.countAffectedRelationships(context.Background(), "remote.example")
	require.Equal(t, 3, followers)
	require.Equal(t, 4, following)
}

func TestSeveranceProcessor_HandleDynamoDBRecord_ContinuesOnError(t *testing.T) {
	sev := &fakeSeveranceService{err: errors.New("boom")}
	p := &SeveranceProcessor{
		logger: zap.NewNop(),
		registry: &fakeRegistry{
			storage: &fakeRepoStorage{
				MockRepositoryStorage: testingmocks.NewMockRepositoryStorage(),
				rel: func() storageInterfaces.ConcreteRelationshipRepository {
					rel := testingmocks.NewMockRelationshipRepository()
					rel.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(1, 2, nil)
					return rel
				}(),
			},
			sev: sev,
		},
	}

	ctx := &apptheory.EventContext{RequestID: "req"}
	records := []events.DynamoDBEventRecord{
		{
			EventName: "INSERT",
			EventID:   "e1",
			Change: events.DynamoDBStreamRecord{
				NewImage: map[string]events.DynamoDBAttributeValue{
					"PK":     events.NewStringAttribute("DOMAIN_BLOCK#remote.example"),
					"SK":     events.NewStringAttribute("METADATA"),
					"Domain": events.NewStringAttribute("remote.example"),
				},
			},
		},
		{EventName: "REMOVE", EventID: "e2"},
	}

	for _, record := range records {
		require.NoError(t, p.HandleDynamoDBRecord(ctx, record))
	}
}

func TestSeveranceProcessor_ProcessRecord_RoutesAndBranches(t *testing.T) {
	sev := &fakeSeveranceService{}
	p := &SeveranceProcessor{
		logger: zap.NewNop(),
		registry: &fakeRegistry{
			storage: &fakeRepoStorage{
				MockRepositoryStorage: testingmocks.NewMockRepositoryStorage(),
				rel: func() storageInterfaces.ConcreteRelationshipRepository {
					rel := testingmocks.NewMockRelationshipRepository()
					rel.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(1, 2, nil)
					return rel
				}(),
			},
			sev: sev,
		},
	}

	require.NoError(t, p.processRecord(context.Background(), events.DynamoDBEventRecord{EventName: "REMOVE"}))
	require.NoError(t, p.processRecord(context.Background(), events.DynamoDBEventRecord{EventName: "INSERT"}))

	require.NoError(t, p.processRecord(context.Background(), events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change:    events.DynamoDBStreamRecord{NewImage: map[string]events.DynamoDBAttributeValue{"PK": events.NewStringAttribute("x")}},
	}))

	// Domain block -> DetectSeverance called.
	domainBlock := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":     events.NewStringAttribute("DOMAIN_BLOCK#remote.example"),
				"SK":     events.NewStringAttribute("METADATA"),
				"Domain": events.NewStringAttribute("remote.example"),
			},
		},
	}
	require.NoError(t, p.processRecord(context.Background(), domainBlock))
	require.Equal(t, 1, sev.calls)

	// Federation issue -> DetectSeverance called.
	issue := events.DynamoDBEventRecord{
		EventName: "MODIFY",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("FEDERATION_ISSUE#1"),
				"SK":        events.NewStringAttribute("METADATA"),
				"Domain":    events.NewStringAttribute("remote.example"),
				"IssueType": events.NewStringAttribute("unreachable"),
				"Severity":  events.NewStringAttribute("critical"),
			},
		},
	}
	require.NoError(t, p.processRecord(context.Background(), issue))
	require.Equal(t, 2, sev.calls)

	// Health metrics -> DetectSeverance called.
	health := events.DynamoDBEventRecord{
		EventName: "INSERT",
		Change: events.DynamoDBStreamRecord{
			NewImage: map[string]events.DynamoDBAttributeValue{
				"PK":        events.NewStringAttribute("FEDERATION_METRICS#1"),
				"SK":        events.NewStringAttribute("HEALTH#v1"),
				"Domain":    events.NewStringAttribute("remote.example"),
				"IsHealthy": events.NewBooleanAttribute(false),
			},
		},
	}
	require.NoError(t, p.processRecord(context.Background(), health))
	require.Equal(t, 3, sev.calls)
}

func TestSeveranceProcessor_HandleDomainBlock_Branches(t *testing.T) {
	p := &SeveranceProcessor{
		logger: zap.NewNop(),
		registry: &fakeRegistry{
			storage: &fakeRepoStorage{
				MockRepositoryStorage: testingmocks.NewMockRepositoryStorage(),
				rel: func() storageInterfaces.ConcreteRelationshipRepository {
					rel := testingmocks.NewMockRelationshipRepository()
					rel.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(1, 2, nil)
					return rel
				}(),
			},
			sev: nil,
		},
	}

	require.NoError(t, p.handleDomainBlock(context.Background(), map[string]events.DynamoDBAttributeValue{}))

	err := p.handleDomainBlock(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain": events.NewStringAttribute("remote.example"),
	})
	require.Error(t, err)

	sev := &fakeSeveranceService{err: errors.New("boom")}
	p.registry = &fakeRegistry{
		storage: &fakeRepoStorage{
			MockRepositoryStorage: testingmocks.NewMockRepositoryStorage(),
			rel: func() storageInterfaces.ConcreteRelationshipRepository {
				rel := testingmocks.NewMockRelationshipRepository()
				rel.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(1, 2, nil)
				return rel
			}(),
		},
		sev: sev,
	}
	require.Error(t, p.handleDomainBlock(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain": events.NewStringAttribute("remote.example"),
	}))
}

func TestSeveranceProcessor_HandleFederationIssue_AndHealth_Branches(t *testing.T) {
	baseStorage := testingmocks.NewMockRepositoryStorage()

	withCounts := func(followers, following int) storageCore.RepositoryStorage {
		return &fakeRepoStorage{
			MockRepositoryStorage: baseStorage,
			rel: func() storageInterfaces.ConcreteRelationshipRepository {
				rel := testingmocks.NewMockRelationshipRepository()
				rel.On("CountRelationshipsByDomain", mock.Anything, "remote.example").Return(followers, following, nil)
				return rel
			}(),
		}
	}

	p := &SeveranceProcessor{
		logger: zap.NewNop(),
		registry: &fakeRegistry{
			storage: withCounts(0, 0),
			sev:     &fakeSeveranceService{},
		},
	}

	// Issue gating branches.
	require.NoError(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{}))
	require.NoError(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IssueType": events.NewStringAttribute("unreachable"),
		"Severity":  events.NewStringAttribute("low"),
	}))
	require.NoError(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IssueType": events.NewStringAttribute("other"),
		"Severity":  events.NewStringAttribute("critical"),
	}))

	// Early return when no relationships.
	require.NoError(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IssueType": events.NewStringAttribute("timeout"),
		"Severity":  events.NewStringAttribute("high"),
	}))

	// Severance service nil branch.
	p.registry = &fakeRegistry{storage: withCounts(1, 2), sev: nil}
	require.Error(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IssueType": events.NewStringAttribute("timeout"),
		"Severity":  events.NewStringAttribute("critical"),
	}))

	// DetectSeverance error branch.
	p.registry = &fakeRegistry{storage: withCounts(1, 2), sev: &fakeSeveranceService{err: errors.New("boom")}}
	require.Error(t, p.handleFederationIssue(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IssueType": events.NewStringAttribute("unreachable"),
		"Severity":  events.NewStringAttribute("critical"),
	}))

	// Federation health gating branches.
	require.NoError(t, p.handleFederationHealth(context.Background(), map[string]events.DynamoDBAttributeValue{}))
	require.NoError(t, p.handleFederationHealth(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IsHealthy": events.NewBooleanAttribute(true),
	}))

	// Early return when no relationships.
	p.registry = &fakeRegistry{storage: withCounts(0, 0), sev: &fakeSeveranceService{}}
	require.NoError(t, p.handleFederationHealth(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IsHealthy": events.NewBooleanAttribute(false),
	}))

	// Severance service nil branch.
	p.registry = &fakeRegistry{storage: withCounts(1, 2), sev: nil}
	require.Error(t, p.handleFederationHealth(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IsHealthy": events.NewBooleanAttribute(false),
	}))

	// DetectSeverance error branch.
	p.registry = &fakeRegistry{storage: withCounts(1, 2), sev: &fakeSeveranceService{err: errors.New("boom")}}
	require.Error(t, p.handleFederationHealth(context.Background(), map[string]events.DynamoDBAttributeValue{
		"Domain":    events.NewStringAttribute("remote.example"),
		"IsHealthy": events.NewBooleanAttribute(false),
	}))
}

func TestMain_UsesLambdaStartFn(t *testing.T) {
	originalStart := lambdaStartFn
	originalLogger := logger
	originalProcessor := processor
	t.Cleanup(func() {
		lambdaStartFn = originalStart
		logger = originalLogger
		processor = originalProcessor
	})

	logger = zap.NewNop()
	processor = &SeveranceProcessor{logger: zap.NewNop(), registry: &fakeRegistry{}}

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
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK":     events.NewStringAttribute("DOMAIN_BLOCK#remote.example"),
						"SK":     events.NewStringAttribute("METADATA"),
						"Domain": events.NewStringAttribute("remote.example"),
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
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.True(t, called)
}

func TestInitializeSeveranceStorage_FailsClosed(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() { newLambdaOptimizedClientFn = origNewClient })

	newLambdaOptimizedClientFn = func(context.Context, string) (dynamormCore.DB, error) {
		return nil, errors.New("storage unavailable")
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
		Logger: zap.NewNop(),
	}
	_, err := initializeSeveranceStorage(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage client initialization failed")
}

func TestInitializeSeveranceStorage_CreatesReposAndStoresClient(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	origNewFactory := newRepositoryFactoryFn
	t.Cleanup(func() {
		newLambdaOptimizedClientFn = origNewClient
		newRepositoryFactoryFn = origNewFactory
	})

	db := dynamormmocks.NewMockExtendedDB()
	expectedRepos := testingmocks.NewMockRepositoryStorage()
	var gotRegion, gotTable string
	newLambdaOptimizedClientFn = func(_ context.Context, region string) (dynamormCore.DB, error) {
		gotRegion = region
		return db, nil
	}
	newRepositoryFactoryFn = func(gotDB dynamormCore.DB, tableName string, _ *zap.Logger) (storageCore.RepositoryStorage, error) {
		require.Same(t, db, gotDB)
		gotTable = tableName
		return expectedRepos, nil
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "severance-table"},
		Logger: zap.NewNop(),
	}
	gotRepos, err := initializeSeveranceStorage(ctx)
	require.NoError(t, err)
	require.Same(t, expectedRepos, gotRepos)
	require.Same(t, db, ctx.DynamoDB)
	require.Same(t, expectedRepos, ctx.Repos)
	require.Equal(t, "us-east-1", gotRegion)
	require.Equal(t, "severance-table", gotTable)
}
