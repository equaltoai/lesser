package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// agentAvatarTestStorage swaps the user and status repositories for mocks so a
// test can observe which reads the agent avatar path performs. Everything else
// (notably the account repository that serves agent governance state) is the
// round12 harness's real repository.
type agentAvatarTestStorage struct {
	*round12GraphStorage
	userRepo   interfaces.UserRepository
	statusRepo interfaces.StatusRepository
}

func (s *agentAvatarTestStorage) User() interfaces.UserRepository { return s.userRepo }

func (s *agentAvatarTestStorage) Status() interfaces.StatusRepository { return s.statusRepo }

type agentAvatarHarness struct {
	resolver   *Resolver
	withAvatar *storage.User
	bare       *storage.User
	userRepo   *storagemocks.MockUserRepositoryInterface
	statusRepo *storagemocks.MockStatusRepositoryInterface
}

// newAgentAvatarHarness seeds one agent with an avatar and one without, then
// swaps in a user mock and a zero-expectation status mock. Any status read on
// the avatar path panics the mock and fails the calling test.
func newAgentAvatarHarness(t *testing.T) *agentAvatarHarness {
	t.Helper()

	resolver, graphStorage := newRound12GraphResolver(t)
	resolver.Config.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	require.NoError(t, graphStorage.Instance().SetAgentInstanceConfig(context.Background(), policy))

	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	withAvatar := &storage.User{
		Username:     "avatar-agent",
		DisplayName:  "Avatar Agent",
		IsAgent:      true,
		AgentType:    "ASSISTANT",
		AgentVersion: "1.0.0",
		Avatar:       "https://localhost/media/avatars/avatar-agent.png",
		CreatedAt:    now,
	}
	withoutAvatar := &storage.User{
		Username:     "bare-agent",
		DisplayName:  "Bare Agent",
		IsAgent:      true,
		AgentType:    "ASSISTANT",
		AgentVersion: "1.0.0",
		CreatedAt:    now.Add(time.Minute),
	}
	seedGraphAgentContractUser(graphStorage, withAvatar)
	seedGraphAgentContractUser(graphStorage, withoutAvatar)

	userRepo := storagemocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, withAvatar.Username).Return(withAvatar, nil).Once()
	userRepo.On("GetUser", mock.Anything, withoutAvatar.Username).Return(withoutAvatar, nil).Once()
	statusRepo := storagemocks.NewMockStatusRepositoryInterface()

	resolver.Storage = &agentAvatarTestStorage{
		round12GraphStorage: graphStorage,
		userRepo:            userRepo,
		statusRepo:          statusRepo,
	}

	return &agentAvatarHarness{
		resolver:   resolver,
		withAvatar: withAvatar,
		bare:       withoutAvatar,
		userRepo:   userRepo,
		statusRepo: statusRepo,
	}
}

// TestGraphAgentAvatarResolvesWithoutStatusCountScan pins the #1517 contract:
// agent.avatar/avatarStatic come from the account record already loaded by the
// Agent query, are absent (null) when the agent has no avatar, and the
// resolution never issues the post-history status count that
// GET /api/v1/accounts/{id} performs.
func TestGraphAgentAvatarResolvesWithoutStatusCountScan(t *testing.T) {
	h := newAgentAvatarHarness(t)

	agent, err := h.resolver.Query().Agent(context.Background(), h.withAvatar.Username)
	require.NoError(t, err)
	require.NotNil(t, agent)
	require.NotNil(t, agent.Avatar)
	require.Equal(t, h.withAvatar.Avatar, *agent.Avatar)
	require.NotNil(t, agent.AvatarStatic)
	require.Equal(t, *agent.Avatar, *agent.AvatarStatic)

	bare, err := h.resolver.Query().Agent(context.Background(), h.bare.Username)
	require.NoError(t, err)
	require.NotNil(t, bare)
	require.Nil(t, bare.Avatar)
	require.Nil(t, bare.AvatarStatic)

	h.statusRepo.AssertNotCalled(t, "CountStatusesByAuthor", mock.Anything, mock.Anything)
	require.Empty(t, h.statusRepo.Calls, "avatar resolution must not read the status repository at all")
	h.userRepo.AssertExpectations(t)
}

// TestGraphAgentAvatarQueryServesGeneratedSchema runs the exact client query
// from #1517 through the generated executor (the production handler shape) to
// prove the two new fields are wired into the served schema, not just the
// resolver method.
func TestGraphAgentAvatarQueryServesGeneratedSchema(t *testing.T) {
	h := newAgentAvatarHarness(t)

	server := handler.New(NewExecutableSchema(NewConfig(h.resolver)))
	server.AddTransport(transport.POST{})

	avatarFields := graphAgentAvatarQuery(t, server, h.withAvatar.Username)
	require.Equal(t, fmt.Sprintf("%q", h.withAvatar.Avatar), string(avatarFields["avatar"]))
	require.Equal(t, fmt.Sprintf("%q", h.withAvatar.Avatar), string(avatarFields["avatarStatic"]))

	bareFields := graphAgentAvatarQuery(t, server, h.bare.Username)
	require.Equal(t, "null", string(bareFields["avatar"]))
	require.Equal(t, "null", string(bareFields["avatarStatic"]))

	require.Empty(t, h.statusRepo.Calls, "the served query must not read the status repository")
}

// graphAgentAvatarQuery posts the #1517 query and returns the raw JSON values
// for the selected Agent fields, so a null is distinguishable from "".
func graphAgentAvatarQuery(t *testing.T, server *handler.Server, username string) map[string]json.RawMessage {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"query": fmt.Sprintf(`query { agent(username: %q) { username avatar avatarStatic } }`, username),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var payload struct {
		Data struct {
			Agent map[string]json.RawMessage `json:"agent"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Empty(t, payload.Errors, rec.Body.String())
	require.Equal(t, fmt.Sprintf("%q", username), string(payload.Data.Agent["username"]))
	require.Contains(t, payload.Data.Agent, "avatar")
	require.Contains(t, payload.Data.Agent, "avatarStatic")

	return payload.Data.Agent
}
