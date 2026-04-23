package federation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type federationStoreStub struct {
	mu sync.Mutex

	getActorPrivateKeyFn func(ctx context.Context, username string) (string, error)
	getActorFn           func(ctx context.Context, username string) (*activitypub.Actor, error)
	getFollowersFn       func(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	getCachedActorFn     func(ctx context.Context, actorID string) (*activitypub.Actor, error)
	cacheActorFn         func(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
	recordActivityFn     func(ctx context.Context, activity *storage.FederationActivity) error

	recorded []*storage.FederationActivity
	cached   []string
}

func (s *federationStoreStub) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	if s.getActorPrivateKeyFn == nil {
		return "", errors.New("GetActorPrivateKey not configured")
	}
	return s.getActorPrivateKeyFn(ctx, username)
}

func (s *federationStoreStub) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	if s.getActorFn == nil {
		return nil, errors.New("GetActor not configured")
	}
	return s.getActorFn(ctx, username)
}

func (s *federationStoreStub) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if s.getFollowersFn == nil {
		return nil, "", errors.New("GetFollowers not configured")
	}
	return s.getFollowersFn(ctx, username, limit, cursor)
}

func (s *federationStoreStub) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	if s.getCachedActorFn == nil {
		return nil, errors.New("GetCachedRemoteActor not configured")
	}
	return s.getCachedActorFn(ctx, actorID)
}

func (s *federationStoreStub) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	s.mu.Lock()
	s.cached = append(s.cached, handle)
	s.mu.Unlock()

	if s.cacheActorFn == nil {
		return nil
	}
	return s.cacheActorFn(ctx, handle, actor, ttl)
}

func (s *federationStoreStub) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	s.mu.Lock()
	s.recorded = append(s.recorded, activity)
	s.mu.Unlock()

	if s.recordActivityFn == nil {
		return nil
	}
	return s.recordActivityFn(ctx, activity)
}

type httpDoerStub struct {
	doFn func(req *http.Request) (*http.Response, error)

	mu       sync.Mutex
	requests []*http.Request
}

func (d *httpDoerStub) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	d.requests = append(d.requests, req)
	d.mu.Unlock()

	if d.doFn == nil {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}
	return d.doFn(req)
}

type sqsClientStub struct {
	sendFn func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)

	mu    sync.Mutex
	calls []*sqs.SendMessageInput
}

func (s *sqsClientStub) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	s.mu.Lock()
	s.calls = append(s.calls, params)
	s.mu.Unlock()

	if s.sendFn == nil {
		return &sqs.SendMessageOutput{}, nil
	}
	return s.sendFn(ctx, params, optFns...)
}

func mustTestRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

func testSigningActor() *activitypub.Actor {
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: "Person",
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:    "https://example.com/users/alice#main-key",
			Owner: "https://example.com/users/alice",
		},
	}
}

func TestDeliveryService_DeliverActivity_SuccessAndFailures(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)

	t.Run("success", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			recordActivityFn:     func(_ context.Context, _ *storage.FederationActivity) error { return nil },
		}
		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "application/activity+json", req.Header.Get("Content-Type"))
				return &http.Response{
					StatusCode: http.StatusAccepted,
					Body:       io.NopCloser(strings.NewReader("accepted")),
					Header:     make(http.Header),
				}, nil
			},
		}

		d := &DeliveryService{
			store:      store,
			httpClient: httpDoer,
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}

		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Actor:      "https://example.com/users/alice",
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", testSigningActor())
		require.NoError(t, err)
	})

	t.Run("marshal_error", func(t *testing.T) {
		d := &DeliveryService{
			store:      &federationStoreStub{},
			httpClient: &httpDoerStub{},
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     make(chan int),
		}, "https://remote.example/inbox", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrActivityMarshalFailed)
	})

	t.Run("request_creation_error_records_failure", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			recordActivityFn:     func(_ context.Context, _ *storage.FederationActivity) error { return nil },
		}
		d := &DeliveryService{
			store:      store,
			httpClient: &httpDoerStub{},
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "://bad", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRequestCreationFailed)
	})

	t.Run("private_key_error", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return "", errors.New("boom") },
		}
		d := &DeliveryService{
			store:      store,
			httpClient: &httpDoerStub{},
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrPrivateKeyRetrievalFailed)
	})

	t.Run("private_key_parse_error", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return "not a key", nil },
		}
		d := &DeliveryService{
			store:      store,
			httpClient: &httpDoerStub{},
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrPrivateKeyParseFailed)
	})

	t.Run("http_do_error", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
		}
		httpDoer := &httpDoerStub{
			doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("nope") },
		}
		d := &DeliveryService{
			store:      store,
			httpClient: httpDoer,
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHTTPRequestFailed)
	})

	t.Run("non_2xx_status", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
		}
		httpDoer := &httpDoerStub{
			doFn: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("fail")),
					Header:     make(http.Header),
				}, nil
			},
		}
		d := &DeliveryService{
			store:      store,
			httpClient: httpDoer,
			logger:     zap.NewNop(),
			cfg:        &appConfig.Config{Domain: "example.com"},
		}
		err := d.DeliverActivity(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", testSigningActor())
		require.Error(t, err)
		require.ErrorIs(t, err, ErrDeliveryHTTPStatusFailed)
	})
}

func TestDeliveryService_FetchRemoteActor_CacheAndFailures(t *testing.T) {
	t.Run("cached_hit", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, actorID string) (*activitypub.Actor, error) {
				require.Equal(t, "https://remote.example/users/bob", actorID)
				return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: actorID}, Inbox: "https://remote.example/users/bob/inbox"}, nil
			},
		}
		httpDoer := &httpDoerStub{
			doFn: func(_ *http.Request) (*http.Response, error) {
				t.Fatalf("unexpected network call for cached actor")
				return nil, nil
			},
		}
		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		actor, err := d.fetchRemoteActor(context.Background(), "https://remote.example/users/bob")
		require.NoError(t, err)
		require.Equal(t, "https://remote.example/users/bob", actor.ID)
	})

	t.Run("request_creation_error", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
		}
		d := &DeliveryService{store: store, httpClient: &httpDoerStub{}, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		_, err := d.fetchRemoteActor(context.Background(), "://bad")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRequestCreationFailed)
	})

	t.Run("http_do_error", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
		}
		httpDoer := &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("nope") }}
		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		_, err := d.fetchRemoteActor(context.Background(), "https://remote.example/users/bob")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrFetchRemoteActorFailed)
	})

	t.Run("non_200_status", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
		}
		httpDoer := &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("nope")),
				Header:     make(http.Header),
			}, nil
		}}
		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		_, err := d.fetchRemoteActor(context.Background(), "https://remote.example/users/bob")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrFetchActorHTTPStatusFailed)
	})

	t.Run("decode_error", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
		}
		httpDoer := &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{not-json")),
				Header:     make(http.Header),
			}, nil
		}}
		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		_, err := d.fetchRemoteActor(context.Background(), "https://remote.example/users/bob")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrActorDecodeFailed)
	})

	t.Run("caches_actor_even_if_cache_write_fails", func(t *testing.T) {
		store := &federationStoreStub{
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
			cacheActorFn: func(_ context.Context, _ string, _ *activitypub.Actor, _ time.Duration) error {
				return errors.New("cache down")
			},
		}

		actorDoc := activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/bob",
				Type: "Person",
			},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
		}
		body, err := jsonMarshalStable(&actorDoc)
		require.NoError(t, err)

		httpDoer := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}

		actor, err := d.fetchRemoteActor(context.Background(), actorDoc.ID)
		require.NoError(t, err)
		require.Equal(t, actorDoc.ID, actor.ID)

		store.mu.Lock()
		cached := append([]string(nil), store.cached...)
		store.mu.Unlock()
		require.Contains(t, cached, "@bob@remote.example")
	})
}

func TestDeliveryService_RecipientsAndQueueing(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)

	t.Run("deliver_to_followers_resolves_remote_handle_targets", func(t *testing.T) {
		signingActor := testSigningActor()

		bob := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		carol := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol", Type: "Person"},
			PreferredUsername: "carol",
			Inbox:             "https://remote.example/users/carol/inbox",
			Outbox:            "https://remote.example/users/carol/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		carolJSON, err := jsonMarshalStable(&carol)
		require.NoError(t, err)

		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				return []string{"local", "bob@remote.example", "carol@remote.example", "missing@remote.example"}, "", nil
			},
			getActorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
				if username != "local" {
					return nil, errors.New("not found")
				}
				return &activitypub.Actor{
					BaseObject: activitypub.BaseObject{ID: "https://example.com/users/local", Type: "Person"},
					Inbox:      "https://example.com/users/local/inbox",
					Outbox:     "https://example.com/users/local/outbox",
				}, nil
			},
			getCachedActorFn: func(_ context.Context, identifier string) (*activitypub.Actor, error) {
				switch identifier {
				case "bob@remote.example":
					return &bob, nil
				default:
					return nil, errors.New("cache miss")
				}
			},
		}

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				switch req.Method {
				case http.MethodGet:
					switch req.URL.String() {
					case "https://remote.example/.well-known/webfinger?resource=acct%3Acarol%40remote.example":
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: io.NopCloser(strings.NewReader(`{
								"subject":"acct:carol@remote.example",
								"links":[{"rel":"self","type":"application/activity+json","href":"https://remote.example/users/carol"}]
							}`)),
							Header: make(http.Header),
						}, nil
					case "https://remote.example/.well-known/webfinger?resource=acct%3Amissing%40remote.example":
						return &http.Response{
							StatusCode: http.StatusNotFound,
							Body:       io.NopCloser(strings.NewReader("missing")),
							Header:     make(http.Header),
						}, nil
					case carol.ID:
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(carolJSON)),
							Header:     make(http.Header),
						}, nil
					default:
						t.Fatalf("unexpected GET %s", req.URL.String())
						return nil, nil
					}
				case http.MethodPost:
					require.Equal(t, "https://remote.example/inbox", req.URL.String())
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("ok")),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected method %s", req.Method)
					return nil, nil
				}
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		err = d.DeliverToFollowers(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-remote-followers", Type: "Create"},
		}, signingActor)
		require.NoError(t, err)

		httpDoer.mu.Lock()
		requests := append([]*http.Request(nil), httpDoer.requests...)
		httpDoer.mu.Unlock()

		var getTargets []string
		for _, req := range requests {
			switch req.Method {
			case http.MethodGet:
				getTargets = append(getTargets, req.URL.String())
			}
		}

		require.Contains(t, getTargets, "https://remote.example/.well-known/webfinger?resource=acct%3Acarol%40remote.example")
		require.Contains(t, getTargets, "https://remote.example/.well-known/webfinger?resource=acct%3Amissing%40remote.example")
		require.Contains(t, getTargets, "https://remote.example/users/carol")
		require.NotContains(t, getTargets, bob.ID)
	})

	t.Run("deliver_to_followers_groups_by_shared_inbox_and_skips_locals", func(t *testing.T) {
		signingActor := testSigningActor()

		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				return []string{"local", "remote1", "remote2", "missing"}, "", nil
			},
			getActorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
				switch username {
				case "local":
					return &activitypub.Actor{
						BaseObject: activitypub.BaseObject{ID: "https://example.com/users/local"},
						Inbox:      "https://example.com/users/local/inbox",
						Outbox:     "https://example.com/users/local/outbox",
					}, nil
				case "remote1":
					return &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: "https://remote.example/users/one"},
						PreferredUsername:         "one",
						Inbox:                     "https://remote.example/users/one/inbox",
						Outbox:                    "https://remote.example/users/one/outbox",
						Endpoints:                 &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
						ManuallyApprovesFollowers: false,
					}, nil
				case "remote2":
					return &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/two"},
						PreferredUsername: "two",
						Inbox:             "https://remote.example/users/two/inbox",
						Outbox:            "https://remote.example/users/two/outbox",
						Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
					}, nil
				default:
					return nil, errors.New("not found")
				}
			},
		}

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				switch req.Method {
				case http.MethodPost:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("ok")),
						Header:     make(http.Header),
					}, nil
				default:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
				}
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		err := d.DeliverToFollowers(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-1",
				Type: "Create",
				To:   []string{"https://example.com/users/alice/followers"},
			},
		}, signingActor)
		require.NoError(t, err)
	})

	t.Run("deliver_to_recipients_shared_inbox_then_fallback_to_individual", func(t *testing.T) {
		signingActor := testSigningActor()

		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getCachedActorFn:     func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
			getActorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
				// DeliverDirectMessage path uses GetActor with recipientID; keep it non-fatal.
				return &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: username}, Inbox: username + "/inbox", Outbox: username + "/outbox"}, nil
			},
		}

		bob := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		charlie := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/charlie", Type: "Person"},
			PreferredUsername: "charlie",
			Inbox:             "https://remote.example/users/charlie/inbox",
			Outbox:            "https://remote.example/users/charlie/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}

		bobJSON, err := jsonMarshalStable(&bob)
		require.NoError(t, err)
		charlieJSON, err := jsonMarshalStable(&charlie)
		require.NoError(t, err)

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				switch req.Method {
				case http.MethodGet:
					switch req.URL.String() {
					case bob.ID:
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(bobJSON)), Header: make(http.Header)}, nil
					case charlie.ID:
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(charlieJSON)), Header: make(http.Header)}, nil
					default:
						return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("nope")), Header: make(http.Header)}, nil
					}
				case http.MethodPost:
					// Fail shared inbox once to exercise fallback, succeed elsewhere.
					if req.URL.String() == "https://remote.example/inbox" {
						return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("fail")), Header: make(http.Header)}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
				default:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
				}
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-1",
				Type: "Create",
				To:   []string{bob.ID, charlie.ID},
			},
			Object: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}

		err = d.DeliverToRecipientsWithPrivacy(context.Background(), activity, signingActor)
		require.NoError(t, err)
	})

	t.Run("deliver_to_recipients_expands_private_followers_collection_once", func(t *testing.T) {
		signingActor := testSigningActor()
		followersCollection := followersCollectionForActor(signingActor)

		carol := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/carol", Type: "Person"},
			PreferredUsername: "carol",
			Inbox:             "https://remote.example/users/carol/inbox",
			Outbox:            "https://remote.example/users/carol/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		dave := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/dave", Type: "Person"},
			PreferredUsername: "dave",
			Inbox:             "https://remote.example/users/dave/inbox",
			Outbox:            "https://remote.example/users/dave/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}

		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				return []string{"carol@remote.example", "dave@remote.example"}, "", nil
			},
			getCachedActorFn: func(_ context.Context, identifier string) (*activitypub.Actor, error) {
				switch identifier {
				case "carol@remote.example", carol.ID:
					return &carol, nil
				case "dave@remote.example", dave.ID:
					return &dave, nil
				default:
					return nil, errors.New("cache miss")
				}
			},
			getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("unexpected local actor lookup")
			},
		}

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "https://remote.example/inbox", req.URL.String())
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-private-followers",
				Type: "Create",
				To:   []string{followersCollection, carol.ID},
			},
			Object: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-private", Type: "Note"}},
		}

		require.NoError(t, d.DeliverToRecipientsWithPrivacy(context.Background(), activity, signingActor))

		httpDoer.mu.Lock()
		requests := append([]*http.Request(nil), httpDoer.requests...)
		httpDoer.mu.Unlock()
		require.Len(t, requests, 1)
		require.Equal(t, "https://remote.example/inbox", requests[0].URL.String())
	})

	t.Run("deliver_to_recipients_skips_public_followers_collection_expansion", func(t *testing.T) {
		signingActor := testSigningActor()
		followersCollection := followersCollectionForActor(signingActor)

		bob := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
		}

		followerLookups := 0
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				followerLookups++
				return []string{"bob@remote.example"}, "", nil
			},
			getCachedActorFn: func(_ context.Context, identifier string) (*activitypub.Actor, error) {
				if identifier == bob.ID {
					return &bob, nil
				}
				return nil, errors.New("cache miss")
			},
		}

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, bob.Inbox, req.URL.String())
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-public-followers",
				Type: "Create",
				To:   []string{activitypub.PublicAddress, bob.ID},
				CC:   []string{followersCollection},
			},
			Object: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-public", Type: "Note"}},
		}

		require.NoError(t, d.DeliverToRecipientsWithPrivacy(context.Background(), activity, signingActor))
		require.Equal(t, 0, followerLookups)
	})

	t.Run("queue_delivery_falls_back_to_sync_on_marshal_error_and_on_sqs_error", func(t *testing.T) {
		ctx := context.Background()
		signingActor := testSigningActor()

		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
		}

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodPost, req.Method)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
			},
		}

		d := &DeliveryService{
			store:      store,
			httpClient: httpDoer,
			logger:     zap.NewNop(),
			sqsClient: &sqsClientStub{
				sendFn: func(_ context.Context, _ *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
					return nil, errors.New("sqs down")
				},
			},
			queueURL: "https://sqs.example/queue",
			cfg: &appConfig.Config{
				Domain:                 "example.com",
				FederationDeliveryMode: "async",
			},
		}

		// Marshal error in delivery message forces sync fallback.
		err := d.QueueDelivery(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     make(chan int),
		}, "https://remote.example/inbox", signingActor)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrActivityMarshalFailed)

		// SQS error forces sync fallback too.
		err = d.QueueDelivery(ctx, &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-2", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", signingActor)
		require.NoError(t, err)
	})
}

func TestDeliveryService_NewAndWrappers(t *testing.T) {
	cfg := &appConfig.Config{Domain: "example.com"}
	svc := NewDeliveryService(&federationStoreStub{}, cfg)
	require.NotNil(t, svc)

	// Cover non-empty queue URL branch; success/failure depends on environment config but should not panic.
	cfgWithQueue := &appConfig.Config{Domain: "example.com", FederationQueueURL: "https://sqs.example/queue"}
	svc = NewDeliveryService(&federationStoreStub{}, cfgWithQueue)
	require.NotNil(t, svc)

	// DeliverToRecipients is a thin wrapper around DeliverToRecipientsWithPrivacy.
	d := &DeliveryService{
		store: &federationStoreStub{
			getActorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
				return &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/" + username, Type: "Person"},
					PreferredUsername: username,
					Inbox:             "https://example.com/users/" + username + "/inbox",
					Outbox:            "https://example.com/users/" + username + "/outbox",
				}, nil
			},
		},
		logger: zap.NewNop(),
		cfg:    &appConfig.Config{Domain: "example.com"},
	}
	err := d.DeliverToRecipients(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create", To: []string{"https://example.com/users/local"}},
	}, testSigningActor())
	require.NoError(t, err)
}

func TestDeliveryService_DirectMessage_AndSharedInboxHelpers(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)
	signingActor := testSigningActor()

	t.Run("deliver_direct_message_success", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("unexpected local actor lookup")
			},
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("cache miss") },
		}

		recipient := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		recipientJSON, err := jsonMarshalStable(&recipient)
		require.NoError(t, err)

		httpDoer := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				require.Equal(t, recipient.ID, req.URL.String())
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(recipientJSON)), Header: make(http.Header)}, nil
			case http.MethodPost:
				require.Equal(t, "https://remote.example/inbox", req.URL.String())
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
			default:
				t.Fatalf("unexpected method %s", req.Method)
				return nil, nil
			}
		}}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-1",
				Type: "Create",
				To: []string{
					"https://remote.example/users/bob",
				},
			},
			Object: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}
		require.NoError(t, d.DeliverDirectMessage(context.Background(), activity, signingActor))

		httpDoer.mu.Lock()
		requests := append([]*http.Request(nil), httpDoer.requests...)
		httpDoer.mu.Unlock()
		require.Len(t, requests, 2)
		require.Equal(t, http.MethodGet, requests[0].Method)
		require.Equal(t, recipient.ID, requests[0].URL.String())
		require.Equal(t, http.MethodPost, requests[1].Method)
		require.Equal(t, "https://remote.example/inbox", requests[1].URL.String())
	})

	t.Run("deliver_direct_message_failure_aggregates_errors", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("unexpected local actor lookup")
			},
			getCachedActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("cache miss") },
		}

		recipient := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
			Endpoints:         &activitypub.Endpoints{SharedInbox: "https://remote.example/inbox"},
		}
		recipientJSON, err := jsonMarshalStable(&recipient)
		require.NoError(t, err)

		httpDoer := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				switch req.URL.String() {
				case recipient.ID:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(recipientJSON)), Header: make(http.Header)}, nil
				case "https://remote.example/users/missing":
					return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("missing")), Header: make(http.Header)}, nil
				default:
					t.Fatalf("unexpected GET %s", req.URL.String())
					return nil, nil
				}
			case http.MethodPost:
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("fail")), Header: make(http.Header)}, nil
			default:
				t.Fatalf("unexpected method %s", req.Method)
				return nil, nil
			}
		}}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "act-1",
				Type: "Create",
				To: []string{
					"https://remote.example/users/bob",
					"https://remote.example/users/missing",
				},
			},
			Object: &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}

		err = d.DeliverDirectMessage(context.Background(), activity, signingActor)
		require.ErrorIs(t, err, ErrDeliveryDirectMessageToInboxesFailed)
	})
}

func TestDeliveryService_RecipientDelivery_MoreBranches(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)
	signingActor := testSigningActor()

	t.Run("deliver_to_recipients_single_recipient_skips_shared_inbox_optimization", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getCachedActorFn:     func(_ context.Context, _ string) (*activitypub.Actor, error) { return nil, errors.New("miss") },
		}

		recipient := activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
			PreferredUsername: "bob",
			Inbox:             "https://remote.example/users/bob/inbox",
			Outbox:            "https://remote.example/users/bob/outbox",
		}
		recipientJSON, err := jsonMarshalStable(&recipient)
		require.NoError(t, err)

		httpDoer := &httpDoerStub{
			doFn: func(req *http.Request) (*http.Response, error) {
				switch req.Method {
				case http.MethodGet:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(recipientJSON)), Header: make(http.Header)}, nil
				case http.MethodPost:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
				default:
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
				}
			},
		}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create", To: []string{recipient.ID}},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}
		require.NoError(t, d.DeliverToRecipients(context.Background(), activity, signingActor))
	})
}

func TestDeliveryService_FollowersAndQueueDelivery_MoreBranches(t *testing.T) {
	privateKey := mustTestRSAPrivateKeyPEM(t)
	signingActor := testSigningActor()

	t.Run("deliver_to_followers_get_followers_error", func(t *testing.T) {
		store := &federationStoreStub{
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				return nil, "", errors.New("boom")
			},
		}
		d := &DeliveryService{store: store, httpClient: &httpDoerStub{}, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		err := d.DeliverToFollowers(context.Background(), &activitypub.Activity{BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"}}, signingActor)
		require.ErrorIs(t, err, ErrGetFollowersFailed)
	})

	t.Run("deliver_to_followers_delivery_errors_fail_overall", func(t *testing.T) {
		store := &federationStoreStub{
			getActorPrivateKeyFn: func(_ context.Context, _ string) (string, error) { return privateKey, nil },
			getFollowersFn: func(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
				return []string{"remote1@remote.example", "remote2@remote.example"}, "", nil
			},
			getActorFn: func(_ context.Context, _ string) (*activitypub.Actor, error) {
				return nil, errors.New("unexpected local actor lookup")
			},
			getCachedActorFn: func(_ context.Context, identifier string) (*activitypub.Actor, error) {
				username := strings.TrimSuffix(identifier, "@remote.example")
				return &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/" + username, Type: "Person"},
					PreferredUsername: username,
					Inbox:             "https://remote.example/users/" + username + "/inbox",
					Outbox:            "https://remote.example/users/" + username + "/outbox",
				}, nil
			},
		}

		httpDoer := &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("fail")), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
		}}

		d := &DeliveryService{store: store, httpClient: httpDoer, logger: zap.NewNop(), cfg: &appConfig.Config{Domain: "example.com"}}
		err := d.DeliverToFollowers(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create", To: []string{"https://example.com/users/alice/followers"}},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, signingActor)
		require.ErrorIs(t, err, ErrDeliveryToInboxesFailed)
	})

	t.Run("queue_delivery_async_success_records_metrics", func(t *testing.T) {
		d := &DeliveryService{
			logger:    zap.NewNop(),
			sqsClient: &sqsClientStub{},
			queueURL:  "https://sqs.example/queue",
			cfg:       &appConfig.Config{Domain: "example.com", FederationDeliveryMode: "async"},
		}

		err := d.QueueDelivery(context.Background(), &activitypub.Activity{
			BaseObject: activitypub.BaseObject{ID: "act-1", Type: "Create"},
			Object:     &activitypub.Note{BaseObject: activitypub.BaseObject{ID: "note-1", Type: "Note"}},
		}, "https://remote.example/inbox", signingActor)
		require.NoError(t, err)
	})
}

func TestDeliveryService_CalculateDeliveryPriority_CoversActivityTypes(t *testing.T) {
	d := &DeliveryService{
		logger: zap.NewNop(),
		cfg:    &appConfig.Config{Domain: "example.com"},
	}

	tests := []string{"Create", "Like", "Announce", "Follow", "Accept", "Undo", "Delete", "Update", "Unknown"}
	for _, typ := range tests {
		t.Run(typ, func(t *testing.T) {
			priority := d.calculateDeliveryPriority(context.Background(), &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: typ}}, "https://remote.example/inbox")
			require.GreaterOrEqual(t, priority, 1)
			require.LessOrEqual(t, priority, 9)
		})
	}

	// Cover store != nil branch.
	d.store = &federationStoreStub{}
	_ = d.calculateDeliveryPriority(context.Background(), &activitypub.Activity{BaseObject: activitypub.BaseObject{Type: "Create"}}, "https://remote.example/inbox")
}

func TestDeliveryService_GenerateDeliveryID(t *testing.T) {
	id := generateDeliveryID()
	require.NotEmpty(t, id)
}

func TestDeliveryHelpers_RecipientDedupAndFollowersCollection(t *testing.T) {
	require.Nil(t, orderedUniqueRecipients(nil))

	recipients := orderedUniqueRecipients(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			To:   []string{"  https://remote.example/users/alice  ", activitypub.PublicAddress, "https://remote.example/users/alice"},
			CC:   []string{"", "https://remote.example/users/bob"},
			BTo:  []string{"https://remote.example/users/bob", "https://remote.example/users/carol"},
			BCC:  []string{"https://remote.example/users/carol", "https://remote.example/users/dave"},
			Type: activitypub.CreateType,
		},
	})
	require.Equal(t, []string{
		"https://remote.example/users/alice",
		"https://remote.example/users/bob",
		"https://remote.example/users/carol",
		"https://remote.example/users/dave",
	}, recipients)

	require.Empty(t, followersCollectionForActor(nil))
	require.False(t, isFollowersCollectionForActor(nil, "https://remote.example/users/alice/followers"))

	actorWithFollowers := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/alice"},
		Followers:  "https://remote.example/users/alice/custom-followers",
	}
	require.Equal(t, "https://remote.example/users/alice/custom-followers", followersCollectionForActor(actorWithFollowers))
	require.True(t, isFollowersCollectionForActor(actorWithFollowers, " https://remote.example/users/alice/custom-followers "))

	actorWithFallback := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/bob/"},
	}
	require.Equal(t, "https://remote.example/users/bob/followers", followersCollectionForActor(actorWithFallback))
	require.False(t, isFollowersCollectionForActor(actorWithFallback, "https://remote.example/users/alice/followers"))

	actorWithoutFollowers := &activitypub.Actor{}
	require.Empty(t, followersCollectionForActor(actorWithoutFollowers))
	require.False(t, isFollowersCollectionForActor(actorWithoutFollowers, "https://remote.example/users/bob/followers"))
}

func TestDeliveryService_ResolveDeliverableTarget_EdgeCases(t *testing.T) {
	store := &federationStoreStub{
		getActorFn: func(_ context.Context, username string) (*activitypub.Actor, error) {
			return &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/" + username, Type: "Person"},
				PreferredUsername: username,
			}, nil
		},
	}
	d := &DeliveryService{
		store:  store,
		logger: zap.NewNop(),
		cfg:    &appConfig.Config{Domain: "example.com"},
	}

	t.Run("rejects empty identifier", func(t *testing.T) {
		target, err := d.resolveDeliverableTarget(context.Background(), "   ")
		require.Error(t, err)
		require.Nil(t, target)
	})

	t.Run("rejects malformed actor url", func(t *testing.T) {
		target, err := d.resolveDeliverableTarget(context.Background(), "https://%zz")
		require.Error(t, err)
		require.Nil(t, target)
	})

	t.Run("rejects local actor url without username", func(t *testing.T) {
		target, err := d.resolveDeliverableTarget(context.Background(), "https://example.com/")
		require.Error(t, err)
		require.Nil(t, target)
		var notFound common.ActorNotFoundError
		require.ErrorAs(t, err, &notFound)
	})

	t.Run("rejects invalid handle format", func(t *testing.T) {
		target, err := d.resolveDeliverableTarget(context.Background(), "alice@remote@example.com")
		require.Error(t, err)
		require.Nil(t, target)
	})

	t.Run("resolves local handle as local target", func(t *testing.T) {
		target, err := d.resolveDeliverableTarget(context.Background(), "alice")
		require.NoError(t, err)
		require.NotNil(t, target)
		require.True(t, target.isLocal)
		require.Equal(t, "https://example.com/users/alice", target.actor.ID)
	})
}

func jsonMarshalStable(v any) ([]byte, error) {
	// Local helper to keep these tests focused; the exact JSON shape isn't critical.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}
