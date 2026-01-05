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
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestSignatureService_fetchActorPublicKeyWithPEM_ErrorBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("request_creation_error", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
			logger:     logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "http://[::1", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRequestCreationFailed)
	})

	t.Run("http_error", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("boom") }},
			logger:     logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "https://remote.example/users/alice", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFetchActorHTTPFailed)
	})

	t.Run("non_ok_status", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusInternalServerError, Status: "500", Body: io.NopCloser(strings.NewReader(""))}, nil
			}},
			logger: logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "https://remote.example/users/alice", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFetchActorHTTPStatusFailed)
	})

	t.Run("parse_error", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{"))}, nil
			}},
			logger: logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "https://remote.example/users/alice", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrActorUnmarshalFailed)
	})

	t.Run("missing_public_key", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: activitypub.PersonType},
					Inbox:      "https://remote.example/users/alice/inbox",
				}
				body, err := json.Marshal(actor)
				require.NoError(t, err)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
			}},
			logger: logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "https://remote.example/users/alice", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrActorHasNoPublicKey)
	})

	t.Run("invalid_public_key_pem", func(t *testing.T) {
		svc := &SignatureService{
			httpClient: &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) {
				actor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: activitypub.PersonType},
					Inbox:      "https://remote.example/users/alice/inbox",
					PublicKey: &activitypub.PublicKey{
						ID:           "https://remote.example/users/alice#main-key",
						PublicKeyPem: "not pem",
					},
				}
				body, err := json.Marshal(actor)
				require.NoError(t, err)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
			}},
			logger: logger,
		}

		_, _, _, _, err := svc.fetchActorPublicKeyWithPEM(ctx, "https://remote.example/users/alice", logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPublicKeyParseFailed)
	})
}

func TestSignatureService_fetchPublicKeyWithRetry_ContextCanceledDuringSleep(t *testing.T) {
	logger := zaptest.NewLogger(t)

	ctx, cancel := context.WithCancel(context.Background())

	svc := &SignatureService{
		publicKeyCacheRepo: &fakePublicKeyCacheRepo{getErr: errors.New("cache miss")},
		httpClient:         &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("boom") }},
		logger:             logger,
		sleep: func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	cancel()

	_, _, _, err := svc.fetchPublicKeyWithRetry(ctx, "https://remote.example/users/alice", logger)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSignatureService_verifyWithAlgorithm_ParseSignatureHeaderError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := &SignatureService{logger: logger}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/inbox", nil)
	req.Host = "example.com"

	err := svc.verifyWithAlgorithm(req, struct{}{}, AlgorithmHS2019)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureParseFailed)
}

func TestSignatureService_VerifySignature_UpdateStatsErrorIgnored(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKeyPEM, err := EncodePublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)

	actorURL := "https://remote.example/users/alice"
	keyID := actorURL + "#main-key"

	repo := &fakePublicKeyCacheRepo{
		cache: &models.PublicKeyCache{
			ActorURL:     actorURL,
			KeyID:        keyID,
			PublicKeyPEM: string(publicKeyPEM),
			Algorithm:    "rsa-sha256",
			TTL:          time.Now().Add(1 * time.Hour).Unix(),
		},
		updateErr: errors.New("update stats failed"),
	}

	svc := &SignatureService{
		publicKeyCacheRepo: repo,
		httpClient:         &httpDoerStub{doFn: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
		logger:             logger,
		sleep:              func(context.Context, time.Duration) error { return nil },
	}

	req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
	req.Host = "local.example"
	req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
	require.NoError(t, SignHTTPRequest(req, privateKey, keyID))

	require.NoError(t, svc.VerifySignature(ctx, req, actorURL))
}
