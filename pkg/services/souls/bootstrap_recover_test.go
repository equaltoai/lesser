package souls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_RecoverHostedGenesisTurnCallsHostRecoverWithoutUserMessage(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_recover_001"
		conversationID = "conv_recover_001"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var (
		sawMethod     string
		sawPath       string
		sawAuth       string
		sawBody       map[string]any
		sawContentType string
	)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		sawPath = r.URL.Path
		sawAuth = r.Header.Get("Authorization")
		sawContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sawBody))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-recover-001")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration_id": registrationID,
			"agent_id":        agentID,
			"conversation_id": conversationID,
			"status":          "assistant_turn_ready",
			"latest_turn_id":  "turn_assistant_recovered",
			"message_count":   2,
			"request_id":      "host-req-recover-001",
		}))
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: instanceKey},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	result, err := service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
		CorrelationID:  "corr-001",
		IdempotencyKey: "idem-001",
	})
	require.NoError(t, err)

	// Verify the HTTP call hit the /recover endpoint with POST and instance-key auth.
	require.Equal(t, http.MethodPost, sawMethod)
	require.Equal(t, "/api/v1/soul/instance/agents/register/"+registrationID+"/mint-conversation/"+conversationID+"/recover", sawPath)
	require.Equal(t, "Bearer "+instanceKey, sawAuth)
	require.Equal(t, "application/json", sawContentType)

	// Verify the request body includes correlation_id and idempotency_key but NO message.
	require.Equal(t, "corr-001", sawBody["correlation_id"])
	require.Equal(t, "idem-001", sawBody["idempotency_key"])
	_, hasMessage := sawBody["message"]
	require.False(t, hasMessage, "recovery request must not include a message field")
	_, hasModel := sawBody["model"]
	require.False(t, hasModel, "recovery request must not include a model field")

	// Verify the response is mapped correctly.
	require.Equal(t, registrationID, result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, conversationID, result.ConversationID)
	require.Equal(t, "assistant_turn_ready", result.Status)
	require.Equal(t, "turn_assistant_recovered", result.LatestTurnID)
	require.Equal(t, 2, result.MessageCount)
	require.Equal(t, "host-req-recover-001", result.HostRequestID)
}

func TestService_RecoverHostedGenesisTurnIsIdempotentForNonStuckSession(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_recover_002"
		conversationID = "conv_recover_002"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var callCount int
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/instance/agents/register/"+registrationID+"/mint-conversation/"+conversationID+"/recover", r.URL.Path)
		callCount++

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-recover-idem")
		w.WriteHeader(http.StatusOK)
		// Host returns current state (declaration_ready) — the session was not stuck.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"registration_id": registrationID,
			"agent_id":        agentID,
			"conversation_id": conversationID,
			"status":          "declaration_ready",
			"latest_turn_id":  "turn_final",
			"message_count":   4,
			"request_id":      "host-req-recover-idem",
			"produced_declarations": map[string]any{
				"declaration_id":   "decl_001",
				"declaration_hash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"produced_at":      "2026-07-02T12:00:00Z",
				"declarations": map[string]any{
					"selfDescription": map[string]any{"summary": "recovered soul"},
					"capabilities":    []any{},
					"boundaries":      []any{},
					"transparency":    map[string]any{},
				},
				"evidence": map[string]any{
					"source":          "host_conversation",
					"registration_id": registrationID,
					"conversation_id": conversationID,
					"agent_id":        agentID,
					"message_count":   4,
					"request_id":      "host-req-recover-idem",
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

	result, err := service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, callCount)
	require.Equal(t, "declaration_ready", result.Status)
	require.Equal(t, 4, result.MessageCount)
	require.Equal(t, "host-req-recover-idem", result.HostRequestID)
	// The produced declarations should be compacted but present.
	require.NotEmpty(t, result.ProducedDeclarations)
	require.NotContains(t, result.ProducedDeclarations, "declaration_id")
}

func TestService_RecoverHostedGenesisTurnAcceptsHostWrapperEnvelope(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_recover_003"
		conversationID = "conv_recover_003"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "host-req-header-ignored")
		w.WriteHeader(http.StatusAccepted)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "1",
			"request_id": "host-req-recover-wrapper",
			"conversation": map[string]any{
				"registration_id": registrationID,
				"conversation_id": conversationID,
				"agent_id":        agentID,
				"status":          "in_progress",
				"latest_turn_id":  "turn_recovered_001",
				"message_count":   3,
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

	result, err := service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	require.NoError(t, err)
	require.Equal(t, registrationID, result.RegistrationID)
	require.Equal(t, agentID, result.HostSoulAgentID)
	require.Equal(t, conversationID, result.ConversationID)
	require.Equal(t, "in_progress", result.Status)
	require.Equal(t, 3, result.MessageCount)
	require.Equal(t, "host-req-recover-wrapper", result.HostRequestID)
}

func TestService_RecoverHostedGenesisTurnRejectsUnsupportedHostWrapperVersion(t *testing.T) {
	t.Parallel()

	const (
		instanceKey    = "host-instance-key"
		registrationID = "reg_recover_004"
		conversationID = "conv_recover_004"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version":    "2",
			"request_id": "host-req-recover-v2",
			"conversation": map[string]any{
				"registration_id": registrationID,
				"conversation_id": conversationID,
				"agent_id":        agentID,
				"status":          "in_progress",
				"message_count":   1,
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

	_, err := service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		RegistrationID: registrationID,
		ConversationID: conversationID,
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_RESPONSE_INVALID", hostErr.Code)
	require.Contains(t, hostErr.Message, "unsupported version")
}

func TestService_RecoverHostedGenesisTurnRequiresRegistrationAndConversationIDs(t *testing.T) {
	t.Parallel()

	service := NewService(
		&fakeAccountRepo{},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://host.example"}},
		&config.Config{Domain: "example.com", LesserHostInstanceKey: "key"},
		zap.NewNop(),
	)

	_, err := service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		ConversationID: "conv_001",
	})
	var hostErr *HostBootstrapError
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_REGISTRATION_ID_REQUIRED", hostErr.Code)

	_, err = service.RecoverHostedGenesisTurn(context.Background(), BootstrapConversationRecoverInput{
		RegistrationID: "reg_001",
	})
	require.ErrorAs(t, err, &hostErr)
	require.Equal(t, "HOST_CONVERSATION_ID_REQUIRED", hostErr.Code)
}
