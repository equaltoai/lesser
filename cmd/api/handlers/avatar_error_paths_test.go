package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingAvatarDeleteStore delegates every operation to the in-memory walkthrough
// store except delete, which always fails. That makes the best-effort orphan
// cleanup in the avatar handlers observable without failing the request itself.
type failingAvatarDeleteStore struct {
	*avatarTestS3Store
}

func (failingAvatarDeleteStore) DeleteFile(context.Context, string, string) error {
	return errors.New("object store delete failed")
}

// newAvatarMediaStubHandler builds a handler whose media service is a stub, so a
// serve-path failure can be driven directly.
func newAvatarMediaStubHandler(t *testing.T, mediaSvc MediaService) *Handler {
	t.Helper()

	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
	handler.registry = &RegistryStub{MediaSvc: mediaSvc}
	return handler
}

// TestUploadAvatarRejectsAMalformedRequest covers the parse failures that are not
// a missing file part. An empty body is the caller omitting the upload (422),
// while a body the handler cannot read as multipart is a malformed request (400).
func TestUploadAvatarRejectsAMalformedRequest(t *testing.T) {
	handler, store, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	t.Run("empty body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar",
			map[string]string{"Authorization": "Bearer " + token}, nil, nil,
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	})

	t.Run("content type without a boundary", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar",
			map[string]string{
				"Authorization": "Bearer " + token,
				"Content-Type":  "multipart/form-data",
			}, nil, avatarUploadBody(t, "me.png"),
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.Status)
		assert.Contains(t, string(resp.Body), "failed to parse avatar upload")
	})

	t.Run("body that is neither base64 nor multipart", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar",
			map[string]string{
				"Authorization": "Bearer " + token,
				"Content-Type":  "multipart/form-data; boundary=" + avatarTestBoundary,
			}, nil, []byte("totally not a multipart body"),
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.Status)
	})

	assert.Empty(t, store.objects, "a rejected upload must never store an object")
}

// TestUploadAvatarRejectsATruncatedFilePart pins the reader-failure path: a part
// whose bytes end mid-flight yields no file data, so the upload is rejected as
// missing rather than stored half-read.
func TestUploadAvatarRejectsATruncatedFilePart(t *testing.T) {
	handler, store, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	body := avatarUploadBody(t, "me.png")
	truncated := body[:len(body)-16]
	require.Less(t, len(truncated), len(body), "the truncated body must actually be shorter")

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, truncated,
	)
	resp, err := handler.HandleUploadAvatarLift(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	assert.Empty(t, store.objects, "a truncated upload must never store an object")
}

// TestUploadAvatarRollsBackTheStoredObjectWhenTheAccountWriteFails pins the
// rollback: once the bytes are stored, a failure to attach them to the account
// must not leave the object behind, and a failure of the rollback itself must not
// change the reported outcome.
func TestUploadAvatarRollsBackTheStoredObjectWhenTheAccountWriteFails(t *testing.T) {
	t.Run("the stored object is cleaned up", func(t *testing.T) {
		handler, store, accountsStore := newAvatarWalkthroughHandler(t)
		cfg := round11TestConfig()
		token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

		accountsStore.SetAvatarFunc = func(context.Context, *accounts.SetAvatarCommand) (*accounts.SetAvatarResult, error) {
			return nil, accounts.ErrStoreAccount
		}

		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, avatarUploadBody(t, "me.png"),
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
		assert.Empty(t, store.objects, "an avatar whose account write failed must not survive as an orphan")
	})

	t.Run("a failing cleanup does not change the outcome", func(t *testing.T) {
		handler, store, accountsStore := newAvatarWalkthroughHandler(t)
		cfg := round11TestConfig()
		token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

		mediaSvc, ok := handler.registry.Media().(*media.Service)
		require.True(t, ok)
		mediaSvc.SetS3Service(failingAvatarDeleteStore{store})

		accountsStore.SetAvatarFunc = func(context.Context, *accounts.SetAvatarCommand) (*accounts.SetAvatarResult, error) {
			return nil, accounts.ErrStoreAccount
		}

		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, avatarUploadBody(t, "me.png"),
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("a failed profile projection after a successful attach", func(t *testing.T) {
		handler, store, accountsStore := newAvatarWalkthroughHandler(t)
		cfg := round11TestConfig()
		token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

		accountsStore.SetAvatarFunc = func(context.Context, *accounts.SetAvatarCommand) (*accounts.SetAvatarResult, error) {
			// No account to project: the transformation cannot succeed.
			return &accounts.SetAvatarResult{}, nil
		}

		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, avatarUploadBody(t, "me.png"),
		)
		resp, err := handler.HandleUploadAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.Status)
		assert.NotEmpty(t, store.objects,
			"the attach succeeded, so the object stays referenced; only the projection failed")
	})
}

// TestClearAvatarRequiresWriteScope closes the gap left by the upload-scope test:
// the delete route carries the same write-scope requirement, so a read-scoped
// token is forbidden and an anonymous caller is unauthorized.
func TestClearAvatarRequiresWriteScope(t *testing.T) {
	handler, _, _ := newAvatarWalkthroughHandler(t)
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodDelete, "/api/v1/accounts/avatar",
		map[string]string{"Authorization": "Bearer " + readToken}, nil, nil,
	)
	resp, err := handler.HandleClearAvatarLift(ctx)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.Status)

	anon := round10NewLiftContextWithBodyBytes(http.MethodDelete, "/api/v1/accounts/avatar", nil, nil, nil)
	anonResp, err := handler.HandleClearAvatarLift(anon)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, anonResp.Status)
}

// TestClearAvatarReportsFailures covers the two ways a clear can fail after
// authentication: the service refuses the write, or the refreshed account cannot
// be projected back as a Mastodon account.
func TestClearAvatarReportsFailures(t *testing.T) {
	t.Run("a failed clear", func(t *testing.T) {
		handler, _, accountsStore := newAvatarWalkthroughHandler(t)
		cfg := round11TestConfig()
		token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

		accountsStore.ClearAvatarFunc = func(context.Context, *accounts.ClearAvatarCommand) (*accounts.ClearAvatarResult, error) {
			return nil, accounts.ErrClearAvatar
		}

		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil,
		)
		resp, err := handler.HandleClearAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("a failed profile projection", func(t *testing.T) {
		handler, _, accountsStore := newAvatarWalkthroughHandler(t)
		cfg := round11TestConfig()
		token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

		accountsStore.ClearAvatarFunc = func(context.Context, *accounts.ClearAvatarCommand) (*accounts.ClearAvatarResult, error) {
			return &accounts.ClearAvatarResult{}, nil
		}

		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodDelete, "/api/v1/accounts/avatar", avatarUploadHeaders(token), nil, nil,
		)
		resp, err := handler.HandleClearAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}

// TestGetAvatarDistinguishesAStoreFailureFromAnAbsentAvatar is the difference
// between "no such avatar" and "the avatar store is broken". Only the former is a
// 404; hiding a broken store behind a 404 would make an outage look like a
// deleted avatar.
func TestGetAvatarDistinguishesAStoreFailureFromAnAbsentAvatar(t *testing.T) {
	const id = "550e8400-e29b-41d4-a716-446655440000"

	t.Run("an unavailable store", func(t *testing.T) {
		handler := newAvatarMediaStubHandler(t, &MediaServiceStub{
			GetAvatarFunc: func(context.Context, string) ([]byte, string, error) {
				return nil, "", media.ErrAvatarStoreUnavailable
			},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/"+id, nil, nil, nil)
		ctx.Params["id"] = id

		resp, err := handler.HandleGetAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("an unexpected read failure", func(t *testing.T) {
		handler := newAvatarMediaStubHandler(t, &MediaServiceStub{
			GetAvatarFunc: func(context.Context, string) ([]byte, string, error) {
				return nil, "", errors.Join(media.ErrMediaRetrievalFailed, errors.New("kms access denied"))
			},
		})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/"+id, nil, nil, nil)
		ctx.Params["id"] = id

		resp, err := handler.HandleGetAvatarLift(ctx)
		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}

// TestRespondAvatarUploadErrorMapsEveryServiceFailure pins the status each
// store-layer refusal becomes at the HTTP boundary. Unsupported bytes are the
// caller's mistake (422), an oversized file is a 413, and everything else —
// including a store that is not configured at all — is a server-side failure.
func TestRespondAvatarUploadErrorMapsEveryServiceFailure(t *testing.T) {
	handler, _, _ := newAvatarWalkthroughHandler(t)
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/accounts/avatar", nil, nil, nil)

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "too large", err: media.ErrAvatarTooLarge, want: http.StatusRequestEntityTooLarge},
		{name: "no file data", err: media.ErrAvatarRequired, want: http.StatusUnprocessableEntity},
		{name: "unsupported image type", err: media.ErrAvatarContentTypeNotAllowed, want: http.StatusUnprocessableEntity},
		{name: "store unavailable", err: media.ErrAvatarStoreUnavailable, want: http.StatusInternalServerError},
		{name: "wrapped store unavailable", err: errors.Join(errors.New("store"), media.ErrAvatarStoreUnavailable), want: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handler.respondAvatarUploadError(ctx, "alice", tc.err)
			require.NoError(t, err)
			assert.Equal(t, tc.want, resp.Status)
		})
	}
}

// TestAvatarIDParamToleratesAMissingContext keeps the serve-route lookup total:
// an absent context yields no id, which the store rejects as an invalid id rather
// than panicking inside the handler.
func TestAvatarIDParamToleratesAMissingContext(t *testing.T) {
	assert.Empty(t, avatarIDParam(nil))

	ctx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/api/v1/avatars/abc", nil, nil, nil)
	ctx.Params["id"] = "abc"
	assert.Equal(t, "abc", avatarIDParam(ctx))
}

// TestUpdateCredentialsFullMirrorsTheAvatarURLRejection pins the second
// update_credentials implementation to the same ruling as the Lift handler: an
// avatar URL is refused with a pointer to the upload route and never reaches the
// profile service, so neither credentials path can take an avatar URL from a
// caller.
func TestUpdateCredentialsFullMirrorsTheAvatarURLRejection(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	updateProfileHits := 0
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			UpdateProfileFunc: func(context.Context, *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
				updateProfileHits++
				return &accounts.AccountResult{}, nil
			},
		},
	}

	headers := map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "alice")}
	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil,
		[]byte(`{"avatar":"https://example.com/api/v1/avatars/11111111-2222-4333-8444-555555555555"}`),
	)

	resp, err := handler.HandleUpdateCredentialsFull(ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.Status)
	assert.Contains(t, string(resp.Body), errAvatarURLNotAccepted)
	assert.Contains(t, string(resp.Body), "POST /api/v1/accounts/avatar")
	assert.Zero(t, updateProfileHits, "a rejected avatar parameter must not reach the profile service")
}
