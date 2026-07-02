package souls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_ListHostedGenesisConversationsCallsHostListEndpoint(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var (
		sawMethod string
		sawPath   string
		sawAuth   string
	)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		sawPath = r.URL.Path
		sawAuth = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-list-001")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": []map[string]any{
				{
					"conversation_id": "conv_001",
					"registration_id": "reg_001",
					"status":          "in_progress",
					"message_count":   2,
					"latest_turn_id":  "turn_001",
					"created_at":      "2026-07-01T10:00:00Z",
					"updated_at":      "2026-07-01T11:00:00Z",
				},
				{
					"conversation_id": "conv_002",
					"registration_id": "reg_001",
					"status":          "declaration_ready",
					"message_count":   4,
					"latest_turn_id":  "turn_004",
					"created_at":      "2026-07-01T12:00:00Z",
					"updated_at":      "2026-07-01T13:00:00Z",
				},
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	require.NoError(t, err)

	require.Equal(t, http.MethodGet, sawMethod)
	require.Equal(t, "/api/v1/soul/instance/agents/"+agentID+"/mint-conversations", sawPath)
	require.Equal(t, "Bearer "+instanceKey, sawAuth)

	require.Len(t, result, 2)
	// Sorted by updated_at descending — conv_002 has the later updated_at.
	require.Equal(t, "conv_002", result[0].ConversationID)
	require.Equal(t, "declaration_ready", result[0].Status)
	require.Equal(t, 4, result[0].MessageCount)
	require.Equal(t, "turn_004", result[0].LatestTurnID)
	require.NotNil(t, result[0].CreatedAt)
	require.NotNil(t, result[0].UpdatedAt)

	require.Equal(t, "conv_001", result[1].ConversationID)
	require.Equal(t, "in_progress", result[1].Status)
	require.Equal(t, 2, result[1].MessageCount)
}

func TestService_ListHostedGenesisConversationsSortedByUpdatedAtDescending(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": []map[string]any{
				{
					"conversation_id": "conv_old",
					"status":          "in_progress",
					"message_count":   1,
					"updated_at":      "2026-06-01T10:00:00Z",
				},
				{
					"conversation_id": "conv_newest",
					"status":          "declaration_ready",
					"message_count":   3,
					"updated_at":      "2026-07-02T14:00:00Z",
				},
				{
					"conversation_id": "conv_mid",
					"status":          "assistant_turn_ready",
					"message_count":   2,
					"updated_at":      "2026-06-15T12:00:00Z",
				},
				{
					"conversation_id": "conv_no_updated",
					"status":          "in_progress",
					"message_count":   1,
				},
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	require.NoError(t, err)
	require.Len(t, result, 4)

	// Sorted by updated_at descending; nil updated_at entries go last.
	require.Equal(t, "conv_newest", result[0].ConversationID)
	require.Equal(t, "conv_mid", result[1].ConversationID)
	require.Equal(t, "conv_old", result[2].ConversationID)
	require.Equal(t, "conv_no_updated", result[3].ConversationID)
	require.Nil(t, result[3].UpdatedAt)
}

func TestService_ListHostedGenesisConversationsBoundedTo50(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		conversations := make([]map[string]any, 0, 60)
		for i := 0; i < 60; i++ {
			conversations = append(conversations, map[string]any{
				"conversation_id": "conv_" + string(rune('a'+i)),
				"status":          "in_progress",
				"message_count":   i + 1,
				"updated_at":      time.Date(2026, 7, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339),
			})
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": conversations,
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	require.NoError(t, err)
	require.Len(t, result, 50, "list must be bounded to 50 results")
	// The most recent 50 should be kept (sorted by updated_at descending).
	require.Equal(t, 60, result[0].MessageCount)
	require.Equal(t, 11, result[49].MessageCount)
}

func TestService_ListHostedGenesisConversationsRequiresAgentID(t *testing.T) {
	t.Parallel()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "key"},
		zap.NewNop(),
	)

	_, err := service.ListHostedGenesisConversations(context.Background(), "")
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_AGENT_ID_REQUIRED", hostErr.Code)
}

func TestService_ListHostedGenesisConversationsHandlesEmptyResponse(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": []any{},
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	require.NoError(t, err)
	require.Empty(t, result)
}
