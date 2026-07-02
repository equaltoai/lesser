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

func TestService_ListHostedGenesisConversationsHostHTTPErrorReturnsTypedError(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-list-forbidden")
		w.WriteHeader(http.StatusForbidden)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":        "soul_instance.boundary_violation",
				"message":     "instance boundary violation",
				"status_code": 403,
				"request_id":  "host-req-list-forbidden",
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

	_, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "soul_instance.boundary_violation", hostErr.Code)
	require.Equal(t, http.StatusForbidden, hostErr.StatusCode)
}

func TestService_ListHostedGenesisConversationsInvalidJSONReturnsTypedError(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-list-bad-json")
		w.WriteHeader(http.StatusOK)
		// Valid JSON but conversations is a string, not an array —
		// json.Unmarshal into hostMintConversationsListResponse will fail.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": "not-an-array",
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.ListHostedGenesisConversations(context.Background(), agentID)
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
}

func TestService_ListHostedGenesisConversationsMisconfiguredServiceReturnsError(t *testing.T) {
	t.Parallel()

	service := &Service{}
	_, err := service.ListHostedGenesisConversations(context.Background(), "0xabc")
	require.Error(t, err)
}

func TestService_ListHostedGenesisConversationsSortsMultipleNilUpdatedAtEntries(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Two entries with nil UpdatedAt — exercises the both-nil tie-break branch.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": []map[string]any{
				{
					"conversation_id": "conv_no_updated_b",
					"status":          "in_progress",
					"message_count":   1,
				},
				{
					"conversation_id": "conv_no_updated_a",
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
	require.Len(t, result, 2)
	// Both have nil UpdatedAt — tie-break by conversation_id ascending.
	require.Equal(t, "conv_no_updated_a", result[0].ConversationID)
	require.Equal(t, "conv_no_updated_b", result[1].ConversationID)
	require.Nil(t, result[0].UpdatedAt)
	require.Nil(t, result[1].UpdatedAt)
}

func TestService_ListHostedGenesisConversationsSortsNonNilBeforeNilUpdatedAt(t *testing.T) {
	t.Parallel()

	const (
		instanceKey = "host-instance-key"
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Three entries: first nil, then two non-nil — Go's insertion sort
		// calls less(j, j-1) which triggers the summaries[j].UpdatedAt == nil
		// branch when comparing a non-nil entry against a preceding nil entry.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"conversations": []map[string]any{
				{
					"conversation_id": "conv_no_time_first",
					"status":          "in_progress",
					"message_count":   1,
				},
				{
					"conversation_id": "conv_with_time_a",
					"status":          "in_progress",
					"message_count":   2,
					"updated_at":      "2026-07-01T10:00:00Z",
				},
				{
					"conversation_id": "conv_with_time_b",
					"status":          "declaration_ready",
					"message_count":   3,
					"updated_at":      "2026-07-01T12:00:00Z",
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
	require.Len(t, result, 3)
	// Non-nil UpdatedAt entries sort before nil; among non-nil, newer first.
	require.Equal(t, "conv_with_time_b", result[0].ConversationID)
	require.Equal(t, "conv_with_time_a", result[1].ConversationID)
	require.Equal(t, "conv_no_time_first", result[2].ConversationID)
	require.NotNil(t, result[0].UpdatedAt)
	require.NotNil(t, result[1].UpdatedAt)
	require.Nil(t, result[2].UpdatedAt)
}
