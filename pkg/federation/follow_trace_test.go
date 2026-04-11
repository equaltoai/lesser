package federation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestNewFollowTraceMetadata_Gating(t *testing.T) {
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://sim.dev.example.com/activities/follow-1",
			Type: activitypub.FollowType,
		},
		Actor:  "https://sim.dev.example.com/users/ops",
		Object: "https://theory.dev.example.com/users/steward",
	}

	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv(FollowTraceEnvVar, "")
		assert.Nil(t, NewFollowTraceMetadata(activity, activity.Actor, activity.Object.(string)))
	})

	t.Run("matches ops to steward across url and inbox forms", func(t *testing.T) {
		t.Setenv(FollowTraceEnvVar, "true")

		trace := NewFollowTraceMetadata(activity, "@ops@sim.dev.example.com", "https://theory.dev.example.com/users/steward/inbox")
		require.NotNil(t, trace)
		assert.Equal(t, activity.ID, trace.ActivityID)
		assert.Equal(t, activitypub.FollowType, trace.ActivityType)
		assert.Equal(t, "ops", trace.SenderUsername)
		assert.Equal(t, "steward", trace.TargetUsername)
	})

	t.Run("rejects non matching pair", func(t *testing.T) {
		t.Setenv(FollowTraceEnvVar, "1")
		assert.Nil(t, NewFollowTraceMetadata(activity, activity.Actor, "https://theory.dev.example.com/users/alice"))
	})
}

func TestWithFollowTrace_ContextRoundTrip(t *testing.T) {
	t.Setenv(FollowTraceEnvVar, "true")

	trace := NewFollowTraceMetadata(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://sim.dev.example.com/activities/follow-2",
			Type: activitypub.FollowType,
		},
		Actor:  "https://sim.dev.example.com/users/ops",
		Object: "https://theory.dev.example.com/users/steward",
	}, "ops", "steward")
	require.NotNil(t, trace)

	ctx := WithFollowTrace(context.Background(), trace)
	got, ok := FollowTraceFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, trace, got)
}

func TestBuildSignatureTraceFields_ReportsCanonicalString(t *testing.T) {
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	req, err := BuildSignedActivityPubRequest(
		context.Background(),
		http.MethodPost,
		"https://theory.dev.example.com/users/steward/inbox?shared=true",
		"Lesser/1.0",
		[]byte(`{"type":"Follow"}`),
		privateKey,
		"https://sim.dev.example.com/users/ops#main-key",
	)
	require.NoError(t, err)

	fields := encodeTraceFields(t, BuildSignatureTraceFields(req))
	assert.Equal(t, http.MethodPost, fields["request_method"])
	assert.Equal(t, "https://theory.dev.example.com/users/steward/inbox?shared=true", fields["request_url"])
	assert.True(t, fields["request_host_present"].(bool))
	assert.Equal(t, "theory.dev.example.com", fields["request_url_host"])
	assert.True(t, fields["request_digest_header_present"].(bool))
	assert.True(t, fields["request_date_header_present"].(bool))
	assert.True(t, fields["request_signature_header_present"].(bool))
	assert.True(t, fields["signature_parsed"].(bool))
	assert.True(t, fields["signature_key_id_present"].(bool))
	assert.True(t, fields["signature_algorithm_present"].(bool))
	assert.Equal(t, int64(5), fields["signature_header_count"])
	assert.Greater(t, int(fields["signature_canonical_len"].(int64)), 0)
	assert.NotContains(t, fields, "request_signature_header")
	assert.NotContains(t, fields, "request_digest_header")
	assert.NotContains(t, fields, "signature_key_id")
	assert.NotContains(t, fields, "signature_algorithm")
	assert.NotContains(t, fields, "signature_canonical_string")
}

func TestFollowTraceHelpers_AdditionalCoverage(t *testing.T) {
	t.Setenv(FollowTraceEnvVar, "true")

	trace := &FollowTraceMetadata{
		ActivityID:     "activity-1",
		ActivityType:   activitypub.FollowType,
		SenderUsername: "ops",
		TargetUsername: "steward",
	}

	fields := encodeTraceFields(t, FollowTraceFields(trace, "receiver.raw_request"))
	assert.Equal(t, "receiver.raw_request", fields["trace_stage"])
	assert.Equal(t, "ops", fields["trace_sender_username"])
	assert.Equal(t, "steward", fields["trace_target_username"])
	assert.Nil(t, FollowTraceFields(nil, "unused"))

	nilFields := encodeTraceFields(t, BuildSignatureTraceFields(nil))
	assert.Equal(t, "nil_request", nilFields["trace_request_error"])

	req := httptest.NewRequest(http.MethodPost, "https://theory.dev.example.com/users/steward/inbox", nil)
	req.Host = "theory.dev.example.com"
	req.Header.Set(SignatureHeader, `not-valid`)
	parseFields := encodeTraceFields(t, BuildSignatureTraceFields(req))
	assert.True(t, parseFields["request_signature_header_present"].(bool))
	assert.True(t, parseFields["signature_parse_failed"].(bool))

	assert.Equal(t, "ops", normalizeFollowTraceIdentity("@Ops@sim.dev.example.com"))
	assert.Equal(t, "steward", normalizeFollowTraceIdentity("https://theory.dev.example.com/users/steward/inbox"))
	assert.Equal(t, "theory.dev.example.com", normalizeFollowTraceIdentity("https://theory.dev.example.com"))
	assert.Equal(t, "remote@example.com", normalizeFollowTraceIdentity("remote@example.com"))

	assert.Equal(t, "steward", usernameFromFollowTracePath("/users/steward/inbox"))
	assert.Equal(t, "ops", usernameFromFollowTracePath("/actors/ops/outbox"))
	assert.Equal(t, "", usernameFromFollowTracePath("inbox"))
}

func TestBuildSignedActivityPubRequest_TraceContextCoverage(t *testing.T) {
	t.Setenv(FollowTraceEnvVar, "true")

	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)

	trace := NewFollowTraceMetadata(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://sim.dev.example.com/activities/follow-3",
			Type: activitypub.FollowType,
		},
		Actor:  "https://sim.dev.example.com/users/ops",
		Object: "https://theory.dev.example.com/users/steward",
	}, "ops", "steward")
	require.NotNil(t, trace)

	req, err := BuildSignedActivityPubRequest(
		WithFollowTrace(context.Background(), trace),
		http.MethodPost,
		"https://theory.dev.example.com/users/steward/inbox",
		"Lesser/1.0",
		[]byte(`{"type":"Follow"}`),
		privateKey,
		"https://sim.dev.example.com/users/ops#main-key",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, req.Header.Get(SignatureHeader))
}

func TestSignatureService_VerifySignature_TraceContextCoverage(t *testing.T) {
	t.Setenv(FollowTraceEnvVar, "true")

	logger := zaptest.NewLogger(t)
	privateKey, err := GenerateRSAKeyPair(2048)
	require.NoError(t, err)
	publicKeyPEM, err := EncodePublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)

	trace := NewFollowTraceMetadata(&activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			ID:   "https://sim.dev.example.com/activities/follow-4",
			Type: activitypub.FollowType,
		},
		Actor:  "https://sim.dev.example.com/users/ops",
		Object: "https://theory.dev.example.com/users/steward",
	}, "ops", "steward")
	require.NotNil(t, trace)

	ctx := WithFollowTrace(context.Background(), trace)
	req, err := BuildSignedActivityPubRequest(
		ctx,
		http.MethodPost,
		"https://theory.dev.example.com/users/steward/inbox",
		"Lesser/1.0",
		[]byte(`{"type":"Follow"}`),
		privateKey,
		"https://sim.dev.example.com/users/ops#main-key",
	)
	require.NoError(t, err)
	req = req.WithContext(ctx)

	repo := &fakePublicKeyCacheRepo{
		cache: &models.PublicKeyCache{
			ActorURL:     "https://sim.dev.example.com/users/ops",
			KeyID:        "https://sim.dev.example.com/users/ops#main-key",
			PublicKeyPEM: string(publicKeyPEM),
			Algorithm:    AlgorithmHS2019,
			TTL:          time.Now().Add(time.Hour).Unix(),
		},
	}

	svc := &SignatureService{
		publicKeyCacheRepo: repo,
		httpClient:         &fakeSignatureHTTPClient{do: func(_ *http.Request) (*http.Response, error) { return nil, assert.AnError }},
		logger:             logger,
		sleep:              func(context.Context, time.Duration) error { return nil },
	}

	require.NoError(t, svc.VerifySignature(ctx, req, "https://sim.dev.example.com/users/ops"))
	require.Len(t, repo.updateCalls, 1)
	assert.True(t, repo.updateCalls[0])
}

func encodeTraceFields(t *testing.T, fields []zapcore.Field) map[string]any {
	t.Helper()

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	return encoder.Fields
}
