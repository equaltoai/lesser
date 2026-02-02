package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeRelayActorRepo struct {
	actor *activitypub.Actor
	err   error

	calls int
}

func (f *fakeRelayActorRepo) GetActorByUsername(_ context.Context, _ string) (*activitypub.Actor, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.actor, nil
}

type fakeRelayRepo struct {
	relays map[string]*storage.RelayInfo

	storeCalls []*storage.RelayInfo
	storeErr   error

	getCalls []string
	getErr   error

	removeCalls []string
	removeErr   error

	activeRelays []*storage.RelayInfo
	activeErr    error
}

func (f *fakeRelayRepo) StoreRelayInfo(_ context.Context, relay *storage.RelayInfo) error {
	f.storeCalls = append(f.storeCalls, relay)
	if f.relays != nil {
		f.relays[relay.URL] = relay
	}
	return f.storeErr
}

func (f *fakeRelayRepo) GetRelayInfo(_ context.Context, relayURL string) (*storage.RelayInfo, error) {
	f.getCalls = append(f.getCalls, relayURL)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.relays == nil {
		return nil, errors.New("not found")
	}
	relay, ok := f.relays[relayURL]
	if !ok {
		return nil, errors.New("not found")
	}
	return relay, nil
}

func (f *fakeRelayRepo) RemoveRelayInfo(_ context.Context, relayURL string) error {
	f.removeCalls = append(f.removeCalls, relayURL)
	if f.relays != nil {
		delete(f.relays, relayURL)
	}
	return f.removeErr
}

func (f *fakeRelayRepo) GetActiveRelays(_ context.Context) ([]*storage.RelayInfo, error) {
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	return f.activeRelays, nil
}

type fakeRelayCostRepo struct {
	createCalls []*models.RelayCost
	createErr   error

	budget      *models.RelayBudget
	getErr      error
	updateErr   error
	updateCalls []*models.RelayBudget
}

func (f *fakeRelayCostRepo) CreateRelayCost(_ context.Context, relayCost *models.RelayCost) error {
	f.createCalls = append(f.createCalls, relayCost)
	return f.createErr
}

func (f *fakeRelayCostRepo) GetRelayBudget(_ context.Context, _ string, _ string) (*models.RelayBudget, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.budget == nil {
		return nil, errors.New("no budget")
	}
	return f.budget, nil
}

func (f *fakeRelayCostRepo) UpdateRelayBudget(_ context.Context, budget *models.RelayBudget) error {
	f.updateCalls = append(f.updateCalls, budget)
	return f.updateErr
}

type fakeRelayDelivery struct {
	errByInbox map[string]error

	calls []struct {
		inbox        string
		activityType string
	}
}

func (f *fakeRelayDelivery) DeliverActivity(_ context.Context, activity *activitypub.Activity, targetInbox string, _ *activitypub.Actor) error {
	f.calls = append(f.calls, struct {
		inbox        string
		activityType string
	}{inbox: targetInbox, activityType: activity.Type})

	if f.errByInbox == nil {
		return nil
	}
	return f.errByInbox[targetInbox]
}

func TestRelayService_SubscribeToRelay_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayInbox := "https://relay.example/inbox"
	actorUsername := "bob"

	relayActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   relayURL,
			Type: "Application",
		},
		Inbox: relayInbox,
	}
	relayActorJSON, err := json.Marshal(relayActor)
	require.NoError(t, err)

	relayRepo := &fakeRelayRepo{relays: map[string]*storage.RelayInfo{}}
	costRepo := &fakeRelayCostRepo{}
	delivery := &fakeRelayDelivery{}

	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID: actorUsername,
				},
				PreferredUsername: actorUsername,
				Followers:         "https://local.example/users/bob/followers",
			},
		},
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, relayURL, req.URL.String())
			assert.Contains(t, req.Header.Get("Accept"), "application/activity+json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(relayActorJSON)),
			}, nil
		}},
		delivery: delivery,
		domain:   "local.example",
	}

	require.NoError(t, svc.SubscribeToRelay(ctx, relayURL, actorUsername))

	require.Len(t, relayRepo.storeCalls, 1)
	assert.Equal(t, relayURL, relayRepo.storeCalls[0].URL)
	assert.Equal(t, relayInbox, relayRepo.storeCalls[0].InboxURL)
	assert.False(t, relayRepo.storeCalls[0].Active)

	require.Len(t, delivery.calls, 1)
	assert.Equal(t, relayInbox, delivery.calls[0].inbox)
	assert.Equal(t, activitypub.FollowType, delivery.calls[0].activityType)

	require.Len(t, costRepo.createCalls, 1)
	assert.Equal(t, "subscription", costRepo.createCalls[0].OperationType)
	assert.True(t, costRepo.createCalls[0].Success)
}

func TestRelayService_SubscribeToRelay_InvalidURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &fakeRelayCostRepo{}
	delivery := &fakeRelayDelivery{}

	svc := &RelayService{
		actorRepo:  &fakeRelayActorRepo{actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "bob"}}},
		relayRepo:  &fakeRelayRepo{},
		costRepo:   costRepo,
		logger:     logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
		delivery:   delivery,
		domain:     "local.example",
	}

	err := svc.SubscribeToRelay(ctx, "://bad", "bob")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRelayURL)

	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
	assert.Contains(t, costRepo.createCalls[0].ErrorMessage, "invalid URL")
}

func TestRelayService_HandleRelayActivity_AcceptActivatesInactiveRelay(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {
				URL:      relayURL,
				InboxURL: "https://relay.example/inbox",
				Active:   false,
			},
		},
	}

	costRepo := &fakeRelayCostRepo{}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	accept := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AcceptType},
	}
	require.NoError(t, svc.HandleRelayActivity(ctx, accept, relayURL))

	// LastSeen update attempt + activation store
	require.GreaterOrEqual(t, len(relayRepo.storeCalls), 1)
	assert.True(t, relayRepo.relays[relayURL].Active)

	require.Len(t, costRepo.createCalls, 1)
	assert.Equal(t, "processing", costRepo.createCalls[0].OperationType)
	assert.True(t, costRepo.createCalls[0].Success)
}

func TestRelayService_HandleRelayActivity_InactiveRelayBlocksAnnounce(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: false},
		},
	}

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	announce := &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType}}
	err := svc.HandleRelayActivity(ctx, announce, relayURL)
	assert.ErrorIs(t, err, ErrUnknownInactiveRelay)

	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_UnsubscribeFromRelay_RemovesLocalEvenIfDeliveryFails(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: true},
		},
	}

	delivery := &fakeRelayDelivery{
		errByInbox: map[string]error{"https://relay.example/inbox": errors.New("delivery failed")},
	}

	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{
			actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"},
			},
		},
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  delivery,
		domain:    "local.example",
	}

	require.NoError(t, svc.UnsubscribeFromRelay(ctx, relayURL, "bob"))
	assert.Empty(t, relayRepo.relays)
	require.Len(t, relayRepo.removeCalls, 1)
	assert.Equal(t, relayURL, relayRepo.removeCalls[0])
}

func TestRelayService_ForwardToRelays_PartialFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relay1 := &storage.RelayInfo{URL: "https://relay1.example/actor", InboxURL: "https://relay1.example/inbox", Active: true, CreatedAt: time.Now(), LastSeenAt: time.Now()}
	relay2 := &storage.RelayInfo{URL: "https://relay2.example/actor", InboxURL: "https://relay2.example/inbox", Active: true, CreatedAt: time.Now(), LastSeenAt: time.Now()}

	relayRepo := &fakeRelayRepo{activeRelays: []*storage.RelayInfo{relay1, relay2}}
	delivery := &fakeRelayDelivery{
		errByInbox: map[string]error{
			relay2.InboxURL: errors.New("deliver failed"),
		},
	}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  delivery,
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/1", Type: activityTypeCreate}}
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}, Followers: "https://local.example/users/bob/followers"}

	err := svc.ForwardToRelays(ctx, activity, actor)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRelayForwardingFailed)

	require.Len(t, delivery.calls, 2)
}

func TestExtractDomainFromRelayURL(t *testing.T) {
	assert.Equal(t, keyTypeUnknown, extractDomainFromRelayURL(""))
	assert.Equal(t, keyTypeUnknown, extractDomainFromRelayURL("not a url"))
	assert.Equal(t, "relay.example", extractDomainFromRelayURL("https://relay.example/actor"))
}

func TestCalculateDataTransferCost(t *testing.T) {
	assert.Equal(t, int64(0), calculateDataTransferCost(1234, "inbound"))
	assert.Greater(t, calculateDataTransferCost(1024*1024*1024, "outbound"), int64(0))
}

func TestRelayService_VerifyFetchedActorType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			body := `{"id":"` + relayURL + `","type":"Person","inbox":"https://relay.example/inbox"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}},
	}

	_, err := svc.fetchRelayActor(ctx, relayURL)
	assert.ErrorIs(t, err, ErrNotRelayActor)
}

func TestRelayService_HandleRelayAnnounce_ObjectConversionErrors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relay := &RelayInfo{URL: "https://relay.example/actor"}

	svc := &RelayService{logger: logger}

	t.Run("invalid_object_type", func(t *testing.T) {
		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType},
			Object:     "bad",
		}
		assert.ErrorIs(t, svc.handleRelayAnnounce(ctx, announce, relay), ErrInvalidAnnouncedObjectType)
	})

	t.Run("marshal_error", func(t *testing.T) {
		announce := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType},
			Object:     map[string]any{"bad": func() {}},
		}
		assert.ErrorIs(t, svc.handleRelayAnnounce(ctx, announce, relay), ErrMarshalAnnouncedObjectFailed)
	})
}

func TestRelayService_HandleRelayActivity_AnnounceMapSuccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: true},
		},
	}

	costRepo := &fakeRelayCostRepo{}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType},
		Object: map[string]any{
			"id":   "https://remote.example/activities/1",
			"type": "Create",
		},
	}
	require.NoError(t, svc.HandleRelayActivity(ctx, announce, relayURL))

	require.Len(t, costRepo.createCalls, 1)
	assert.True(t, costRepo.createCalls[0].Success)
}

func TestRelayService_ForwardToRelays_NoRelays(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		relayRepo: &fakeRelayRepo{activeRelays: []*storage.RelayInfo{}},
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/1", Type: "Create"}}
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}, Followers: "https://local.example/users/bob/followers"}

	require.NoError(t, svc.ForwardToRelays(ctx, activity, actor))
}

func TestRelayService_ForwardToRelays_SkipsWhenBudgetExceeded(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relay := &storage.RelayInfo{URL: "https://relay.example/actor", InboxURL: "https://relay.example/inbox", Active: true, CreatedAt: time.Now(), LastSeenAt: time.Now()}
	relayRepo := &fakeRelayRepo{activeRelays: []*storage.RelayInfo{relay}}
	costRepo := &fakeRelayCostRepo{
		budget: &models.RelayBudget{
			RelayURL:                relay.URL,
			Period:                  "daily",
			LimitMicroCents:         100,
			CurrentUsageMicroCents:  95,
			WarningThresholdPercent: 75.0,
		},
	}
	delivery := &fakeRelayDelivery{}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		delivery:  delivery,
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/1", Type: "Create"}}
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}, Followers: "https://local.example/users/bob/followers"}

	require.NoError(t, svc.ForwardToRelays(ctx, activity, actor))
	assert.Empty(t, delivery.calls)
	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_fetchRelayActor_NonOKStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		}},
	}

	_, err := svc.fetchRelayActor(ctx, "https://relay.example/actor")
	assert.ErrorIs(t, err, ErrFetchRelayActorHTTPFailed)
}

func TestRelayService_fetchRelayActor_ParseError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("{")),
			}, nil
		}},
	}

	_, err := svc.fetchRelayActor(ctx, "https://relay.example/actor")
	require.Error(t, err)
}

func TestRelayService_fetchRelayActor_RequestCreationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		logger:     logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
	}

	_, err := svc.fetchRelayActor(ctx, "http://[::1")
	require.Error(t, err)
}

func TestRelayService_SubscribeToRelay_DeliveryError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayInbox := "https://relay.example/inbox"
	actorUsername := "bob"

	relayActorJSON := []byte(`{"id":"` + relayURL + `","type":"Application","inbox":"` + relayInbox + `"}`)

	costRepo := &fakeRelayCostRepo{}
	delivery := &fakeRelayDelivery{errByInbox: map[string]error{relayInbox: errors.New("boom")}}
	relayRepo := &fakeRelayRepo{relays: map[string]*storage.RelayInfo{}}

	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{
			actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}},
		},
		relayRepo: relayRepo,
		costRepo:  costRepo,
		logger:    logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(relayActorJSON)),
			}, nil
		}},
		delivery: delivery,
		domain:   "local.example",
	}

	err := svc.SubscribeToRelay(ctx, relayURL, actorUsername)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeliverFollowActivityFailed)

	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_UnsubscribeFromRelay_RemoveError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	removeErr := errors.New("remove failed")

	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: true},
		},
		removeErr: removeErr,
	}

	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}}},
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	err := svc.UnsubscribeFromRelay(ctx, relayURL, "bob")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRemoveRelayInfoFailed)
}

func TestRelayService_ForwardToRelays_GetRelaysError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		relayRepo: &fakeRelayRepo{activeErr: errors.New("boom")},
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "https://local.example/activities/1", Type: "Create"}}
	actor := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}}

	require.Error(t, svc.ForwardToRelays(ctx, activity, actor))
}

func TestRelayService_HandleRelayActivity_UnknownRelay(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		relayRepo: &fakeRelayRepo{getErr: errors.New("not found")},
		costRepo:  costRepo,
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	err := svc.HandleRelayActivity(ctx, &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.AcceptType}}, "https://relay.example/actor")
	assert.ErrorIs(t, err, ErrUnknownInactiveRelay)
	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_HandleRelayActivity_RelayLastSeenUpdateFailureDoesNotFail(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: true},
		},
		storeErr: errors.New("store failed"),
	}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Update"}}
	require.NoError(t, svc.HandleRelayActivity(ctx, activity, relayURL))
}

func TestRelayService_fetchRelayActor_HTTPClientError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}},
	}

	_, err := svc.fetchRelayActor(ctx, "https://relay.example/actor")
	require.Error(t, err)
}

func TestRelayService_doTrackRelayCost_RetryCostDoubling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		costRepo: costRepo,
		logger:   logger,
	}

	require.NoError(t, svc.doTrackRelayCost(ctx, "https://relay.example/actor", "delivery", "outbound", activityTypeCreate, time.Now().Add(-10*time.Millisecond), "req1", false, "retry: boom"))
	require.Len(t, costRepo.createCalls, 1)
	assert.Equal(t, int64(200), costRepo.createCalls[0].HTTPRequestCost)
	assert.Equal(t, 1, costRepo.createCalls[0].RetryCount)
}

func TestRelayService_checkRelayBudget_WarningUpdatesFlag(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	budget := &models.RelayBudget{
		RelayURL:                "https://relay.example/actor",
		LimitMicroCents:         100,
		CurrentUsageMicroCents:  70,
		WarningThresholdPercent: 75,
		WarningAlertSent:        false,
	}

	costRepo := &fakeRelayCostRepo{budget: budget}
	svc := &RelayService{
		costRepo: costRepo,
		logger:   logger,
	}

	require.NoError(t, svc.checkRelayBudget(ctx, budget.RelayURL, 10))
	require.Len(t, costRepo.updateCalls, 1)
	assert.True(t, costRepo.updateCalls[0].WarningAlertSent)
}

func TestRelayService_checkRelayBudget_AllowsWhenNoBudget(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{
		costRepo: &fakeRelayCostRepo{getErr: errors.New("not found")},
		logger:   logger,
	}

	require.NoError(t, svc.checkRelayBudget(ctx, "https://relay.example/actor", 1))
}

func TestRelayService_checkRelayBudget_Exceeded(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &fakeRelayCostRepo{budget: &models.RelayBudget{
		RelayURL:               "https://relay.example/actor",
		LimitMicroCents:        100,
		CurrentUsageMicroCents: 99,
	}}

	svc := &RelayService{
		costRepo: costRepo,
		logger:   logger,
	}

	assert.ErrorIs(t, svc.checkRelayBudget(ctx, "https://relay.example/actor", 10), ErrRelayBudgetExceeded)
}

func TestRelayService_SubscribeToRelay_FetchRelayActorError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "bob"}}},
		relayRepo: &fakeRelayRepo{},
		costRepo:  costRepo,
		logger:    logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}},
		delivery: &fakeRelayDelivery{},
		domain:   "local.example",
	}

	err := svc.SubscribeToRelay(ctx, "https://relay.example/actor", "bob")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchRelayActorFailed)
	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_SubscribeToRelay_GetActorError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayInbox := "https://relay.example/inbox"
	relayActorJSON := []byte(`{"id":"` + relayURL + `","type":"Application","inbox":"` + relayInbox + `"}`)

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{err: errors.New("actor missing")},
		relayRepo: &fakeRelayRepo{},
		costRepo:  costRepo,
		logger:    logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(relayActorJSON))}, nil
		}},
		delivery: &fakeRelayDelivery{},
		domain:   "local.example",
	}

	err := svc.SubscribeToRelay(ctx, relayURL, "bob")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGetActorFailed)
	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_SubscribeToRelay_StoreRelayInfoError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayInbox := "https://relay.example/inbox"
	relayActorJSON := []byte(`{"id":"` + relayURL + `","type":"Application","inbox":"` + relayInbox + `"}`)

	costRepo := &fakeRelayCostRepo{}
	svc := &RelayService{
		actorRepo: &fakeRelayActorRepo{
			actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://local.example/users/bob"}},
		},
		relayRepo: &fakeRelayRepo{storeErr: errors.New("store failed")},
		costRepo:  costRepo,
		logger:    logger,
		httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(relayActorJSON))}, nil
		}},
		delivery: &fakeRelayDelivery{},
		domain:   "local.example",
	}

	err := svc.SubscribeToRelay(ctx, relayURL, "bob")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreRelayInfoFailed)
	require.Len(t, costRepo.createCalls, 1)
	assert.False(t, costRepo.createCalls[0].Success)
}

func TestRelayService_HandleRelayActivity_RejectRemovesRelay(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: false},
		},
	}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	reject := &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.RejectType}}
	require.NoError(t, svc.HandleRelayActivity(ctx, reject, relayURL))
	assert.Empty(t, relayRepo.relays)
}

func TestRelayService_HandleRelayActivity_DefaultTypeNoop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"
	relayRepo := &fakeRelayRepo{
		relays: map[string]*storage.RelayInfo{
			relayURL: {URL: relayURL, InboxURL: "https://relay.example/inbox", Active: true},
		},
	}

	svc := &RelayService{
		relayRepo: relayRepo,
		costRepo:  &fakeRelayCostRepo{},
		logger:    logger,
		delivery:  &fakeRelayDelivery{},
		domain:    "local.example",
	}

	activity := &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Update"}}
	require.NoError(t, svc.HandleRelayActivity(ctx, activity, relayURL))
}

func TestRelayService_handleRelayAnnounce_ObjectIsActivity(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	svc := &RelayService{logger: logger}

	announce := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType},
		Object: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "https://remote.example/activities/1", Type: activityTypeCreate},
		},
	}

	require.NoError(t, svc.handleRelayAnnounce(ctx, announce, &RelayInfo{URL: "https://relay.example/actor"}))
}

func TestRelayService_fetchRelayActor_UsesHeaders(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	relayURL := "https://relay.example/actor"

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			assert.Contains(t, req.Header.Get("Accept"), "application/activity+json")
			assert.Equal(t, "Lesser/1.0", req.Header.Get("User-Agent"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewReader([]byte(`{
  "id":"` + relayURL + `",
  "type":"Service",
  "inbox":"https://relay.example/inbox"
}`))),
			}, nil
		}},
	}

	actor, err := svc.fetchRelayActor(ctx, relayURL)
	require.NoError(t, err)
	assert.Equal(t, "Service", actor.Type)
}

func TestRelayService_fetchRelayActor_BuildRequestUsesContext(t *testing.T) {
	logger := zaptest.NewLogger(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			assert.ErrorIs(t, req.Context().Err(), context.Canceled)
			return nil, context.Canceled
		}},
	}

	_, err := svc.fetchRelayActor(ctx, "https://relay.example/actor")
	require.Error(t, err)
}

func TestRelayService_VerifyHTTPSRequestConstruction(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	var gotReq *http.Request

	svc := &RelayService{
		logger: logger,
		httpClient: &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			gotReq = req
			body := []byte(`{"id":"` + req.URL.String() + `","type":"Application","inbox":"` + req.URL.String() + `/inbox"}`)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/activity+json"}},
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		}},
	}

	_, err := svc.fetchRelayActor(ctx, "https://relay.example/actor")
	require.NoError(t, err)
	require.NotNil(t, gotReq)
	assert.Equal(t, http.MethodGet, gotReq.Method)
}
