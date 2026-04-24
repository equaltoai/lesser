package federation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDeliveryService_Project20_DeliverToFollowersAndRecipientsSuppressesFollowerDuplicate(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.example/users/bob/inbox",
		Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
	}
	store := &federationStoreStub{
		getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
		getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
			return []string{"bob@remote.example"}, "", nil
		},
		getCachedActorFn: func(_ context.Context, key string) (*activitypub.Actor, error) {
			switch key {
			case "bob@remote.example", remoteActor.ID:
				return remoteActor, nil
			default:
				return nil, storage.ErrNotFound
			}
		},
		recordActivityFn: func(_ context.Context, _ *storage.FederationActivity) error { return nil },
	}
	httpDoer := &httpDoerStub{
		doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(&emptyReader{}),
				Header:     make(http.Header),
			}, nil
		},
	}
	actor := testSigningActor()
	actor.Followers = actor.ID + "/followers"
	d := &DeliveryService{
		store:      store,
		httpClient: httpDoer,
		logger:     zap.NewNop(),
		cfg:        &appConfig.Config{Domain: "example.com"},
	}

	err := d.DeliverToFollowersAndRecipients(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/create-1",
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{actor.Followers, remoteActor.ID},
		},
		Actor:  actor.ID,
		Object: "https://example.com/objects/note-1",
	}, actor)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		httpDoer.mu.Lock()
		defer httpDoer.mu.Unlock()
		return len(httpDoer.requests) == 1
	}, time.Second, 10*time.Millisecond)

	httpDoer.mu.Lock()
	defer httpDoer.mu.Unlock()
	require.Equal(t, "https://remote.example/inbox", httpDoer.requests[0].URL.String())
}

func TestDeliveryService_Project20_FollowerResolutionFailureStillDeliversExplicitRecipients(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.example/users/bob/inbox",
		Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
	}
	store := &federationStoreStub{
		getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
		getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
			return nil, "", errors.New("followers unavailable")
		},
		getCachedActorFn: func(_ context.Context, key string) (*activitypub.Actor, error) {
			if key == remoteActor.ID {
				return remoteActor, nil
			}
			return nil, storage.ErrNotFound
		},
		recordActivityFn: func(_ context.Context, _ *storage.FederationActivity) error { return nil },
	}
	httpDoer := &httpDoerStub{
		doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(&emptyReader{}),
				Header:     make(http.Header),
			}, nil
		},
	}
	actor := testSigningActor()
	actor.Followers = actor.ID + "/followers"
	d := &DeliveryService{
		store:      store,
		httpClient: httpDoer,
		logger:     zap.NewNop(),
		cfg:        &appConfig.Config{Domain: "example.com"},
	}

	err := d.DeliverToFollowersAndRecipients(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/activities/create-2",
			Type: activitypub.CreateType,
			To:   []string{activitypub.PublicAddress},
			CC:   []string{actor.Followers, remoteActor.ID},
		},
		Actor:  actor.ID,
		Object: "https://example.com/objects/note-2",
	}, actor)
	require.NoError(t, err)

	httpDoer.mu.Lock()
	defer httpDoer.mu.Unlock()
	require.Len(t, httpDoer.requests, 1)
	require.Equal(t, "https://remote.example/users/bob/inbox", httpDoer.requests[0].URL.String())
}

func TestDeliveryService_Project20_EmptyFollowerTargetsNoop(t *testing.T) {
	d := &DeliveryService{logger: zap.NewNop()}
	require.NoError(t, d.deliverResolvedFollowerTargets(
		context.Background(),
		&activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-empty", Type: activitypub.CreateType}},
		testSigningActor(),
		nil,
		zap.NewNop(),
	))
}

type emptyReader struct{}

func (r *emptyReader) Read(_ []byte) (int, error) {
	return 0, io.EOF
}
