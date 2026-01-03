package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/golang-jwt/jwt/v5"
	dynamock "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type round12FederationService struct {
	deliverErrors    []error
	deliverCalls     int
	followersCalls   int
	recipientsCalls  int
	followersError   error
	recipientsError  error
	lastTargetInbox  string
	lastDeliverActor *activitypub.Actor
	lastDeliverAct   *activitypub.Activity
}

func (s *round12FederationService) DeliverActivity(_ context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	s.lastTargetInbox = targetInbox
	s.lastDeliverActor = signingActor
	s.lastDeliverAct = activity

	idx := s.deliverCalls
	s.deliverCalls++
	if idx < len(s.deliverErrors) {
		return s.deliverErrors[idx]
	}
	return nil
}

func (s *round12FederationService) DeliverToFollowers(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	s.followersCalls++
	return s.followersError
}

func (s *round12FederationService) DeliverToRecipients(_ context.Context, _ *activitypub.Activity, _ *activitypub.Actor) error {
	s.recipientsCalls++
	return s.recipientsError
}

type round12InstanceRepo struct {
	state *models.InstanceState
	err   error
}

func (r *round12InstanceRepo) GetInstanceState(_ context.Context) (*models.InstanceState, error) {
	return r.state, r.err
}

type round12FederationActivityRepo struct {
	created []*models.FederationActivity
	err     error
}

func (r *round12FederationActivityRepo) Create(_ context.Context, activity *models.FederationActivity) error {
	r.created = append(r.created, activity)
	return r.err
}

type round12FederationCostRepo struct {
	recorded []*models.FederationCostTracking
	updates  []round12BudgetUpdate

	checkResult *repositories.BudgetCheckResult
	checkErr    error
}

type round12BudgetUpdate struct {
	domain       string
	period       string
	activityType string
	direction    string
	cost         int64
}

func (r *round12FederationCostRepo) RecordFederationCost(_ context.Context, cost *models.FederationCostTracking) error {
	r.recorded = append(r.recorded, cost)
	return nil
}

func (r *round12FederationCostRepo) UpdateBudgetUsage(_ context.Context, domain, period, activityType, direction string, cost int64) error {
	r.updates = append(r.updates, round12BudgetUpdate{
		domain:       domain,
		period:       period,
		activityType: activityType,
		direction:    direction,
		cost:         cost,
	})
	return nil
}

func (r *round12FederationCostRepo) CheckBudgetLimits(_ context.Context, domain, period, activityType, direction string, estimatedCost int64) (*repositories.BudgetCheckResult, error) {
	if r.checkErr != nil {
		return nil, r.checkErr
	}
	if r.checkResult != nil {
		return r.checkResult, nil
	}
	return &repositories.BudgetCheckResult{Allowed: true, CurrentUsage: 0}, nil
}

func TestExtractDomainFromURL_Round12(t *testing.T) {
	require.Equal(t, "", extractDomainFromURL(""))
	require.Equal(t, "example.com", extractDomainFromURL("example.com"))
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com/inbox"))
	require.Equal(t, "example.com:8443", extractDomainFromURL("https://example.com:8443/inbox"))
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com"))
	require.Equal(t, "example.com", extractDomainFromURL("https://example.com:8443"))
	require.Equal(t, "example.com", extractDomainFromURL("http://example.com/inbox"))
	require.Equal(t, "example.com", extractDomainFromURL("http://example.com"))
	require.Equal(t, "example.com", extractDomainFromURL("http://example.com:8080"))
}

func TestOutboxProcessor_DeliverActivityWithRetry_Round12(t *testing.T) {
	origSleep := outboxSleep
	t.Cleanup(func() { outboxSleep = origSleep })

	sleepCalls := 0
	outboxSleep = func(time.Duration) { sleepCalls++ }

	fedSvc := &round12FederationService{deliverErrors: []error{errors.New("boom")}}
	op := &OutboxProcessor{
		federationService: fedSvc,
		logger:            zap.NewNop(),
		retryConfig: RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  1 * time.Millisecond,
			MaxDelay:      1 * time.Millisecond,
			BackoffFactor: 2,
		},
	}

	msg := ActivityDeliveryMessage{
		Activity:    &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType}},
		Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
		TargetInbox: "https://remote.example/inbox",
	}

	result := op.deliverActivityWithRetry(context.Background(), msg, &federation.CostCalculationParams{})
	require.True(t, result.Success)
	require.Equal(t, 2, result.Attempt)
	require.Equal(t, 2, fedSvc.deliverCalls)
	require.Equal(t, 1, sleepCalls)

	fedSvc = &round12FederationService{deliverErrors: []error{errors.New("boom"), errors.New("boom"), errors.New("boom")}}
	op.federationService = fedSvc
	result = op.deliverActivityWithRetry(context.Background(), msg, &federation.CostCalculationParams{})
	require.False(t, result.Success)
	require.Equal(t, 3, result.Attempt)
	require.Equal(t, 3, fedSvc.deliverCalls)
}

func TestOutboxProcessor_RecordComprehensiveCostTracking_Round12(t *testing.T) {
	origAsync := outboxRunAsync
	t.Cleanup(func() { outboxRunAsync = origAsync })
	outboxRunAsync = func(fn func()) { fn() }

	costRepo := &round12FederationCostRepo{}
	op := &OutboxProcessor{
		federationCostRepository: costRepo,
		costCalculator:           federation.NewCostCalculator(),
		logger:                   zap.NewNop(),
	}

	params := &federation.CostCalculationParams{
		ActivityID:   "https://example.com/activities/1",
		Domain:       "remote.example",
		ActivityType: activitypub.CreateType,
		Direction:    "outbound",
		Timestamp:    time.Now(),
	}

	op.recordComprehensiveCostTracking(ActivityDeliveryMessage{
		Activity:    &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: params.ActivityID, Type: params.ActivityType}},
		TargetInbox: "https://remote.example/inbox",
	}, DeliveryResult{Success: true, Attempt: 1, Duration: 10 * time.Millisecond}, params, 20*time.Millisecond)

	require.Len(t, costRepo.recorded, 1)
	require.Len(t, costRepo.updates, 1)
	require.Equal(t, "remote.example", costRepo.recorded[0].Domain)
}

func TestOutboxProcessor_TrackDeliveryStatus_Round12(t *testing.T) {
	repo := &round12FederationActivityRepo{}
	op := &OutboxProcessor{
		federationActivityRepository: repo,
		logger:                       zap.NewNop(),
	}

	err := op.trackDeliveryStatus(context.Background(), ActivityDeliveryMessage{
		Activity:    &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType}},
		TargetInbox: "https://remote.example/inbox",
	}, DeliveryResult{Success: true, StatusCode: 200, Duration: 5 * time.Millisecond, Attempt: 1})
	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, "remote.example", repo.created[0].Domain)
	require.True(t, repo.created[0].Success)
}

func TestOutboxProcessor_ProcessMessage_Round12(t *testing.T) {
	origAsync := outboxRunAsync
	origSleep := outboxSleep
	t.Cleanup(func() {
		outboxRunAsync = origAsync
		outboxSleep = origSleep
	})
	outboxRunAsync = func(fn func()) { fn() }
	outboxSleep = func(time.Duration) {}

	req := lift.NewRequest(&adapters.Request{
		Method:      "POST",
		Path:        "/",
		TriggerType: lift.TriggerAPIGatewayV2,
	})
	ctx := lift.NewContext(context.Background(), req)

	buildProcessor := func() (*OutboxProcessor, *round12FederationService, *round12FederationActivityRepo, *round12FederationCostRepo) {
		fedSvc := &round12FederationService{}
		fedActivityRepo := &round12FederationActivityRepo{}
		fedCostRepo := &round12FederationCostRepo{}
		return &OutboxProcessor{
			federationService:            fedSvc,
			federationActivityRepository: fedActivityRepo,
			federationCostRepository:     fedCostRepo,
			costCalculator:               federation.NewCostCalculator(),
			logger:                       zap.NewNop(),
			retryConfig: RetryConfig{
				MaxAttempts:   1,
				InitialDelay:  0,
				MaxDelay:      0,
				BackoffFactor: 1,
			},
		}, fedSvc, fedActivityRepo, fedCostRepo
	}

	t.Run("invalid_json", func(t *testing.T) {
		op, _, _, _ := buildProcessor()
		err := op.processMessage(ctx, events.SQSMessage{MessageId: "msg-1", Body: "not-json"})
		require.Error(t, err)
	})

	t.Run("missing_activity", func(t *testing.T) {
		op, _, _, _ := buildProcessor()
		body, err := json.Marshal(ActivityDeliveryMessage{
			Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
			TargetInbox: "https://remote.example/inbox",
		})
		require.NoError(t, err)
		err = op.processMessage(ctx, events.SQSMessage{MessageId: "msg-1", Body: string(body)})
		require.Error(t, err)
	})

	t.Run("budget_blocked", func(t *testing.T) {
		op, fedSvc, _, fedCostRepo := buildProcessor()
		fedCostRepo.checkResult = &repositories.BudgetCheckResult{
			Allowed: false,
			Message: "over limit",
		}
		body, err := json.Marshal(ActivityDeliveryMessage{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType},
				Actor:      "https://example.com/users/alice",
				Object:     map[string]any{"type": "Note"},
			},
			Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
			TargetInbox: "https://remote.example/inbox",
			Attempt:     1,
		})
		require.NoError(t, err)
		err = op.processMessage(ctx, events.SQSMessage{MessageId: "msg-1", Body: string(body)})
		require.Error(t, err)
		require.Equal(t, 0, fedSvc.deliverCalls)
		require.Len(t, fedCostRepo.recorded, 1)
	})

	t.Run("delivery_success", func(t *testing.T) {
		op, fedSvc, fedActivityRepo, fedCostRepo := buildProcessor()
		fedSvc.deliverErrors = nil
		body, err := json.Marshal(ActivityDeliveryMessage{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType},
				Actor:      "https://example.com/users/alice",
				Object:     map[string]any{"type": "Note"},
			},
			Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
			TargetInbox: "https://remote.example/inbox",
			Attempt:     1,
		})
		require.NoError(t, err)
		err = op.processMessage(ctx, events.SQSMessage{MessageId: "msg-1", Body: string(body)})
		require.NoError(t, err)
		require.Equal(t, 1, fedSvc.deliverCalls)
		require.Len(t, fedActivityRepo.created, 1)
		require.Len(t, fedCostRepo.recorded, 1)
		require.Len(t, fedCostRepo.updates, 1)
	})

	t.Run("retryable_failure", func(t *testing.T) {
		op, fedSvc, _, fedCostRepo := buildProcessor()
		fedCostRepo.checkErr = errors.New("budget check failed")
		fedSvc.deliverErrors = []error{errors.New("delivery failed")}

		body, err := json.Marshal(ActivityDeliveryMessage{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType},
				Actor:      "https://example.com/users/alice",
				Object:     map[string]any{"type": "Note"},
			},
			Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
			TargetInbox: "https://remote.example/inbox",
			Attempt:     1,
		})
		require.NoError(t, err)
		err = op.processMessage(ctx, events.SQSMessage{MessageId: "msg-1", Body: string(body)})
		require.Error(t, err)
		require.Equal(t, 1, fedSvc.deliverCalls)
	})
}

func TestOutboxProcessor_HandleSQS_Round12(t *testing.T) {
	origAsync := outboxRunAsync
	origSleep := outboxSleep
	t.Cleanup(func() {
		outboxRunAsync = origAsync
		outboxSleep = origSleep
	})
	outboxRunAsync = func(fn func()) { fn() }
	outboxSleep = func(time.Duration) {}

	req := lift.NewRequest(&adapters.Request{
		Method:      "POST",
		Path:        "/",
		TriggerType: lift.TriggerAPIGatewayV2,
	})
	ctx := lift.NewContext(context.Background(), req)

	op := &OutboxProcessor{
		federationService:            &round12FederationService{},
		federationActivityRepository: &round12FederationActivityRepo{},
		federationCostRepository:     &round12FederationCostRepo{},
		costCalculator:               federation.NewCostCalculator(),
		logger:                       zap.NewNop(),
		retryConfig: RetryConfig{
			MaxAttempts:   1,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 1,
		},
	}

	body, err := json.Marshal(ActivityDeliveryMessage{
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType},
			Actor:      "https://example.com/users/alice",
			Object:     map[string]any{"type": "Note"},
		},
		Actor:       &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}},
		TargetInbox: "https://remote.example/inbox",
		Attempt:     1,
	})
	require.NoError(t, err)

	require.NoError(t, op.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "msg-1", Body: string(body)},
	}}))

	err = op.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "msg-1", Body: "not-json"},
	}})
	require.Error(t, err)
}

func TestOutboxErrors_Round12(t *testing.T) {
	require.NotNil(t, repositoryStorageFromContextFailed())
	require.NotNil(t, federationServiceFromContextFailed())
	require.NotNil(t, costCalculatorFromContextFailed())
	require.NotNil(t, outboxProcessorInitializationFailed())
	require.NotNil(t, missingActivityInMessage())
	require.NotNil(t, missingActorInMessage())
	require.NotNil(t, missingTargetInbox())
	require.NotNil(t, invalidToken())
	require.NotNil(t, unexpectedJWTSigningMethod())
	require.NotNil(t, deliveryBudgetLimitExceeded())
	require.NotNil(t, deliveryRetryableFailure(errors.New("boom")))
	require.NotNil(t, lambdaServicesInitializationFailed())
	require.NotNil(t, invalidMessageFormat())
	require.NotNil(t, federationDeliveryStatusRecordFailed())
	require.NotNil(t, jwtTokenParsingFailed(errors.New("boom")))
}

func TestNewOutboxProcessor_Round12(t *testing.T) {
	// In unit tests, the Lambda init helpers may not be fully wired; we still
	// want to execute the initialization path for coverage.
	proc, err := NewOutboxProcessor()
	require.Error(t, err)
	require.Nil(t, proc)
}

func TestOutboxHelpers_Round12(t *testing.T) {
	op := &OutboxProcessor{
		retryConfig: RetryConfig{PermanentErrors: []int{400, 404}},
	}
	require.True(t, op.isPermanentError(400))
	require.False(t, op.isPermanentError(500))

	require.True(t, op.isPublicOrUnlisted(&activitypub.Activity{BaseObject: activitypub.BaseObject{
		To: []string{activitypub.PublicAddress},
	}}))
	require.True(t, op.isPublicOrUnlisted(&activitypub.Activity{BaseObject: activitypub.BaseObject{
		CC: []string{activitypub.PublicAddress},
	}}))
	require.False(t, op.isPublicOrUnlisted(&activitypub.Activity{}))

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.DefaultContext,
			ID:      "https://example.com/activities/1",
			Type:    activitypub.CreateType,
			To:      []string{activitypub.PublicAddress},
			CC:      []string{"https://remote.example/users/dave"},
			BTo:     []string{"https://remote.example/users/bob"},
			BCC:     []string{"https://remote.example/users/carol"},
		},
		Actor:  "https://example.com/users/alice",
		Object: map[string]any{"type": "Note"},
	}
	payload := outboxActivityValidationMap(activity)
	require.Equal(t, "https://example.com/activities/1", payload["id"])
	require.NotEmpty(t, payload["bto"])
	require.NotEmpty(t, payload["bcc"])
}

func TestOutboxProcessor_TriggerFederationDelivery_Round12(t *testing.T) {
	fedSvc := &round12FederationService{}
	op := &OutboxProcessor{
		federationService: fedSvc,
		logger:            zap.NewNop(),
	}

	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}

	require.NoError(t, op.triggerFederationDelivery(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.FollowType},
	}, actor))
	require.Equal(t, 1, fedSvc.recipientsCalls)

	require.NoError(t, op.triggerFederationDelivery(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AcceptType},
	}, actor))
	require.Equal(t, 2, fedSvc.recipientsCalls)

	require.NoError(t, op.triggerFederationDelivery(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.DeleteType, To: []string{activitypub.PublicAddress}},
	}, actor))
	require.Equal(t, 1, fedSvc.followersCalls)
	require.Equal(t, 3, fedSvc.recipientsCalls)

	require.NoError(t, op.triggerFederationDelivery(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: "Unknown"},
	}, actor))
}

func TestOutboxApp_HTTPHandlers_ErrorBranches_Round12(t *testing.T) {
	origStart := startLambda
	origNew := newOutboxProc
	t.Cleanup(func() {
		startLambda = origStart
		newOutboxProc = origNew
	})

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()

	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: true}}
	fedSvc := &round12FederationService{recipientsError: errors.New("delivery failed")}

	lambdaCtx := &common.LambdaContext{
		Logger:    zap.NewNop(),
		Config:    &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		StartTime: time.Now(),
	}

	processor := &OutboxProcessor{
		federationService:            fedSvc,
		db:                           mockDB,
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		instanceRepository:           instanceRepo,
		federationActivityRepository: &round12FederationActivityRepo{},
		federationCostRepository:     &round12FederationCostRepo{},
		logger:                       zap.NewNop(),
		costCalculator:               federation.NewCostCalculator(),
		retryConfig: RetryConfig{
			MaxAttempts:   1,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 1,
		},
		lambdaCtx: lambdaCtx,
	}

	actorRepo.On("GetActorByUsername", mock.Anything, "bootstrap").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/bootstrap",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Maybe()
	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Maybe()

	newOutboxProc = func() (*OutboxProcessor, error) { return processor, nil }

	var captured func(context.Context, interface{}) (interface{}, error)
	startLambda = func(handler interface{}) {
		if h, ok := handler.(func(context.Context, interface{}) (interface{}, error)); ok {
			captured = h
		}
	}

	main()
	require.NotNil(t, captured)

	call := func(method, path, rawQuery string, headers map[string]any, body string) *lift.Response {
		if headers == nil {
			headers = map[string]any{}
		}
		event := map[string]any{
			"version":        "2.0",
			"routeKey":       method + " " + path,
			"rawPath":        path,
			"rawQueryString": rawQuery,
			"headers":        headers,
			"requestContext": map[string]any{
				"requestId": "test-request-id",
				"http": map[string]any{
					"method": method,
					"path":   path,
				},
			},
			"body":            body,
			"isBase64Encoded": false,
		}
		result, err := captured(context.Background(), event)
		require.NoError(t, err)
		resp, ok := result.(*lift.Response)
		require.True(t, ok)
		return resp
	}

	// Locked bootstrap actor is forbidden without hitting actor lookup.
	resp := call("GET", "/users/bootstrap/outbox", "", nil, "")
	require.Equal(t, 403, resp.StatusCode)

	// Locked non-bootstrap actor returns an empty collection/page but still checks actor existence.
	actorRepo.On("GetActorByUsername", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Once()
	resp = call("GET", "/users/alice/outbox", "", nil, "")
	require.Equal(t, 200, resp.StatusCode)

	// Actor not found (unlocked).
	instanceRepo.state = &models.InstanceState{Locked: false}
	actorRepo.On("GetActorByUsername", mock.Anything, "missing").Return(nil, common.ActorNotFoundError{Username: "missing"}).Once()
	resp = call("GET", "/users/missing/outbox", "", nil, "")
	require.Equal(t, 404, resp.StatusCode)

	// POST blocked while locked.
	instanceRepo.state = &models.InstanceState{Locked: true}
	resp = call("POST", "/users/alice/outbox", "", nil, `{"type":"Create","object":{"type":"Note"}}`)
	require.Equal(t, 403, resp.StatusCode)

	// POST with invalid token.
	instanceRepo.state = &models.InstanceState{Locked: false}
	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer bad.token.value",
	}, `{"type":"Create","object":{"type":"Note"}}`)
	require.Equal(t, 401, resp.StatusCode)

	// POST with insufficient scope.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeRead},
	})
	signed, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer " + signed,
		"content-type":  "application/json",
	}, `{"type":"Create","object":{"type":"Note"}}`)
	require.Equal(t, 403, resp.StatusCode)

	// POST invalid JSON.
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err = token.SignedString([]byte("secret"))
	require.NoError(t, err)
	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer " + signed,
		"content-type":  "application/json",
	}, `{`)
	require.Equal(t, 400, resp.StatusCode)

	// POST unsupported activity type.
	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer " + signed,
		"content-type":  "application/json",
	}, `{"type":"Nope","object":{"type":"Note"}}`)
	require.Equal(t, 422, resp.StatusCode)

	// POST invalid ActivityPub format (missing object).
	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer " + signed,
		"content-type":  "application/json",
	}, `{"type":"Create"}`)
	require.Equal(t, 422, resp.StatusCode)
}

func TestOutboxApp_HTTPHandlers_Round12(t *testing.T) {
	origStart := startLambda
	origNew := newOutboxProc
	origAsync := outboxRunAsync
	t.Cleanup(func() {
		startLambda = origStart
		newOutboxProc = origNew
		outboxRunAsync = origAsync
	})

	outboxRunAsync = func(fn func()) { fn() }

	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Count").Return(int64(3), nil).Maybe()

	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()

	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: false}}
	fedSvc := &round12FederationService{}
	fedActivityRepo := &round12FederationActivityRepo{}
	fedCostRepo := &round12FederationCostRepo{}

	lambdaCtx := &common.LambdaContext{
		Logger:    zap.NewNop(),
		Config:    &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		StartTime: time.Now(),
	}

	processor := &OutboxProcessor{
		federationService:            fedSvc,
		db:                           mockDB,
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		instanceRepository:           instanceRepo,
		federationActivityRepository: fedActivityRepo,
		federationCostRepository:     fedCostRepo,
		logger:                       zap.NewNop(),
		costCalculator:               federation.NewCostCalculator(),
		retryConfig: RetryConfig{
			MaxAttempts:   1,
			InitialDelay:  0,
			MaxDelay:      0,
			BackoffFactor: 1,
		},
		lambdaCtx: lambdaCtx,
	}

	actorRepo.On("GetActorByUsername", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
		Outbox: "",
	}, nil).Maybe()
	activityRepo.On("GetOutboxActivities", mock.Anything, "alice", mock.Anything, mock.Anything).Return([]*activitypub.Activity{
		{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType}},
	}, "next", nil).Maybe()

	newOutboxProc = func() (*OutboxProcessor, error) { return processor, nil }

	var captured func(context.Context, interface{}) (interface{}, error)
	startLambda = func(handler interface{}) {
		if h, ok := handler.(func(context.Context, interface{}) (interface{}, error)); ok {
			captured = h
		}
	}

	main()
	require.NotNil(t, captured)

	call := func(method, path, rawQuery string, headers map[string]any, body string) *lift.Response {
		if headers == nil {
			headers = map[string]any{}
		}
		event := map[string]any{
			"version":        "2.0",
			"routeKey":       method + " " + path,
			"rawPath":        path,
			"rawQueryString": rawQuery,
			"headers":        headers,
			"requestContext": map[string]any{
				"requestId": "test-request-id",
				"http": map[string]any{
					"method": method,
					"path":   path,
				},
			},
			"body":            body,
			"isBase64Encoded": false,
		}
		result, err := captured(context.Background(), event)
		require.NoError(t, err)
		resp, ok := result.(*lift.Response)
		require.True(t, ok)
		return resp
	}

	// GET collection metadata (counts via DB)
	resp := call("GET", "/users/alice/outbox", "", nil, "")
	require.Equal(t, 200, resp.StatusCode)

	// GET page=true (activities via repository)
	resp = call("GET", "/users/alice/outbox", "page=true&limit=1&cursor=prev", nil, "")
	require.Equal(t, 200, resp.StatusCode)

	// POST: unauthenticated
	resp = call("POST", "/users/alice/outbox", "", map[string]any{"content-type": "application/json"}, `{"type":"Create","object":{}}`)
	require.Equal(t, 401, resp.StatusCode)

	// POST: happy path
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	parsed, err := processor.validateJWTToken(signed)
	require.NoError(t, err)
	require.Equal(t, "alice", parsed.Username)

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Once()
	activityRepo.On("CreateActivity", mock.Anything, mock.Anything).Return(nil).Once()

	resp = call("POST", "/users/alice/outbox", "", map[string]any{
		"authorization": "Bearer " + signed,
		"content-type":  "application/json",
	}, `{"type":"Create","to":["https://www.w3.org/ns/activitystreams#Public"],"object":{"type":"Note"}}`)
	require.Equalf(t, 201, resp.StatusCode, "body=%v headers=%v", resp.Body, resp.Headers)

	var created activitypub.Activity
	bodyBytes, err := json.Marshal(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bodyBytes, &created))
	require.Equal(t, activitypub.CreateType, created.Type)
	require.Equal(t, "https://example.com/users/alice", created.Actor)
	require.NotEmpty(t, created.ID)
}

func TestOutboxProcessor_DeliverToFollowersAndRecipients_Round12(t *testing.T) {
	op := &OutboxProcessor{
		logger: zap.NewNop(),
	}

	publicActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/1",
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
		},
	}
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}

	fedSvc := &round12FederationService{followersError: errors.New("followers-down")}
	op.federationService = fedSvc
	require.NoError(t, op.deliverToFollowersAndRecipients(context.Background(), publicActivity, actor))
	require.Equal(t, 1, fedSvc.followersCalls)
	require.Equal(t, 1, fedSvc.recipientsCalls)

	nonPublicActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/2", Type: activitypub.CreateType},
	}
	fedSvc = &round12FederationService{}
	op.federationService = fedSvc
	require.NoError(t, op.deliverToFollowersAndRecipients(context.Background(), nonPublicActivity, actor))
	require.Equal(t, 0, fedSvc.followersCalls)
	require.Equal(t, 1, fedSvc.recipientsCalls)

	fedSvc = &round12FederationService{recipientsError: errors.New("recipients-down")}
	op.federationService = fedSvc
	require.Error(t, op.deliverToFollowersAndRecipients(context.Background(), publicActivity, actor))
	require.Equal(t, 1, fedSvc.followersCalls)
	require.Equal(t, 1, fedSvc.recipientsCalls)
}

func TestOutboxProcessor_TrackDeliveryStatus_ErrorBranches_Round12(t *testing.T) {
	repo := &round12FederationActivityRepo{}
	op := &OutboxProcessor{
		federationActivityRepository: repo,
		logger:                       zap.NewNop(),
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/1", Type: activitypub.CreateType}}
	activity.Object = make(chan int)

	require.NoError(t, op.trackDeliveryStatus(context.Background(), ActivityDeliveryMessage{
		Activity:    activity,
		TargetInbox: "https://remote.example/inbox",
	}, DeliveryResult{Success: true, StatusCode: 200, Duration: 5 * time.Millisecond, Attempt: 1}))
	require.Len(t, repo.created, 1)
	require.Equal(t, int64(len(activity.ID)), repo.created[0].OutboundSize)

	repo = &round12FederationActivityRepo{err: errors.New("db-down")}
	op.federationActivityRepository = repo
	require.Error(t, op.trackDeliveryStatus(context.Background(), ActivityDeliveryMessage{
		Activity:    &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://example.com/activities/2", Type: activitypub.CreateType}},
		TargetInbox: "https://remote.example/inbox",
	}, DeliveryResult{Success: false, StatusCode: 500, Duration: 10 * time.Millisecond, Attempt: 1, Error: errors.New("delivery failed")}))
}

func TestBuildOutboxApp_SQSHandler_ParseBranches_Round12(t *testing.T) {
	processor := &OutboxProcessor{
		logger:    zap.NewNop(),
		lambdaCtx: &common.LambdaContext{Logger: zap.NewNop()},
	}
	app := buildOutboxApp(processor)

	makeCtx := func(rawEvent any) *lift.Context {
		req := lift.NewRequest(&adapters.Request{
			TriggerType: lift.TriggerSQS,
			RawEvent:    rawEvent,
			Records: []any{
				map[string]any{
					"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:outbox-delivery",
				},
			},
		})
		return lift.NewContext(context.Background(), req)
	}

	call := func(ctx *lift.Context) error {
		h, err := app.GetEventRouter().FindEventHandler(ctx)
		require.NoError(t, err)
		return h.HandleEvent(ctx)
	}

	require.Error(t, call(makeCtx(nil)))
	require.NoError(t, call(makeCtx(events.SQSEvent{})))
	require.NoError(t, call(makeCtx(map[string]any{"Records": []any{}})))
	require.Error(t, call(makeCtx(map[string]any{"Records": "not-a-slice"})))
	require.Error(t, call(makeCtx(map[string]any{
		"Records": []any{
			map[string]any{
				"eventSource":    "aws:sqs",
				"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:outbox-delivery",
			},
		},
		"bad": make(chan int),
	})))
}

func TestOutboxMain_PanicsWhenInitializationFails_Round12(t *testing.T) {
	origProc := newOutboxProc
	origStart := startLambda
	t.Cleanup(func() {
		newOutboxProc = origProc
		startLambda = origStart
	})

	newOutboxProc = func() (*OutboxProcessor, error) { return nil, errors.New("boom") }
	startLambda = func(any) {}

	require.Panics(t, main)
}

func TestOutboxProcessor_HandleOutboxGet_DirectErrorBranches_Round12(t *testing.T) {
	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: false}}

	op := &OutboxProcessor{
		actorRepository:    actorRepo,
		activityRepository: activityRepo,
		instanceRepository: instanceRepo,
		logger:             zap.NewNop(),
		lambdaCtx: &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		},
	}

	// Missing username param.
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 400, ctx.Response.StatusCode)

	// Actor lookup failure (non-not-found) returns 500.
	actorRepo.On("GetActorByUsername", mock.Anything, "boom").Return(nil, errors.New("db down")).Once()
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.SetParam("username", "boom")
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 500, ctx.Response.StatusCode)

	// Locked page=true response when instance state cannot be loaded.
	instanceRepo.err = errors.New("no state")
	actorRepo.On("GetActorByUsername", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
		Outbox: "https://example.com/users/alice/outbox",
	}, nil).Once()
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		QueryParams: map[string]string{
			"page":  "true",
			"limit": "0",
		},
	}))
	ctx.SetParam("username", "alice")
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)
	_, ok := ctx.Response.Body.(*activitypub.OrderedCollectionPage)
	require.True(t, ok)
}

func TestOutboxProcessor_HandleOutboxGet_CountAndActivitiesErrors_Round12(t *testing.T) {
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Count").Return(int64(0), errors.New("count failed")).Maybe()

	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: false}}

	op := &OutboxProcessor{
		db:                 mockDB,
		actorRepository:    actorRepo,
		activityRepository: activityRepo,
		instanceRepository: instanceRepo,
		logger:             zap.NewNop(),
		lambdaCtx: &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		},
	}

	actorRepo.On("GetActorByUsername", mock.Anything, "count").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/count",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Once()

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		QueryParams: map[string]string{
			"limit": "0",
		},
	}))
	ctx.SetParam("username", "count")
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 200, ctx.Response.StatusCode)
	body, ok := ctx.Response.Body.(*activitypub.OrderedCollection)
	require.True(t, ok)
	require.Equal(t, 0, body.TotalItems)

	actorRepo.On("GetActorByUsername", mock.Anything, "bob").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/bob",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
		Outbox: "https://example.com/custom/outbox",
	}, nil).Once()
	activityRepo.On("GetOutboxActivities", mock.Anything, "bob", mock.Anything, mock.Anything).Return(nil, "", errors.New("repo down")).Once()

	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		QueryParams: map[string]string{
			"page": "true",
		},
	}))
	ctx.SetParam("username", "bob")
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 500, ctx.Response.StatusCode)
}

func TestOutboxProcessor_HandleOutboxPost_CreateActivityFailure_Round12(t *testing.T) {
	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: false}}

	lambdaCtx := &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
	}

	op := &OutboxProcessor{
		federationService:            &round12FederationService{},
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		instanceRepository:           instanceRepo,
		logger:                       zap.NewNop(),
		lambdaCtx:                    lambdaCtx,
		costCalculator:               federation.NewCostCalculator(),
		retryConfig:                  RetryConfig{MaxAttempts: 1},
		federationCostRepository:     &round12FederationCostRepo{},
		federationActivityRepository: &round12FederationActivityRepo{},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Once()
	activityRepo.On("CreateActivity", mock.Anything, mock.Anything).Return(errors.New("write failed")).Once()

	req := lift.NewRequest(&adapters.Request{
		Method: "POST",
		Headers: map[string]string{
			"Authorization": "Bearer " + signed,
			"Content-Type":  "application/json",
		},
		Body: []byte(`{"type":"Create","to":["https://www.w3.org/ns/activitystreams#Public"],"object":{"type":"Note"}}`),
	})
	ctx := lift.NewContext(context.Background(), req)
	ctx.SetParam("username", "alice")

	require.NoError(t, op.HandleOutboxPost(ctx))
	require.Equal(t, 500, ctx.Response.StatusCode)
}

func TestOutboxProcessor_HandleOutboxPost_TriggerFederationDeliveryFailure_Round12(t *testing.T) {
	actorRepo := testmocks.NewMockActorRepository()
	activityRepo := testmocks.NewMockActivityRepository()
	instanceRepo := &round12InstanceRepo{state: &models.InstanceState{Locked: false}}

	lambdaCtx := &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
	}

	op := &OutboxProcessor{
		federationService:            &round12FederationService{recipientsError: errors.New("delivery failed")},
		actorRepository:              actorRepo,
		activityRepository:           activityRepo,
		instanceRepository:           instanceRepo,
		logger:                       zap.NewNop(),
		lambdaCtx:                    lambdaCtx,
		costCalculator:               federation.NewCostCalculator(),
		retryConfig:                  RetryConfig{MaxAttempts: 1},
		federationCostRepository:     &round12FederationCostRepo{},
		federationActivityRepository: &round12FederationActivityRepo{},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:      "https://example.com/users/alice",
			Type:    activitypub.PersonType,
			Context: activitypub.DefaultContext,
		},
	}, nil).Once()
	activityRepo.On("CreateActivity", mock.Anything, mock.Anything).Return(nil).Once()

	req := lift.NewRequest(&adapters.Request{
		Method: "POST",
		Headers: map[string]string{
			"Authorization": "Bearer " + signed,
			"Content-Type":  "application/json",
		},
		Body: []byte(`{"type":"Create","to":["https://www.w3.org/ns/activitystreams#Public"],"object":{"type":"Note"}}`),
	})
	ctx := lift.NewContext(context.Background(), req)
	ctx.SetParam("username", "alice")

	require.NoError(t, op.HandleOutboxPost(ctx))
	require.Equal(t, 201, ctx.Response.StatusCode)
}

func TestOutboxProcessor_AuthenticateOutboxRequest_MismatchAndActorError_Round12(t *testing.T) {
	actorRepo := testmocks.NewMockActorRepository()

	lambdaCtx := &common.LambdaContext{
		Logger: zap.NewNop(),
		Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
	}

	op := &OutboxProcessor{
		actorRepository: actorRepo,
		logger:          zap.NewNop(),
		lambdaCtx:       lambdaCtx,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)

	makeCtx := func() *lift.Context {
		req := lift.NewRequest(&adapters.Request{
			Headers: map[string]string{
				"Authorization": "Bearer " + signed,
				"Content-Type":  "application/json",
			},
		})
		return lift.NewContext(context.Background(), req)
	}

	_, _, err = op.authenticateOutboxRequest(makeCtx(), "bob")
	require.Error(t, err)

	actorRepo.On("GetActor", mock.Anything, "alice").Return(nil, errors.New("db down")).Once()
	_, _, err = op.authenticateOutboxRequest(makeCtx(), "alice")
	require.Error(t, err)
}

func TestOutboxProcessor_ValidateJWTToken_UnexpectedSigningMethod_Round12(t *testing.T) {
	op := &OutboxProcessor{
		logger: zap.NewNop(),
		lambdaCtx: &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodNone, &auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeWrite},
	})
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = op.validateJWTToken(signed)
	require.Error(t, err)
}

func TestOutboxActivityValidationMap_OmitsNilObject_Round12(t *testing.T) {
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/1",
			Type: activitypub.CreateType,
		},
		Actor:  "https://example.com/users/alice",
		Object: nil,
	}
	payload := outboxActivityValidationMap(activity)
	_, has := payload["object"]
	require.False(t, has)
}

func TestOutboxProcessor_GetBearerToken_InvalidHeader_Round12(t *testing.T) {
	op := &OutboxProcessor{}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Headers: map[string]string{
			"Authorization": "Token abc123",
		},
	}))

	require.Equal(t, "", op.getBearerToken(ctx))
}

func TestOutboxProcessor_HandleOutboxGet_BootstrapUsernameOverride_Round12(t *testing.T) {
	instanceRepo := &round12InstanceRepo{
		state: &models.InstanceState{
			Locked:            true,
			BootstrapUsername: "Admin",
		},
	}

	op := &OutboxProcessor{
		instanceRepository: instanceRepo,
		logger:             zap.NewNop(),
		lambdaCtx: &common.LambdaContext{
			Logger: zap.NewNop(),
			Config: &config.Config{Domain: "example.com", Region: "us-east-1", JWTSecret: "secret"},
		},
	}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.SetParam("username", "admin")
	require.NoError(t, op.HandleOutboxGet(ctx))
	require.Equal(t, 403, ctx.Response.StatusCode)
}
