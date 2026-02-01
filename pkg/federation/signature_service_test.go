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
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakePublicKeyCacheRepo struct {
	cache  *models.PublicKeyCache
	getErr error

	invalidateCalls int
	invalidateErr   error

	storeCalls int
	storeErr   error
	storedPEM  string

	updateCalls []bool
	updateErr   error
}

func (f *fakePublicKeyCacheRepo) GetByActorURL(_ context.Context, _ string) (*models.PublicKeyCache, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.cache, nil
}

func (f *fakePublicKeyCacheRepo) InvalidateCache(_ context.Context, _ string) error {
	f.invalidateCalls++
	return f.invalidateErr
}

func (f *fakePublicKeyCacheRepo) Store(_ context.Context, _ string, _ string, publicKeyPEM string, _ string) (*models.PublicKeyCache, error) {
	f.storeCalls++
	f.storedPEM = publicKeyPEM
	if f.storeErr != nil {
		return nil, f.storeErr
	}
	return &models.PublicKeyCache{PublicKeyPEM: publicKeyPEM}, nil
}

func (f *fakePublicKeyCacheRepo) UpdateStats(_ context.Context, _ string, success bool) error {
	f.updateCalls = append(f.updateCalls, success)
	return f.updateErr
}

type fakeSignatureHTTPClient struct {
	do func(req *http.Request) (*http.Response, error)
}

func (f *fakeSignatureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f.do(req)
}

func TestSignatureService_VerifySignature_CachedAndFetchedKeys(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKeyPEM, err := EncodePublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)

	actorURL := "https://remote.example/users/alice"
	keyID := actorURL + "#main-key"

	makeRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "https://local.example/inbox", nil)
		req.Host = "local.example"
		req.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))
		require.NoError(t, SignHTTPRequest(req, privateKey, keyID))
		return req
	}

	t.Run("cache_hit_uses_basic_verification", func(t *testing.T) {
		repo := &fakePublicKeyCacheRepo{
			cache: &models.PublicKeyCache{
				ActorURL:     actorURL,
				KeyID:        keyID,
				PublicKeyPEM: string(publicKeyPEM),
				Algorithm:    "rsa-sha256",
				TTL:          time.Now().Add(1 * time.Hour).Unix(),
			},
		}

		svc := &SignatureService{
			publicKeyCacheRepo: repo,
			httpClient:         &fakeSignatureHTTPClient{do: func(_ *http.Request) (*http.Response, error) { return nil, errors.New("unexpected") }},
			logger:             logger,
			sleep:              func(context.Context, time.Duration) error { return nil },
		}

		req := makeRequest()
		require.NoError(t, svc.VerifySignature(ctx, req, actorURL))
		require.Len(t, repo.updateCalls, 1)
		assert.True(t, repo.updateCalls[0])
	})

	t.Run("invalid_cached_pem_is_invalidated_then_fetched_and_cached", func(t *testing.T) {
		repo := &fakePublicKeyCacheRepo{
			cache: &models.PublicKeyCache{
				ActorURL:     actorURL,
				KeyID:        keyID,
				PublicKeyPEM: "not pem",
				Algorithm:    "hs2019",
				TTL:          time.Now().Add(1 * time.Hour).Unix(),
			},
		}

		actorDoc := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   actorURL,
				Type: activitypub.PersonType,
			},
			PublicKey: &activitypub.PublicKey{
				ID:           keyID,
				PublicKeyPem: string(publicKeyPEM),
			},
		}
		actorDocJSON, err := json.Marshal(actorDoc)
		require.NoError(t, err)

		httpClient := &fakeSignatureHTTPClient{do: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != actorURL {
				return nil, errors.New("unexpected url")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/activity+json"}},
				Body:       io.NopCloser(bytes.NewReader(actorDocJSON)),
			}, nil
		}}

		svc := &SignatureService{
			publicKeyCacheRepo: repo,
			httpClient:         httpClient,
			logger:             logger,
			sleep:              func(context.Context, time.Duration) error { return nil },
		}

		req := makeRequest()
		require.NoError(t, svc.VerifySignature(ctx, req, actorURL))
		assert.Equal(t, 1, repo.invalidateCalls)
		assert.Equal(t, 1, repo.storeCalls)
		assert.Contains(t, repo.storedPEM, "BEGIN PUBLIC KEY")
	})

	t.Run("fetch_retries_exhausted_returns_authentication_error", func(t *testing.T) {
		repo := &fakePublicKeyCacheRepo{getErr: errors.New("cache miss")}
		httpClient := &fakeSignatureHTTPClient{do: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(""))}, nil
		}}

		svc := &SignatureService{
			publicKeyCacheRepo: repo,
			httpClient:         httpClient,
			logger:             logger,
			sleep:              func(context.Context, time.Duration) error { return nil },
		}

		req := makeRequest()
		err := svc.VerifySignature(ctx, req, actorURL)
		require.Error(t, err)

		var authErr common.AuthenticationError
		assert.ErrorAs(t, err, &authErr)
	})
}

func TestSignatureService_VerifyDigestWithCompatibility(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := &SignatureService{logger: logger}

	body := []byte("hello")
	digest := calculateDigest(body) // "SHA-256=..."

	req := httptest.NewRequest(http.MethodPost, "https://local.example/inbox", bytes.NewReader(body))
	req.Header.Set("Digest", digest)
	require.NoError(t, svc.VerifyDigestWithCompatibility(req, body))

	req = httptest.NewRequest(http.MethodPost, "https://local.example/inbox", bytes.NewReader(body))
	req.Header.Set("Digest", "sha-256="+strings.SplitN(digest, "=", 2)[1])
	require.NoError(t, svc.VerifyDigestWithCompatibility(req, body))

	req = httptest.NewRequest(http.MethodPost, "https://local.example/inbox", bytes.NewReader(body))
	req.Header.Set("Digest", "sha-256=bad")
	err := svc.VerifyDigestWithCompatibility(req, body)
	require.Error(t, err)
}

func TestDetermineAlgorithm(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	assert.Equal(t, AlgorithmHS2019, determineAlgorithm(&privateKey.PublicKey))
	assert.Equal(t, AlgorithmRSASHA256, determineAlgorithm(struct{}{}))
}
