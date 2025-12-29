package lift

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAccountsHelpers(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	h := &Handler{cfg: cfg, logger: logger}

	data := base64.StdEncoding.EncodeToString([]byte("body"))
	decoded, err := h.handleBase64Decoding([]byte(data))
	require.NoError(t, err)
	require.Equal(t, []byte("body"), decoded)

	raw := []byte("------WebKitFormBoundaryXYZ")
	decoded, err = h.handleBase64Decoding(raw)
	require.NoError(t, err)
	require.Equal(t, raw, decoded)

	_, err = h.handleBase64Decoding([]byte("not-multipart"))
	require.Error(t, err)

	boundary, err := h.extractBoundary("multipart/form-data; boundary=abc123")
	require.NoError(t, err)
	require.Equal(t, "abc123", boundary)
	_, err = h.extractBoundary("invalid")
	require.Error(t, err)

	err = h.validateRegistrationRequestLift(apimodels.AccountRegistrationRequest{
		Username:                 "alice",
		Agreement:                false,
		DefaultPostingVisibility: storagemodels.VisibilityPublic,
	})
	require.Error(t, err)

	err = h.validateRegistrationRequestLift(apimodels.AccountRegistrationRequest{
		Username:                 "alice",
		Agreement:                true,
		DefaultPostingVisibility: "bad",
	})
	require.Error(t, err)

	now := time.Now()
	require.Equal(t, now.Format(time.RFC3339), h.formatActorCreatedTimeLift(&now))
	require.NotEmpty(t, h.formatActorCreatedTimeLift(nil))

	require.Equal(t, "", h.getHeaderURLLift(&activitypub.Actor{}))
	require.Equal(t, "https://example.com/header.jpg", h.getHeaderURLLift(&activitypub.Actor{
		Image: &activitypub.Image{URL: "https://example.com/header.jpg"},
	}))
}

func TestAccountsHandlers_SimpleFlows(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	headers := map[string]string{"Authorization": "Bearer " + token}

	account := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			PreferredUsername: "alice",
		},
	}

	registry := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return account, nil
			},
			LookupAccountFunc: func(ctx context.Context, query *accounts.LookupAccountQuery) (*storage.Account, error) {
				return account, nil
			},
			GetFamiliarFollowersFunc: func(ctx context.Context, query *accounts.GetFamiliarFollowersQuery) (*accounts.FamiliarFollowersResult, error) {
				return &accounts.FamiliarFollowersResult{
					Results: []accounts.FamiliarFollowersForAccount{
						{
							ID: query.AccountIDs[0],
							Accounts: []*storage.Account{
								account,
							},
						},
					},
				}, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			GetFollowersFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{account}, "next-cursor", nil
			},
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{account}, "", nil
			},
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) {
				return 1, nil
			},
			CountFollowingFunc: func(ctx context.Context, username string) (int64, error) {
				return 0, nil
			},
		},
	}

	h := &Handler{
		cfg:      cfg,
		logger:   logger,
		registry: registry,
		repos:    &MockRepositoryStorage{},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice", nil, nil, nil)
	require.NoError(t, err)
	ctx.SetParam("id", "alice")
	require.NoError(t, h.HandleGetAccountLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/lookup", nil, map[string]string{"acct": "alice@example.com"}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleAccountLookupLift(ctx2))
	require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/followers", nil, map[string]string{"max_id": "1"}, nil)
	require.NoError(t, err)
	ctx3.SetParam("id", "alice")
	require.NoError(t, h.HandleGetAccountFollowersLift(ctx3))
	require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)
	require.Contains(t, ctx3.Response.Headers["Link"], "next-cursor")

	ctx4, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/following", nil, nil, nil)
	require.NoError(t, err)
	ctx4.SetParam("id", "alice")
	require.NoError(t, h.HandleGetAccountFollowingLift(ctx4))
	require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)

	ctx5, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/familiar_followers", headers, map[string]string{"id[]": "alice"}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetFamiliarFollowersLift(ctx5))
	require.Equal(t, http.StatusOK, ctx5.Response.StatusCode)

	apActor := &activitypub.Actor{
		BaseObject:                activitypub.BaseObject{ID: "https://example.com/users/alice"},
		PreferredUsername:         "alice",
		ManuallyApprovesFollowers: true,
	}
	ctx6, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers", nil, nil, nil)
	require.NoError(t, err)
	ctx6.SetParam("username", "alice")
	require.NoError(t, h.returnActivityPubCollection(ctx6, apActor, "followers"))
	require.Equal(t, http.StatusOK, ctx6.Response.StatusCode)
}
