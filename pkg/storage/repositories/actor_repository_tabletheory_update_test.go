package repositories

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"go.uber.org/zap/zaptest"
)

func TestActorRepositoryUpdateActor_TableTheoryUpdateAcceptsActivityPubJSONLDActor(t *testing.T) {
	ctx := context.Background()
	actorID := "https://example.com/users/della"

	server := newActorUpdateDynamoServer(t, actorID)
	defer server.Close()

	db, err := tabletheory.New(session.Config{
		Region:              "us-east-1",
		Endpoint:            server.URL,
		CredentialsProvider: credentials.NewStaticCredentialsProvider("test", "test", ""),
	})
	require.NoError(t, err)

	repo := NewActorRepository(db, "test-table", zaptest.NewLogger(t), "example.com")
	err = repo.UpdateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      actorID,
			Type:    activitypub.PersonType,
		},
		PreferredUsername: "della",
		Name:              "Della Marlowe",
		Summary:           "same bio",
		URL:               "https://example.com/@della",
		Inbox:             actorID + "/inbox",
		Outbox:            actorID + "/outbox",
		Followers:         actorID + "/followers",
		Following:         actorID + "/following",
		Liked:             actorID + "/liked",
		Endpoints:         &activitypub.Endpoints{SharedInbox: "https://example.com/inbox"},
	})
	require.NoError(t, err)

	updatePayload := server.updatePayload()
	require.NotEmpty(t, updatePayload, "expected UpdateItem to be executed")
	require.NotContains(t, updatePayload, "security validation failed")

	var update map[string]any
	require.NoError(t, json.Unmarshal([]byte(updatePayload), &update))
	values, ok := update["ExpressionAttributeValues"].(map[string]any)
	require.True(t, ok)

	var actorAttr map[string]any
	for _, raw := range values {
		attr, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if m, ok := attr["M"].(map[string]any); ok {
			if _, hasContext := m["@context"]; hasContext {
				actorAttr = m
				break
			}
		}
	}
	require.NotNil(t, actorAttr, "updated actor should remain a native DynamoDB map with JSON-LD @context")
	require.Contains(t, actorAttr, "@context")
	require.Contains(t, actorAttr, "preferredUsername")
}

type actorUpdateDynamoServer struct {
	*httptest.Server

	t                     *testing.T
	mu                    sync.Mutex
	capturedUpdatePayload string
	actorID               string
}

func newActorUpdateDynamoServer(t *testing.T, actorID string) *actorUpdateDynamoServer {
	t.Helper()

	state := &actorUpdateDynamoServer{
		t:       t,
		actorID: actorID,
	}
	state.Server = httptest.NewServer(http.HandlerFunc(state.handle))
	return state
}

func (s *actorUpdateDynamoServer) updatePayload() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.capturedUpdatePayload
}

func (s *actorUpdateDynamoServer) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	require.NoError(s.t, err)

	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	target := r.Header.Get("X-Amz-Target")
	switch {
	case strings.Contains(target, "Query"):
		_, _ = w.Write([]byte(`{"Items":[` + actorUpdateDynamoItemJSON(s.actorID) + `],"Count":1,"ScannedCount":1}`))
	case strings.Contains(target, "GetItem"):
		_, _ = w.Write([]byte(`{"Item":` + actorUpdateDynamoItemJSON(s.actorID) + `}`))
	case strings.Contains(target, "UpdateItem"):
		s.mu.Lock()
		s.capturedUpdatePayload = string(body)
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	default:
		s.t.Fatalf("unexpected DynamoDB target %q body=%s", target, string(body))
	}
}

func actorUpdateDynamoItemJSON(actorID string) string {
	return `{
		"PK":{"S":"ACTOR#della"},
		"SK":{"S":"PROFILE"},
		"username":{"S":"della"},
		"numericID":{"S":"123456"},
		"version":{"N":"7"},
		"followerCount":{"N":"0"},
		"followingCount":{"N":"0"},
		"statusCount":{"N":"2"},
		"actor":{"M":{
			"id":{"S":"` + actorID + `"},
			"type":{"S":"Person"},
			"preferredUsername":{"S":"della"},
			"name":{"S":"Della"},
			"url":{"S":"https://example.com/@della"},
			"inbox":{"S":"` + actorID + `/inbox"},
			"outbox":{"S":"` + actorID + `/outbox"},
			"followers":{"S":"` + actorID + `/followers"},
			"following":{"S":"` + actorID + `/following"},
			"liked":{"S":"` + actorID + `/liked"}
		}}
	}`
}
