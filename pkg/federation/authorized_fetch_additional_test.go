package federation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestAuthorizedFetchService_FetchObject_ErrorBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	privateKeyPEM, err := EncodePrivateKeyPEM(privateKey)
	require.NoError(t, err)

	signingActor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://local.example/users/alice"},
		PreferredUsername: "alice",
		PublicKey: &activitypub.PublicKey{
			ID: "https://local.example/users/alice#main-key",
		},
	}

	t.Run("request_creation_failed", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, nil }},
		}

		_, err := svc.FetchObject(ctx, "http://[::1", signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRequestCreationFailed)
	})

	t.Run("repository_access_validation_failed", func(t *testing.T) {
		actor := *signingActor
		actor.PreferredUsername = ""

		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, nil }},
		}

		_, err := svc.FetchObject(ctx, "https://remote.example/objects/1", &actor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRepositoryAccessValidationFailed)
	})

	t.Run("private_key_parse_failed", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: "not pem"},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, nil }},
		}

		_, err := svc.FetchObject(ctx, "https://remote.example/objects/1", signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivateKeyParseFailed)
	})

	t.Run("http_request_failed", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient:   &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("boom") }},
		}

		_, err := svc.FetchObject(ctx, "https://remote.example/objects/1", signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrHTTPRequestFailed)
	})

	t.Run("object_missing_type", func(t *testing.T) {
		objectURL := "https://remote.example/objects/123"
		svc := &AuthorizedFetchService{
			actorRepo:    &fakeAuthorizedFetchActorRepo{privateKeyPEM: string(privateKeyPEM)},
			instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
			logger:       logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return httpJSONResponse(http.StatusOK, map[string]any{"id": objectURL}), nil
			}},
		}

		_, err := svc.FetchObject(ctx, objectURL, signingActor)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrObjectValidationFailed)
	})
}

func TestAuthorizedFetchService_VerifyAuthorizedFetch_SignatureVerificationFailed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	signerKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	otherKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	otherPublicPEM, err := EncodePublicKeyPEM(&otherKey.PublicKey)
	require.NoError(t, err)

	actorID := "https://remote.example/users/alice"
	keyID := actorID + "#main-key"

	svc := &AuthorizedFetchService{
		actorRepo:    &fakeAuthorizedFetchActorRepo{},
		instanceRepo: &fakeAuthorizedFetchInstanceRepo{},
		logger:       logger,
		httpClient: &fakeHTTPDoer{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == actorID {
				return httpJSONResponse(http.StatusOK, &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   actorID,
						Type: activitypub.PersonType,
					},
					PublicKey: &activitypub.PublicKey{
						ID:           keyID,
						PublicKeyPem: string(otherPublicPEM),
					},
				}), nil
			}
			return nil, errors.New("unexpected url")
		}},
	}

	req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
	req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
	req.Host = "local.example"
	require.NoError(t, SignHTTPRequest(req, signerKey, keyID))

	_, err = svc.VerifyAuthorizedFetch(ctx, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureVerificationFailed)
}

func TestAuthorizedFetchService_fetchActorWithoutAuth_ErrorBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	actorURL := "https://remote.example/users/alice"

	t.Run("http_error", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			logger:     logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("boom") }},
		}

		_, err := svc.fetchActorWithoutAuth(ctx, actorURL)
		require.Error(t, err)
	})

	t.Run("non_ok_status", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			logger: logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(""))}, nil
			}},
		}

		_, err := svc.fetchActorWithoutAuth(ctx, actorURL)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFetchActorHTTPFailed)
	})

	t.Run("parse_error", func(t *testing.T) {
		svc := &AuthorizedFetchService{
			logger: logger,
			httpClient: &fakeHTTPDoer{do: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{"))}, nil
			}},
		}

		_, err := svc.fetchActorWithoutAuth(ctx, actorURL)
		require.Error(t, err)
	})
}
