package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeAuthorizedFetchActorRepo struct {
	privateKeyPEM string
	err           error
}

func (f *fakeAuthorizedFetchActorRepo) GetActorPrivateKey(_ context.Context, username string) (string, error) {
	_ = username
	if f.err != nil {
		return "", f.err
	}
	return f.privateKeyPEM, nil
}

type fakeAuthorizedFetchInstanceRepo struct {
	rules []storage.InstanceRule
	err   error
}

func (f *fakeAuthorizedFetchInstanceRepo) GetInstanceRules(_ context.Context) ([]storage.InstanceRule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rules, nil
}

type fakeHTTPDoer struct {
	do func(req *http.Request) (*http.Response, error)
}

func (f *fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return f.do(req)
}

func httpJSONResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestAuthorizedFetchService_FetchObject_SuccessAndValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	objectURL := "https://remote.example/objects/123"
	signingActor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://local.example/users/alice"},
		PreferredUsername: "alice",
		PublicKey: &activitypub.PublicKey{
			ID: "https://local.example/users/alice#main-key",
		},
	}

	t.Run("success", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, objectURL, req.URL.String())
				return httpJSONResponse(http.StatusOK, map[string]any{
					"id":   objectURL,
					"type": "Note",
				}), nil
			}},
		}

		obj, err := svc.FetchObject(ctx, objectURL, signingActor)
		require.NoError(t, err)
		result, ok := obj.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, objectURL, result["id"])
		assert.Equal(t, "Note", result["type"])
	})

	t.Run("non_200_status", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return httpJSONResponse(http.StatusNotFound, map[string]any{"error": "nope"}), nil
			}},
		}

		_, err := svc.FetchObject(ctx, objectURL, signingActor)
		assert.ErrorIs(t, err, ErrFetchObjectHTTPFailed)
	})

	t.Run("decode_error", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader("{not json")),
				}, nil
			}},
		}

		_, err := svc.FetchObject(ctx, objectURL, signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrResponseDecodeFailed)
	})

	t.Run("validation_error_id_mismatch", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return httpJSONResponse(http.StatusOK, map[string]any{
					"id":   "https://remote.example/objects/other",
					"type": "Note",
				}), nil
			}},
		}

		_, err := svc.FetchObject(ctx, objectURL, signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrObjectValidationFailed)
	})

	t.Run("private_key_retrieval_error", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{err: errors.New("no key")},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, nil }},
		}

		_, err := svc.FetchObject(ctx, objectURL, signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateKeyRetrievalFailed)
	})
}

func TestAuthorizedFetchService_FetchActor_TypeValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	actorURL := "https://remote.example/users/bob"
	signingActor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://local.example/users/alice"},
		PreferredUsername: "alice",
		PublicKey:         &activitypub.PublicKey{ID: "https://local.example/users/alice#main-key"},
	}

	t.Run("valid_actor_type", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return httpJSONResponse(http.StatusOK, map[string]any{
					"id":   actorURL,
					"type": activitypub.PersonType,
				}), nil
			}},
		}

		actor, err := svc.FetchActor(ctx, actorURL, signingActor)
		require.NoError(t, err)
		assert.Equal(t, actorURL, actor.ID)
	})

	t.Run("invalid_actor_type", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return httpJSONResponse(http.StatusOK, map[string]any{
					"id":   actorURL,
					"type": "NotAnActor",
				}), nil
			}},
		}

		_, err := svc.FetchActor(ctx, actorURL, signingActor)
		assert.ErrorIs(t, err, ErrNotActorObject)
	})
}

func TestAuthorizedFetchService_VerifyAuthorizedFetch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKeyPEM, err := EncodePublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)

	actorID := "https://remote.example/users/alice"
	keyID := actorID + "#main-key"
	actorDoc := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: activitypub.PersonType,
		},
		PublicKey: &activitypub.PublicKey{
			ID:           keyID,
			PublicKeyPem: string(publicKeyPEM),
		},
	}

	makeService := func(do func(req *http.Request) (*http.Response, error)) *AuthorizedFetchService {
		return &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: ""},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: do},
		}
	}

	t.Run("missing_signature_header", func(t *testing.T) {
		svc := makeService(func(_ *http.Request) (*http.Response, error) { return nil, nil })
		req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
		_, err := svc.VerifyAuthorizedFetch(ctx, req)
		assert.ErrorIs(t, err, ErrMissingSignatureHeader)
	})

	t.Run("parse_error", func(t *testing.T) {
		svc := makeService(func(_ *http.Request) (*http.Response, error) { return nil, nil })
		req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
		req.Header.Set("Signature", "not a signature header")
		_, err := svc.VerifyAuthorizedFetch(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSignatureParseFailed)
	})

	t.Run("success", func(t *testing.T) {
		svc := makeService(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == actorID {
				return httpJSONResponse(http.StatusOK, actorDoc), nil
			}
			return nil, errors.New("unexpected url")
		})

		req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
		req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
		req.Host = "local.example"
		require.NoError(t, SignHTTPRequest(req, privateKey, keyID))

		actor, err := svc.VerifyAuthorizedFetch(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, actorID, actor.ID)
	})

	t.Run("public_key_parse_error", func(t *testing.T) {
		badActorDoc := *actorDoc
		badActorDoc.PublicKey = &activitypub.PublicKey{ID: keyID, PublicKeyPem: "not pem"}

		svc := makeService(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == actorID {
				return httpJSONResponse(http.StatusOK, &badActorDoc), nil
			}
			return nil, errors.New("unexpected url")
		})

		req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
		req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
		req.Host = "local.example"
		require.NoError(t, SignHTTPRequest(req, privateKey, keyID))

		_, err := svc.VerifyAuthorizedFetch(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPublicKeyParseFailed)
	})
}

func TestAuthorizedFetchService_IsAuthorizedFetchEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Ensure we restore the global config after this test.
	cfg := appConfig.Get()
	orig := cfg.AuthorizedFetchEnabled
	t.Cleanup(func() { cfg.AuthorizedFetchEnabled = orig })

	t.Run("enabled_via_config", func(t *testing.T) {
		cfg.AuthorizedFetchEnabled = true
		svc := &AuthorizedFetchService{
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{rules: nil},
			logger:       logger,
		}
		assert.True(t, svc.IsAuthorizedFetchEnabled(ctx))
	})

	t.Run("enabled_via_instance_rules", func(t *testing.T) {
		cfg.AuthorizedFetchEnabled = false
		svc := &AuthorizedFetchService{
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{rules: []storage.InstanceRule{
				{ID: "authorized_fetch_enabled", Text: "true"},
			}},
			logger: logger,
		}
		assert.True(t, svc.IsAuthorizedFetchEnabled(ctx))
	})

	t.Run("disabled_on_repo_error", func(t *testing.T) {
		cfg.AuthorizedFetchEnabled = false
		svc := &AuthorizedFetchService{
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{err: errors.New("db")},
			logger:       logger,
		}
		assert.False(t, svc.IsAuthorizedFetchEnabled(ctx))
	})
}

func TestExtractActorIDFromKeyID(t *testing.T) {
	assert.Equal(t, "https://example.com/users/alice", extractActorIDFromKeyID("https://example.com/users/alice#main-key"))
	assert.Equal(t, "https://example.com/users/alice", extractActorIDFromKeyID("https://example.com/users/alice"))
}
