package routing

import (
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type fakeTimer struct {
	finished       bool
	finishedStatus bool
	errorType      string
}

func (f *fakeTimer) Finish(_ interface{}, success bool) {
	f.finished = true
	f.finishedStatus = success
}

func (f *fakeTimer) FinishWithError(_ interface{}, errorType string) {
	f.errorType = errorType
}

func TestInboxHandler_Round10_IsAddressedTo_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	tests := []struct {
		name     string
		activity *activitypub.Activity
		want     bool
	}{
		{name: "to actor", activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{To: []string{env.local.ID}}}, want: true},
		{name: "cc inbox", activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{CC: []string{env.local.Inbox}}}, want: true},
		{name: "bto actor", activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{BTo: []string{env.local.ID}}}, want: true},
		{name: "bcc inbox", activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{BCC: []string{env.local.Inbox}}}, want: true},
		{name: "public to", activity: &activitypub.Activity{BaseObject: activitypub.BaseObject{To: []string{activitypub.PublicAddress}}}, want: true},
		{name: "not addressed", activity: &activitypub.Activity{}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, env.handler.isAddressedTo(tc.activity, env.local))
		})
	}
}

func TestInboxHandler_Round10_ExtractFlagReason_Branches(t *testing.T) {
	env := newInboxTestEnv(t)

	require.Equal(t, "summary", env.handler.extractFlagReason(&activitypub.Activity{BaseObject: activitypub.BaseObject{Summary: "summary"}}))
	require.Equal(t, "content", env.handler.extractFlagReason(&activitypub.Activity{Object: map[string]any{"content": "content"}}))
	require.Equal(t, "No reason provided", env.handler.extractFlagReason(&activitypub.Activity{Object: "x"}))
}

func TestInboxHandler_Round10_MetricsHelperCoverage(t *testing.T) {
	env := newInboxTestEnv(t)

	timer := &fakeTimer{}
	env.handler.recordSuccessfulDelivery(timer, "remote.example")
	require.True(t, timer.finished)

	timer2 := &fakeTimer{}
	env.handler.recordFailedDelivery(timer2, "remote.example", 401)
	require.Equal(t, observability.ErrorTypeAuthentication, env.handler.categorizeErrorType(401))
	require.Equal(t, observability.ErrorTypeRateLimit, env.handler.categorizeErrorType(429))
	require.Equal(t, observability.ErrorTypeValidation, env.handler.categorizeErrorType(404))
	require.Equal(t, observability.ErrorTypeFederation, env.handler.categorizeErrorType(500))

	env.handler.recordSignatureVerificationMetrics("remote.example", true)
	env.handler.recordSignatureVerificationMetrics("remote.example", false)
}

func TestInboxHandler_Round10_AddressingAndPrivacy_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	makeReq := func(activity *activitypub.Activity) *InboxRequest {
		now := time.Now()
		return &InboxRequest{
			Activity:    activity,
			Actor:       env.local,
			ActorDomain: "remote.example",
			StartTime:   now,
			CostParams: &federation.CostCalculationParams{
				ActivityID:    activity.ID,
				Domain:        "remote.example",
				ActivityType:  activity.Type,
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     now,
			},
		}
	}

	t.Run("invalid addressing fields", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: activitypub.CreateType,
				ID:   env.cfg.BaseURL() + "/activities/bad-addressing",
				To:   []string{"https://example.com"},
			},
			Actor: env.remoteActorID,
		}
		require.Error(t, env.handler.validateAddressingAndPrivacy(nil, makeReq(activity)))
	})

	t.Run("privacy violation when BCC appears in visible fields", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: activitypub.CreateType,
				ID:   env.cfg.BaseURL() + "/activities/bad-privacy",
				To:   []string{env.local.ID},
				BCC:  []string{env.local.ID},
			},
			Actor: env.remoteActorID,
		}
		require.Error(t, env.handler.validateAddressingAndPrivacy(nil, makeReq(activity)))
	})

	t.Run("not addressed to actor returns not found", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: activitypub.CreateType,
				ID:   env.cfg.BaseURL() + "/activities/not-addressed",
				To:   []string{"https://remote.example/users/other"},
			},
			Actor: env.remoteActorID,
		}
		require.Error(t, env.handler.validateAddressingAndPrivacy(nil, makeReq(activity)))
	})

	t.Run("direct message validation failure", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: activitypub.CreateType,
				ID:   env.cfg.BaseURL() + "/activities/dm-invalid",
				To:   []string{"https:///users/alice"},
			},
			Actor: env.remoteActorID,
		}
		require.Error(t, env.handler.validateAddressingAndPrivacy(nil, makeReq(activity)))
	})
}

func TestInboxHandler_Round10_VerifyAuthentication_FailureBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	makeReq := func(actorID string, body []byte) *InboxRequest {
		now := time.Now()
		return &InboxRequest{
			Activity:  &activitypub.Activity{Actor: actorID},
			Body:      body,
			StartTime: now,
			CostParams: &federation.CostCalculationParams{
				ActivityID:    "x",
				Domain:        "remote.example",
				ActivityType:  activitypub.CreateType,
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     now,
			},
		}
	}

	body := []byte(`{"@context":"https://www.w3.org/ns/activitystreams","type":"Create","id":"https://example.com/activities/1","actor":"https://remote.example/users/bob","to":["https://example.com/users/alice"],"object":"https://example.com/objects/1"}`)
	headers := map[string]string{
		"Host":         "localhost",
		"Content-Type": "application/activity+json",
	}
	ctx := newAppTheoryContext("POST", "/users/alice/inbox", headers, nil, body)
	ctx.Params["username"] = "alice"

	t.Run("missing signature returns service unavailable", func(t *testing.T) {
		req := makeReq(env.remoteActorID, body)
		require.Error(t, env.handler.verifyAuthentication(ctx, req))
	})

	t.Run("digest mismatch returns validation error", func(t *testing.T) {
		ctx2 := newAppTheoryContext("POST", "/users/alice/inbox", headers, nil, body)
		ctx2.Params["username"] = "alice"

		httpReq, err := env.handler.convertRequest(ctx2, body)
		require.NoError(t, err)
		require.NoError(t, federation.SignHTTPRequestWithAlgorithm(httpReq, env.remotePrivateKey, env.remoteKeyID, federation.AlgorithmRSASHA256))

		ctx2.Request.Headers["date"] = []string{httpReq.Header.Get("Date")}
		ctx2.Request.Headers["signature"] = []string{httpReq.Header.Get("Signature")}
		ctx2.Request.Headers["digest"] = []string{"SHA-256=not-a-valid-digest"}

		req := makeReq(env.remoteActorID, body)
		require.Error(t, env.handler.verifyAuthentication(ctx2, req))
	})
}

func TestInboxHandler_Round10_StoreAndProcessActivity_ErrorBranches(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, []byte(`{}`))
	ctx.Params["username"] = "alice"

	t.Run("activity storage failure", func(t *testing.T) {
		innerDB := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("IfNotExists").Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Limit", mock.Anything).Return(query).Maybe()
		query.On("Create").Return(errors.New("write failed")).Maybe()

		badRepo := repositories.NewActivityRepository(db, env.cfg.DynamoTableName, zap.NewNop(), nil)

		badHandler := *env.handler
		badHandler.activityRepository = badRepo

		now := time.Now()
		req := &InboxRequest{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					Type:    activitypub.CreateType,
					ID:      env.cfg.BaseURL() + "/activities/create-fail",
					To:      []string{env.local.ID},
				},
				Actor:  env.remoteActorID,
				Object: env.cfg.BaseURL() + "/objects/1",
			},
			ActorDomain: "remote.example",
			StartTime:   now,
			CostParams: &federation.CostCalculationParams{
				ActivityID:    "create-fail",
				Domain:        "remote.example",
				ActivityType:  activitypub.CreateType,
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     now,
			},
		}
		require.Error(t, badHandler.storeAndProcessActivity(ctx, req))
	})

	t.Run("activity processing failure", func(t *testing.T) {
		innerDB := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("IfNotExists").Return(query).Maybe()
		query.On("Index", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Limit", mock.Anything).Return(query).Maybe()
		query.On("All", mock.Anything).Return(nil).Maybe()
		query.On("First", mock.AnythingOfType("*models.Block")).Return(dynamormErrors.ErrItemNotFound).Maybe()
		query.On("Create").Return(errors.New("write failed")).Maybe()
		query.On("Update", mock.Anything).Return(nil).Maybe()
		query.On("Delete").Return(nil).Maybe()

		failingRelationshipRepo := repositories.NewRelationshipRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		badHandler := *env.handler
		badHandler.relationshipRepository = failingRelationshipRepo

		now := time.Now()
		req := &InboxRequest{
			Activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					Type:    activitypub.FollowType,
					ID:      env.cfg.BaseURL() + "/activities/follow-fail",
					To:      []string{env.local.ID},
				},
				Actor:  env.remoteActorID,
				Object: env.local.ID,
			},
			Actor:       env.local,
			ActorDomain: "remote.example",
			StartTime:   now,
			CostParams: &federation.CostCalculationParams{
				ActivityID:    "follow-fail",
				Domain:        "remote.example",
				ActivityType:  activitypub.FollowType,
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     now,
			},
		}
		require.Error(t, badHandler.storeAndProcessActivity(ctx, req))
	})
}

func TestInboxHandler_Round10_CheckDomainBlock_Branches(t *testing.T) {
	env := newInboxTestEnv(t)
	setRunAsyncSynchronous(t)

	makeCtx := func() *apptheory.Context {
		ctx := newAppTheoryContext("POST", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, []byte(`{}`))
		ctx.Params["username"] = "alice"
		return ctx
	}

	now := time.Now()
	makeReq := func(domain string) *InboxRequest {
		return &InboxRequest{
			Activity:    &activitypub.Activity{Actor: env.remoteActorID},
			ActorDomain: domain,
			StartTime:   now,
			CostParams:  &federation.CostCalculationParams{ActivityID: "x", Domain: domain, ActivityType: activitypub.CreateType, Timestamp: now},
		}
	}

	t.Run("suspended domain is forbidden", func(t *testing.T) {
		innerDB := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("Index", mock.Anything).Return(query).Maybe()
		query.On("Limit", mock.Anything).Return(query).Maybe()
		query.On("OrderBy", mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Run(func(args mock.Arguments) {
			out := args.Get(0).(*models.InstanceDomainBlock)
			out.Domain = "blocked.example"
			out.Severity = "suspend"
		}).Return(nil).Maybe()

		badRepo := repositories.NewDomainBlockRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		handler := *env.handler
		handler.domainBlockRepository = badRepo

		err := handler.checkDomainBlock(makeCtx(), makeReq("blocked.example"))
		require.Error(t, err)
	})

	t.Run("repository error fails open", func(t *testing.T) {
		innerDB := new(mocks.MockDB)
		query := new(mocks.MockQuery)
		db := &extendedMockDB{inner: innerDB}

		innerDB.On("Model", mock.Anything).Return(query).Maybe()
		query.On("WithContext", mock.Anything).Return(query).Maybe()
		query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
		query.On("First", mock.AnythingOfType("*models.InstanceDomainBlock")).Return(errors.New("boom")).Maybe()

		badRepo := repositories.NewDomainBlockRepository(db, env.cfg.DynamoTableName, zap.NewNop())

		handler := *env.handler
		handler.domainBlockRepository = badRepo

		require.NoError(t, handler.checkDomainBlock(makeCtx(), makeReq("blocked.example")))
	})
}
