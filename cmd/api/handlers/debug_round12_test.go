package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestDebugAuthRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("stage forbidden", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin"})
		headers := map[string]string{"Authorization": "Bearer " + adminToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = "a1"

		requireStatus(t, http.StatusForbidden)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("missing token", func(t *testing.T) {
		cfg2 := round11TestConfig()
		cfg2.Stage = "test"
		h, _, _ := round11NewHandler(t, cfg2, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = "a1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("invalid token", func(t *testing.T) {
		cfg2 := round11TestConfig()
		cfg2.Stage = "test"
		h, _, _ := round11NewHandler(t, cfg2, &round10QueryState{})

		headers := map[string]string{"Authorization": "Bearer bad-token"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = "a1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("insufficient scope", func(t *testing.T) {
		cfg2 := round11TestConfig()
		cfg2.Stage = "test"
		h, _, _ := round11NewHandler(t, cfg2, &round10QueryState{})

		readToken := round11SignAccessToken(t, cfg2.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = "a1"

		requireStatus(t, http.StatusForbidden)(h.HandleDebugFederationTraceLift(ctx))
	})
}

func TestDebugHandlersRound12(t *testing.T) {
	cfg := round11TestConfig()
	cfg.Stage = "test"

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin"})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	now := time.Now().UTC()
	localActivityID := "act-local"
	remoteActivityID := "act-remote"

	localPublished := now.Add(-2 * time.Minute)
	remotePublished := now.Add(-3 * time.Minute)

	state := &round10QueryState{
		activitiesByID: map[string]*storagemodels.Activity{
			localActivityID: {
				Activity: &activitypub.Activity{
					BaseObject: activitypub.BaseObject{
						ID:        localActivityID,
						Type:      "Create",
						Published: &localPublished,
					},
					Actor: "https://example.com/users/alice",
				},
				CreatedAt: localPublished,
			},
			remoteActivityID: {
				Activity: &activitypub.Activity{
					BaseObject: activitypub.BaseObject{
						ID:        remoteActivityID,
						Type:      "Create",
						Published: &remotePublished,
					},
					Actor: "https://remote.example/users/bob",
					Object: map[string]any{
						"to": []any{"https://example.com/users/alice"},
					},
				},
				CreatedAt: remotePublished,
			},
		},
		objectsByID: map[string]storagemodels.Object{
			"obj-1": {
				ID:           "obj-1",
				Type:         "Article",
				AttributedTo: "https://example.com/users/alice",
				Content:      "hello",
				Published:    now.Add(-10 * time.Minute),
				Updated:      now.Add(-5 * time.Minute),
				CreatedAt:    now.Add(-10 * time.Minute),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	t.Run("federation trace missing activity id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace", headers, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("federation trace activity not found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("federation trace local activity", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace/"+localActivityID, headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = localActivityID

		requireStatus(t, http.StatusOK)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("federation trace remote activity", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/trace/"+remoteActivityID, headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = remoteActivityID

		requireStatus(t, http.StatusOK)(h.HandleDebugFederationTraceLift(ctx))
	})

	t.Run("replay remote activity rejected", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/debug/replay/"+remoteActivityID, headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = remoteActivityID

		requireStatus(t, http.StatusBadRequest)(h.HandleDebugReplayLift(ctx))
	})

	t.Run("replay local deliverable activity", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/debug/replay/"+localActivityID, headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["activity_id"] = localActivityID

		requireStatus(t, http.StatusOK)(h.HandleDebugReplayLift(ctx))
	})

	t.Run("object debug", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/object/obj-1", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "obj-1"

		requireStatus(t, http.StatusOK)(h.HandleDebugObjectLift(ctx))
	})

	t.Run("object explain debug", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/object/obj-1/explain", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "obj-1"

		requireStatus(t, http.StatusOK)(h.HandleDebugObjectExplainLift(ctx))
	})

	t.Run("federation domain debug", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/debug/federation/domain/example.net", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["domain"] = "example.net"

		requireStatus(t, http.StatusOK)(h.HandleDebugFederationDomainLift(ctx))
	})
}

func TestDebugObjectNotFoundRound12(t *testing.T) {
	cfg := round11TestConfig()
	cfg.Stage = "test"

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"admin"})
	headers := map[string]string{"Authorization": "Bearer " + adminToken}

	state := &round10QueryState{
		notFoundPKSK: map[string]bool{
			"object#missing#object#missing": true,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/debug/object/missing", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["object_id"] = "missing"

	requireStatus(t, http.StatusNotFound)(h.HandleDebugObjectLift(ctx))
}
