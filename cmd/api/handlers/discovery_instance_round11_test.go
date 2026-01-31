package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryAndInstanceHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	state := &round10QueryState{
		actorList: []storagemodels.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: "Person"},
					PreferredUsername: "bob",
				},
			},
		},
		instanceRules: []storagemodels.InstanceRule{
			{ID: "1", Text: "Be nice"},
		},
		vapidKeys: &storage.VAPIDKeys{
			PublicKey:  "pubkey",
			PrivateKey: "privkey",
			Subject:    "mailto:admin@example.com",
			CreatedAt:  now.Add(-24 * time.Hour),
			UpdatedAt:  now,
		},
		domainBlocks: []storagemodels.InstanceDomainBlock{
			{Domain: "bad.example", Severity: "suspend", PublicComment: "bad", Obfuscate: false},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		SearchSvc: &SearchServiceStub{
			GetDirectoryFunc: func(ctx context.Context, query *search.DirectoryQuery) (*search.DirectoryResult, error) {
				return &search.DirectoryResult{
					Accounts: []search.AccountResult{
						{
							Actor:          &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}, PreferredUsername: "alice", Name: "Alice"},
							FollowersCount: 3,
							FollowingCount: 2,
							StatusesCount:  5,
						},
					},
				}, nil
			},
			GetSuggestionsFunc: func(ctx context.Context, query *search.SuggestionsQuery) (*search.SuggestionsResult, error) {
				return &search.SuggestionsResult{
					Suggestions: []search.SuggestionItem{
						{
							Account: search.AccountResult{
								Actor:          &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob", Name: "Bob"},
								FollowersCount: 1,
							},
							Source: "staff",
						},
					},
				}, nil
			},
			RemoveSuggestionFunc: func(ctx context.Context, cmd *search.RemoveSuggestionCommand) error {
				return nil
			},
		},
	}

	ctxDir, err := round10NewLiftContext(http.MethodGet, "/api/v1/directory", nil, map[string]string{"order": "active", "limit": "1"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetDirectoryLift(ctxDir))
	require.Equal(t, http.StatusOK, ctxDir.Response.StatusCode)

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	ctxSugg1, err := round10NewLiftContext(http.MethodGet, "/api/v1/suggestions", readHeaders, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetSuggestionsV1Lift(ctxSugg1))
	require.Equal(t, http.StatusOK, ctxSugg1.Response.StatusCode)

	ctxSugg2, err := round10NewLiftContext(http.MethodGet, "/api/v2/suggestions", readHeaders, map[string]string{"limit": "1"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetSuggestionsV2Lift(ctxSugg2))
	require.Equal(t, http.StatusOK, ctxSugg2.Response.StatusCode)

	ctxRemove, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxRemove.SetParam("account_id", "bob")
	require.NoError(t, handler.HandleRemoveSuggestionLift(ctxRemove))
	require.Equal(t, http.StatusOK, ctxRemove.Response.StatusCode)

	require.True(t, handler.isLocalLift("https://example.com/users/alice"))
	require.Equal(t, "alice@remote.example", handler.getAccountAcctLift(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/alice"}, PreferredUsername: "alice"}))

	ctxInstance, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstanceV1Lift(ctxInstance))
	require.Equal(t, http.StatusOK, ctxInstance.Response.StatusCode)

	ctxPeers, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/peers", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstancePeersLift(ctxPeers))
	require.Equal(t, http.StatusOK, ctxPeers.Response.StatusCode)

	ctxActivity, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/activity", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstanceActivityLift(ctxActivity))
	require.Equal(t, http.StatusOK, ctxActivity.Response.StatusCode)

	ctxBlocks, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/domain_blocks", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstanceDomainBlocksLift(ctxBlocks))
	require.Equal(t, http.StatusOK, ctxBlocks.Response.StatusCode)

	ctxPrivacy, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/privacy_policy", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstancePrivacyPolicyLift(ctxPrivacy))
	require.Equal(t, http.StatusOK, ctxPrivacy.Response.StatusCode)

	ctxTerms, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance/terms", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetInstanceTermsOfServiceLift(ctxTerms))
	require.Equal(t, http.StatusOK, ctxTerms.Response.StatusCode)

	html := handler.markdownToHTMLLift("# Title\n\nParagraph")
	require.Contains(t, html, "<h1>Title</h1>")
}
