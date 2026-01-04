package routing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestInboxMain_Round10_BuildAppAndLambdaHandler(t *testing.T) {
	env := newInboxTestEnv(t)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "alice",
		"scopes":   []string{"read"},
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(env.cfg.JWTSecret))
	require.NoError(t, err)

	makeAPIEvent := func(method, path string, headers map[string]string) map[string]any {
		eventHeaders := make(map[string]any, len(headers))
		for k, v := range headers {
			eventHeaders[k] = v
		}
		return map[string]any{
			"version":  "2.0",
			"routeKey": "$default",
			"headers":  eventHeaders,
			"requestContext": map[string]any{
				"stage": "$default",
				"http": map[string]any{
					"method": method,
					"path":   path,
				},
			},
		}
	}

	// With metrics enabled, exercise both error and success outcomes.
	app := buildInboxApp(&common.LambdaContext{Logger: env.logger}, env.handler)
	handler := buildInboxLambdaHandler(app, env.handler)

	_, _ = handler(context.Background(), map[string]any{}) // adapter error path

	okHeaders := map[string]string{
		"Host":          "localhost",
		"Authorization": "Bearer " + signed,
	}
	_, _ = handler(context.Background(), makeAPIEvent("GET", "/users/alice/inbox", okHeaders))

	// With metrics disabled and cold start window exceeded, exercise the nil-metrics branches.
	noMetrics := *env.handler
	noMetrics.emfMetrics = nil
	noMetrics.startTime = time.Now().Add(-time.Minute)

	appNoMetrics := buildInboxApp(&common.LambdaContext{Logger: env.logger}, &noMetrics)
	handlerNoMetrics := buildInboxLambdaHandler(appNoMetrics, &noMetrics)
	_, _ = handlerNoMetrics(context.Background(), makeAPIEvent("GET", "/users/alice/inbox", map[string]string{"Host": "localhost"}))

	// Exercise the panic recovery path in the middleware chain.
	_ = app.GET("/panic", func(_ *lift.Context) error { panic("boom") })
	_, _ = handler(context.Background(), makeAPIEvent("GET", "/panic", map[string]string{"Host": "localhost"}))
}

func TestInboxHandler_Round10_GetAndValidationPaths(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("handleGetInbox validation errors", func(t *testing.T) {
		ctx := newLiftContext("GET", "/users//inbox", map[string]string{"Host": "localhost"}, nil, nil)
		require.Error(t, env.handler.handleGetInbox(ctx))

		ctx = newLiftContext("GET", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, nil)
		ctx.SetParam("username", "alice")
		require.Error(t, env.handler.handleGetInbox(ctx)) // no auth header
	})

	t.Run("handleGetInbox collection and page", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"username": "alice",
			"scopes":   []string{"read"},
			"iat":      time.Now().Unix(),
			"exp":      time.Now().Add(time.Hour).Unix(),
		})
		signed, err := token.SignedString([]byte(env.cfg.JWTSecret))
		require.NoError(t, err)

		headers := map[string]string{
			"Host":          "localhost",
			"Authorization": "Bearer " + signed,
		}

		collectionCtx := newLiftContext("GET", "/users/alice/inbox", headers, map[string]string{}, nil)
		collectionCtx.SetParam("username", "alice")
		require.NoError(t, env.handler.handleGetInbox(collectionCtx))
		require.Equal(t, 200, collectionCtx.Response.StatusCode)

		pageCtx := newLiftContext("GET", "/users/alice/inbox", headers, map[string]string{"page": "true"}, nil)
		pageCtx.SetParam("username", "alice")
		require.NoError(t, env.handler.handleGetInbox(pageCtx))
		require.Equal(t, 200, pageCtx.Response.StatusCode)
	})

	t.Run("request body and activity parsing", func(t *testing.T) {
		require.Error(t, env.handler.validateRequestBody(nil))
		require.Error(t, env.handler.validateRequestBody(make([]byte, common.MaxActivitySize+1)))

		_, err := env.handler.parseActivity([]byte("{not-json"))
		require.Error(t, err)

		// Valid JSON but invalid URL for id.
		raw := map[string]any{
			"@context": activitypub.Context,
			"type":     "Create",
			"id":       "not-a-url",
			"actor":    "https://remote.example/users/bob",
			"to":       []string{env.local.ID},
			"object":   map[string]any{"type": "Note", "content": "hi"},
		}
		b, jerr := json.Marshal(raw)
		require.NoError(t, jerr)
		_, err = env.handler.parseActivity(b)
		require.Error(t, err)
	})

	t.Run("verifyAuthentication conversion failure", func(t *testing.T) {
		ctx := newLiftContext("BAD METHOD", "/users/alice/inbox", map[string]string{"Host": "localhost"}, nil, []byte("x"))
		ctx.SetParam("username", "alice")
		req := &InboxRequest{
			Username: "alice",
			Activity: &activitypub.Activity{Actor: "https://remote.example/users/bob"},
			Body:     []byte("x"),
			StartTime: time.Now(),
			CostParams: &federation.CostCalculationParams{
				ActivityID:    "a",
				Domain:        "remote.example",
				ActivityType:  "Create",
				Direction:     "inbound",
				OperationType: "inbox_processing",
				Timestamp:     time.Now(),
			},
		}
		require.Error(t, env.handler.verifyAuthentication(ctx, req))
	})

	t.Run("convertLiftRequest and helpers", func(t *testing.T) {
		ctx := newLiftContext("POST", "/users/alice/inbox", map[string]string{"Host": "localhost"}, map[string]string{"a": "b"}, []byte("x"))
		httpReq, err := env.handler.convertLiftRequest(ctx, []byte("x"))
		require.NoError(t, err)
		require.Equal(t, "localhost", httpReq.URL.Host)

		require.NotEmpty(t, generateActivityID())
		require.Equal(t, "@alice@example.com", env.handler.extractHandleFromActorID("https://example.com/users/alice"))
		require.Equal(t, "alice", env.handler.extractUsernameFromActorID("https://example.com/users/alice"))
		require.Equal(t, "example.com", env.handler.extractDomainFromURL("https://example.com/users/alice"))

		require.Equal(t, 500, env.handler.determineStatusCode(assertErr{}))
		require.Equal(t, 200, env.handler.determineStatusCode(nil))
		require.True(t, env.handler.isRequestSuccessful(nil, 204))
		require.False(t, env.handler.isRequestSuccessful(assertErr{}, 200))
		require.NotEmpty(t, env.handler.parseRemoteInstance("Mastodon/4.0.0"))
	})
}

func TestInboxHandler_Round10_FlagAndMoveProcessing(t *testing.T) {
	env := newInboxTestEnv(t)

	t.Run("flag activity object extraction branches", func(t *testing.T) {
		flagged, err := env.handler.extractFlaggedObjects(&activitypub.Activity{Object: "https://example.com/objects/1"})
		require.NoError(t, err)
		require.Len(t, flagged, 1)

		flagged, err = env.handler.extractFlaggedObjects(&activitypub.Activity{Object: []any{"https://example.com/objects/1", map[string]any{"id": "https://example.com/objects/2"}}})
		require.NoError(t, err)
		require.Len(t, flagged, 2)

		_, err = env.handler.extractFlaggedObjects(&activitypub.Activity{Object: 123})
		require.Error(t, err)
	})

	t.Run("processFlagActivity stores and optionally triggers automated moderation", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:      env.cfg.BaseURL() + "/activities/flag-1",
				Type:    activitypub.FlagType,
				Summary: "this looks like spam",
			},
			Actor:  "https://remote.example/users/reporter",
			Object: "https://example.com/objects/1",
		}
		require.NoError(t, env.handler.processFlagActivity(context.Background(), activity, env.local))
	})

	t.Run("processMoveActivity branches", func(t *testing.T) {
		noTarget := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   env.cfg.BaseURL() + "/activities/move-1",
				Type: activitypub.MoveType,
			},
			Actor: "https://remote.example/users/old",
		}
		require.Error(t, env.handler.processMoveActivity(context.Background(), noTarget, env.local))

		// Configure the returned "new account" actor to include AlsoKnownAs and allow authorization.
		env.local.AlsoKnownAs = []string{"https://remote.example/users/old"}

		ok := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:     env.cfg.BaseURL() + "/activities/move-2",
				Type:   activitypub.MoveType,
			},
			Actor: "https://remote.example/users/old",
			Target: env.cfg.ActorURL("alice"),
		}
		require.NoError(t, env.handler.processMoveActivity(context.Background(), ok, env.local))
	})
}

type assertErr struct{}

func (assertErr) Error() string { return "err" }
