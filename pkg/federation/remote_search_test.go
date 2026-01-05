package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRemoteSearchActorRepo struct {
	localByUsername map[string]*activitypub.Actor
	cachedByHandle  map[string]*activitypub.Actor

	getLocalErr  error
	getCachedErr error
}

func (f *fakeRemoteSearchActorRepo) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	if f.getLocalErr != nil {
		return nil, f.getLocalErr
	}
	return f.localByUsername[username], nil
}

func (f *fakeRemoteSearchActorRepo) GetCachedRemoteActor(_ context.Context, handle string) (*activitypub.Actor, error) {
	if f.getCachedErr != nil {
		return nil, f.getCachedErr
	}
	return f.cachedByHandle[handle], nil
}

type fakeRemoteSearchUserRepo struct {
	mu sync.Mutex

	cached map[string]*activitypub.Actor
	ttls   map[string]time.Duration
	err    error
}

func (f *fakeRemoteSearchUserRepo) CacheRemoteActor(_ context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}
	if f.cached == nil {
		f.cached = make(map[string]*activitypub.Actor)
		f.ttls = make(map[string]time.Duration)
	}
	f.cached[handle] = actor
	f.ttls[handle] = ttl
	return nil
}

type fakeHTTPMux struct {
	mu sync.Mutex

	do func(req *http.Request) (*http.Response, error)
}

func (f *fakeHTTPMux) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	do := f.do
	f.mu.Unlock()
	return do(req)
}

func jsonResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestRemoteSearchHelpers(t *testing.T) {
	t.Run("parseHandle", func(t *testing.T) {
		username, domain, err := parseHandle("@alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, "alice", username)
		assert.Equal(t, "example.com", domain)

		username, domain, err = parseHandle("bob")
		require.NoError(t, err)
		assert.Equal(t, "bob", username)
		assert.Equal(t, "", domain)

		_, _, err = parseHandle("bad@@example.com")
		assert.ErrorIs(t, err, ErrInvalidHandleFormat)

		_, _, err = parseHandle("invalid username@example.com")
		assert.ErrorIs(t, err, ErrInvalidUsernameFormat)

		_, _, err = parseHandle("alice@bad domain")
		assert.ErrorIs(t, err, ErrInvalidDomainFormat)
	})

	t.Run("isValidHandle", func(t *testing.T) {
		assert.True(t, isValidHandle("alice@example.com"))
		assert.True(t, isValidHandle("@alice@example.com"))
		assert.False(t, isValidHandle("alice"))
		assert.False(t, isValidHandle("a@b@c"))
	})

	t.Run("usernameFromActorPath", func(t *testing.T) {
		assert.Equal(t, "alice", usernameFromActorPath("/users/alice"))
		assert.Equal(t, "alice", usernameFromActorPath("https://example.com/users/@alice"))
		assert.Equal(t, "", usernameFromActorPath(""))
		assert.Equal(t, "", usernameFromActorPath("/users/invalid username"))
	})
}

func TestRemoteSearchService_ResolveActor_LocalAndCached(t *testing.T) {
	localActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://local.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://local.example/users/alice/inbox",
		Outbox:            "https://local.example/users/alice/outbox",
	}

	svc := &RemoteSearchService{
		actorRepo: &fakeRemoteSearchActorRepo{
			localByUsername: map[string]*activitypub.Actor{"alice": localActor},
			cachedByHandle:  map[string]*activitypub.Actor{"bob@remote.example": localActor},
		},
		userRepo:   &fakeRemoteSearchUserRepo{},
		httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
		logger:     common.Logger(),
	}

	t.Run("local_actor", func(t *testing.T) {
		res, err := svc.ResolveActor(context.Background(), "alice")
		require.NoError(t, err)
		assert.False(t, res.IsRemote)
		assert.Equal(t, localActor.ID, res.Actor.ID)
	})

	t.Run("cached_remote_actor", func(t *testing.T) {
		res, err := svc.ResolveActor(context.Background(), "bob@remote.example")
		require.NoError(t, err)
		assert.True(t, res.IsRemote)
		assert.Equal(t, "remote.example", res.RemoteDomain)
	})
}

func TestRemoteSearchService_ResolveActor_WebFingerAndFetch(t *testing.T) {
	actorRepo := &fakeRemoteSearchActorRepo{
		localByUsername: map[string]*activitypub.Actor{},
		cachedByHandle:  map[string]*activitypub.Actor{},
	}
	userRepo := &fakeRemoteSearchUserRepo{}

	const (
		username = "bob"
		domain   = "remote.example"
		actorURL = "https://remote.example/users/bob"
	)

	httpClient := &fakeHTTPMux{}
	httpClient.do = func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/.well-known/webfinger":
			return jsonResponse(http.StatusOK, activitypub.WebFingerResource{
				Links: []activitypub.WebFingerLink{
					{Rel: "self", Type: "application/activity+json", Href: actorURL},
				},
			}), nil
		case "/users/bob":
			return jsonResponse(http.StatusOK, &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: actorURL, Type: activitypub.PersonType},
				Inbox:             actorURL + "/inbox",
				Outbox:            actorURL + "/outbox",
				PreferredUsername: username,
			}), nil
		default:
			return nil, errors.New("unexpected url: " + req.URL.String())
		}
	}

	svc := &RemoteSearchService{
		actorRepo:  actorRepo,
		userRepo:   userRepo,
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	res, err := svc.ResolveActor(context.Background(), "bob@remote.example")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.IsRemote)
	assert.Equal(t, domain, res.RemoteDomain)
	assert.Equal(t, actorURL, res.Actor.ID)

	userRepo.mu.Lock()
	_, cached := userRepo.cached["bob@remote.example"]
	userRepo.mu.Unlock()
	assert.True(t, cached)
}

func TestRemoteSearchService_ResolveActor_WebFingerErrors(t *testing.T) {
	svc := &RemoteSearchService{
		actorRepo: &fakeRemoteSearchActorRepo{},
		userRepo:  &fakeRemoteSearchUserRepo{},
		httpClient: &fakeHTTPMux{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/.well-known/webfinger" {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			return nil, errors.New("unexpected")
		}},
		logger: common.Logger(),
	}

	_, err := svc.ResolveActor(context.Background(), "bob@remote.example")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWebFingerLookupFailed)
}

func TestRemoteSearchService_ResolveActorURL_CacheAndFetch(t *testing.T) {
	actorRepo := &fakeRemoteSearchActorRepo{
		cachedByHandle: map[string]*activitypub.Actor{
			"bob@remote.example": {BaseObject: activitypub.BaseObject{ID: "cached"}},
		},
	}
	userRepo := &fakeRemoteSearchUserRepo{}

	svc := &RemoteSearchService{
		actorRepo: actorRepo,
		userRepo:  userRepo,
		httpClient: &fakeHTTPMux{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/users/@carol" {
				return jsonResponse(http.StatusOK, &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example/users/@carol",
						Type: activitypub.PersonType,
					},
					Inbox:  "https://remote.example/users/carol/inbox",
					Outbox: "https://remote.example/users/carol/outbox",
				}), nil
			}
			return nil, errors.New("unexpected: " + req.URL.String())
		}},
		logger: common.Logger(),
	}

	t.Run("cache_hit_by_inferred_username", func(t *testing.T) {
		res, err := svc.ResolveActorURL(context.Background(), "https://remote.example/users/bob")
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, "cached", res.Actor.ID)
	})

	t.Run("fetch_and_cache_by_actor_id_fallback", func(t *testing.T) {
		actorRepo.cachedByHandle = map[string]*activitypub.Actor{} // force miss
		res, err := svc.ResolveActorURL(context.Background(), "https://remote.example/users/@carol")
		require.NoError(t, err)
		assert.True(t, res.IsRemote)

		userRepo.mu.Lock()
		_, cached := userRepo.cached["carol@remote.example"]
		userRepo.mu.Unlock()
		assert.True(t, cached)
	})
}

func TestRemoteSearchService_SearchRemoteActors_FuzzySearch(t *testing.T) {
	svc := &RemoteSearchService{
		actorRepo: &fakeRemoteSearchActorRepo{},
		userRepo:  &fakeRemoteSearchUserRepo{},
		httpClient: &fakeHTTPMux{do: func(req *http.Request) (*http.Response, error) {
			if strings.HasPrefix(req.URL.Path, "/api/v2/search") {
				return jsonResponse(http.StatusOK, map[string]any{
					"accounts": []map[string]any{
						{
							"username":     "bob",
							"acct":         "bob",
							"display_name": "Bob",
							"avatar":       "https://cdn.example/avatar.png",
							"note":         "hello",
							"url":          "https://" + req.URL.Host + "/users/bob",
						},
					},
				}), nil
			}
			return nil, errors.New("unexpected: " + req.URL.String())
		}},
		logger: common.Logger(),
	}

	results, err := svc.SearchRemoteActors(context.Background(), "bob", 2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].IsRemote)
	assert.NotEmpty(t, results[0].RemoteDomain)
}

func TestRemoteSearchService_SearchRemoteActors_ExactHandleStopsEarly(t *testing.T) {
	localActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.example/users/bob/inbox",
		Outbox:            "https://remote.example/users/bob/outbox",
	}

	svc := &RemoteSearchService{
		actorRepo: &fakeRemoteSearchActorRepo{
			cachedByHandle: map[string]*activitypub.Actor{"bob@remote.example": localActor},
		},
		userRepo:   &fakeRemoteSearchUserRepo{},
		httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
		logger:     common.Logger(),
	}

	results, err := svc.SearchRemoteActors(context.Background(), "bob@remote.example", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].IsRemote)
}

func TestRemoteSearchService_SearchRemoteActors_LimitZeroReturns(t *testing.T) {
	svc := &RemoteSearchService{
		actorRepo:  &fakeRemoteSearchActorRepo{},
		userRepo:   &fakeRemoteSearchUserRepo{},
		httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
		logger:     common.Logger(),
	}

	results, err := svc.SearchRemoteActors(context.Background(), "bob", 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRemoteSearchService_ResolveActor_CacheWriteErrorDoesNotFail(t *testing.T) {
	actorRepo := &fakeRemoteSearchActorRepo{
		localByUsername: map[string]*activitypub.Actor{},
		cachedByHandle:  map[string]*activitypub.Actor{},
	}
	userRepo := &fakeRemoteSearchUserRepo{err: errors.New("cache down")}

	const actorURL = "https://remote.example/users/bob"

	httpClient := &fakeHTTPMux{}
	httpClient.do = func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/.well-known/webfinger":
			return jsonResponse(http.StatusOK, activitypub.WebFingerResource{
				Links: []activitypub.WebFingerLink{
					{Rel: "self", Type: "application/activity+json", Href: actorURL},
				},
			}), nil
		case "/users/bob":
			return jsonResponse(http.StatusOK, &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: actorURL, Type: activitypub.PersonType},
				Inbox:             actorURL + "/inbox",
				Outbox:            actorURL + "/outbox",
				PreferredUsername: "bob",
			}), nil
		default:
			return nil, errors.New("unexpected url: " + req.URL.String())
		}
	}

	svc := &RemoteSearchService{
		actorRepo:  actorRepo,
		userRepo:   userRepo,
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	res, err := svc.ResolveActor(context.Background(), "bob@remote.example")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.IsRemote)
}

func TestRemoteSearchService_ResolveActor_FetchRemoteActorError(t *testing.T) {
	httpClient := &fakeHTTPMux{}
	httpClient.do = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/.well-known/webfinger" {
			return jsonResponse(http.StatusOK, activitypub.WebFingerResource{
				Links: []activitypub.WebFingerLink{
					{Rel: "self", Type: "application/activity+json", Href: "https://remote.example/users/bob"},
				},
			}), nil
		}
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("boom"))}, nil
	}

	svc := &RemoteSearchService{
		actorRepo:  &fakeRemoteSearchActorRepo{cachedByHandle: map[string]*activitypub.Actor{}},
		userRepo:   &fakeRemoteSearchUserRepo{},
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	_, err := svc.ResolveActor(context.Background(), "bob@remote.example")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchRemoteActorFailed)
}

func TestRemoteSearchService_ResolveActorURL_InvalidInput(t *testing.T) {
	svc := &RemoteSearchService{
		actorRepo: &fakeRemoteSearchActorRepo{},
		userRepo:  &fakeRemoteSearchUserRepo{},
	}

	_, err := svc.ResolveActorURL(context.Background(), "not a url")
	require.Error(t, err)
}

func TestRemoteSearchService_ResolveActorURL_CacheWriteErrorDoesNotFail(t *testing.T) {
	actorRepo := &fakeRemoteSearchActorRepo{cachedByHandle: map[string]*activitypub.Actor{}}
	userRepo := &fakeRemoteSearchUserRepo{err: errors.New("cache down")}

	actorURL := "https://remote.example/users/bob"
	httpClient := &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: actorURL, Type: activitypub.PersonType},
			Inbox:             actorURL + "/inbox",
			Outbox:            actorURL + "/outbox",
			PreferredUsername: "bob",
		}), nil
	}}

	svc := &RemoteSearchService{
		actorRepo:  actorRepo,
		userRepo:   userRepo,
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	res, err := svc.ResolveActorURL(context.Background(), actorURL)
	require.NoError(t, err)
	assert.True(t, res.IsRemote)
}

func TestRemoteSearchService_ResolveActorURL_SkipsCachingWhenNoUsername(t *testing.T) {
	userRepo := &fakeRemoteSearchUserRepo{}

	actorURL := "https://remote.example/users/bob"
	httpClient := &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/invalid username",
				Type: activitypub.PersonType,
			},
			Inbox:  actorURL + "/inbox",
			Outbox: actorURL + "/outbox",
		}), nil
	}}

	svc := &RemoteSearchService{
		actorRepo:  &fakeRemoteSearchActorRepo{},
		userRepo:   userRepo,
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	res, err := svc.ResolveActorURL(context.Background(), actorURL)
	require.NoError(t, err)
	assert.True(t, res.IsRemote)

	userRepo.mu.Lock()
	defer userRepo.mu.Unlock()
	assert.Empty(t, userRepo.cached)
}

func TestRemoteSearchService_webFingerLookup_ParseAndLinkErrors(t *testing.T) {
	t.Run("parse_error", func(t *testing.T) {
		svc := &RemoteSearchService{
			actorRepo: &fakeRemoteSearchActorRepo{},
			userRepo:  &fakeRemoteSearchUserRepo{},
			httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}, nil
			}},
			logger: common.Logger(),
		}

		_, err := svc.ResolveActor(context.Background(), "bob@remote.example")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrWebFingerLookupFailed)
	})

	t.Run("missing_activitypub_link", func(t *testing.T) {
		svc := &RemoteSearchService{
			actorRepo: &fakeRemoteSearchActorRepo{},
			userRepo:  &fakeRemoteSearchUserRepo{},
			httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, activitypub.WebFingerResource{}), nil
			}},
			logger: common.Logger(),
		}

		_, err := svc.ResolveActor(context.Background(), "bob@remote.example")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrWebFingerLookupFailed)
	})
}

func TestRemoteSearchService_fetchRemoteActor_InvalidActorMissingFields(t *testing.T) {
	const actorURL = "https://remote.example/users/bob"
	httpClient := &fakeHTTPMux{do: func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/.well-known/webfinger":
			return jsonResponse(http.StatusOK, activitypub.WebFingerResource{
				Links: []activitypub.WebFingerLink{
					{Rel: "self", Type: "application/activity+json", Href: actorURL},
				},
			}), nil
		case "/users/bob":
			return jsonResponse(http.StatusOK, &activitypub.Actor{
				BaseObject: activitypub.BaseObject{ID: actorURL, Type: activitypub.PersonType},
				// Missing inbox.
			}), nil
		default:
			return nil, errors.New("unexpected: " + req.URL.String())
		}
	}}

	svc := &RemoteSearchService{
		actorRepo:  &fakeRemoteSearchActorRepo{},
		userRepo:   &fakeRemoteSearchUserRepo{},
		httpClient: httpClient,
		logger:     common.Logger(),
	}

	_, err := svc.ResolveActor(context.Background(), "bob@remote.example")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchRemoteActorFailed)
}

func TestRemoteSearchService_searchRemoteInstance_ErrorBranches(t *testing.T) {
	svc := &RemoteSearchService{
		httpClient: &fakeHTTPMux{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("boom") }},
		logger:     common.Logger(),
	}

	_, err := svc.searchRemoteInstance(context.Background(), "bad domain", "q")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateSearchRequestFailed)

	_, err = svc.searchRemoteInstance(context.Background(), "example.com", "q")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSearchRequestFailed)
}
