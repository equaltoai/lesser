package routing

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInboxHandler_FollowTraceHelpersCoverage(t *testing.T) {
	t.Setenv(federation.FollowTraceEnvVar, "true")

	env := newInboxTestEnv(t)
	assert.Nil(t, env.handler.followTraceForRequest(nil))

	req := &InboxRequest{
		Username: "steward",
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://sim.dev.example.com/activities/follow-trace",
				Type: activitypub.FollowType,
			},
			Actor:  "https://sim.dev.example.com/users/ops",
			Object: "https://theory.dev.example.com/users/steward",
		},
	}

	trace := env.handler.followTraceForRequest(req)
	require.NotNil(t, trace)

	ctx := newAppTheoryContextWithValues(
		http.MethodPost,
		"/users/steward/inbox",
		map[string][]string{
			"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
			"x-lesser-forwarded-host":  {"theory.dev.example.com"},
			"x-lesser-forwarded-proto": {"https"},
			"content-type":             {"application/activity+json"},
			"date":                     {"Thu, 09 Apr 2026 13:00:00 UTC"},
			"signature":                {`keyId="https://sim.dev.example.com/users/ops#main-key",algorithm="hs2019",headers="(request-target) host date",signature="dGVzdA=="`},
		},
		map[string][]string{
			"shared": {"true"},
		},
		[]byte(`{"type":"Follow"}`),
	)
	ctx.RequestID = "req-trace-1"

	env.handler.logFollowTrace(nil, "receiver.noop")
	env.handler.logFollowTrace(trace, "receiver.helper")
	env.handler.logFollowTraceRawRequest(ctx, trace, req.Username)

	reconstructed, err := env.handler.convertRequest(ctx, ctx.Request.Body)
	require.NoError(t, err)

	env.handler.logFollowTraceReconstructedRequest(nil, reconstructed)
	env.handler.logFollowTraceReconstructedRequest(trace, reconstructed)
}

func TestInboxHandler_VerifyDigestEnhanced_TraceCoverage(t *testing.T) {
	t.Setenv(federation.FollowTraceEnvVar, "true")

	env := newInboxTestEnv(t)
	body := []byte(`{"type":"Follow"}`)
	sum := sha256.Sum256(body)

	req := &InboxRequest{
		Username: "steward",
		Activity: &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				ID:   "https://sim.dev.example.com/activities/follow-trace-digest",
				Type: activitypub.FollowType,
			},
			Actor:  "https://sim.dev.example.com/users/ops",
			Object: "https://theory.dev.example.com/users/steward",
		},
		Body: body,
	}

	missingDigestCtx := newAppTheoryContextWithValues(
		http.MethodPost,
		"/users/steward/inbox",
		map[string][]string{
			"host":                     {"theory.dev.example.com"},
			"x-lesser-forwarded-proto": {"https"},
			"content-type":             {"application/activity+json"},
		},
		nil,
		body,
	)
	require.NoError(t, env.handler.verifyDigestEnhanced(missingDigestCtx, req))

	validDigestCtx := newAppTheoryContextWithValues(
		http.MethodPost,
		"/users/steward/inbox",
		map[string][]string{
			"host":                     {"theory.dev.example.com"},
			"x-lesser-forwarded-proto": {"https"},
			"content-type":             {"application/activity+json"},
			"digest":                   {"SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])},
		},
		nil,
		body,
	)
	require.NoError(t, env.handler.verifyDigestEnhanced(validDigestCtx, req))
}
