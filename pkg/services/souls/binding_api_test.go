package souls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testBindingSoulAgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testBindingPrincipal   = "0x1111111111111111111111111111111111111111"
	testBindingInstanceKey = "host-instance-key"
)

func TestService_BindSoulBody_SuccessReplayAndStatusProjection(t *testing.T) {
	var hostCalls int
	host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
		hostCalls++
		require.Equal(t, "Bearer "+testBindingInstanceKey, r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/soul/agents/"+testBindingSoulAgentID, r.URL.Path)
		writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
	})
	defer host.Close()

	repo := newSoulBindingFakeRepo(host.URL)
	svc := newSoulBindingTestService(repo)
	input := testSoulBindingInput("bind-key-1")

	first, err := svc.BindSoulBody(context.Background(), input)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Equal(t, "active", first.Soul.Status)
	require.True(t, first.Soul.Bound)
	require.Equal(t, testBindingSoulAgentID, first.Soul.AgentID)
	require.Equal(t, "drone-ada", first.Soul.BoundAgentUsername)
	require.Equal(t, testBindingPrincipal, first.Soul.BoundPrincipalAddress)
	require.Equal(t, SoulAuthorityModelInstanceTrust, first.Soul.AuthorityModel)
	require.Equal(t, SoulAnchorStateHostedOffchain, first.Soul.AnchorState)
	require.Equal(t, SoulOperationalBindingHostedBound, first.Soul.OperationalBinding)
	require.Equal(t, 1, first.Soul.PublishedVersion)
	require.True(t, strings.HasPrefix(first.PayloadHash, "sha256:"))
	require.Equal(t, 1, repo.bindCalls, "service must write only through BindSoulBody")

	second, err := svc.BindSoulBody(context.Background(), input)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.PayloadHash, second.PayloadHash)
	require.Equal(t, 2, repo.bindCalls, "same-key replay still resolves through Lesser-owned binding writer")

	status, err := svc.GetSoulBinding(context.Background(), testBindingSoulAgentID, "drone-ada")
	require.NoError(t, err)
	require.NotNil(t, status, "projection should be non-nil")
	require.Equal(t, "active", status.Soul.Status)
	require.Equal(t, "drone-ada", status.Soul.BoundAgentUsername)
	require.GreaterOrEqual(t, hostCalls, 3, "POST and GET re-fetch Host source truth")
}

func TestService_BindSoulBody_IdempotencyMismatch(t *testing.T) {
	host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
		writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
	})
	defer host.Close()

	repo := newSoulBindingFakeRepo(host.URL)
	svc := newSoulBindingTestService(repo)

	input := testSoulBindingInput("same-key")
	_, err := svc.BindSoulBody(context.Background(), input)
	require.NoError(t, err)

	input.BodyActorID = "body://ptah/other-correlation"
	_, err = svc.BindSoulBody(context.Background(), input)
	require.ErrorIs(t, err, ErrSoulBindingIdempotencyMismatch)
}

func TestService_BindSoulBody_SamePairAfterReceiptExpiry(t *testing.T) {
	host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
		writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
	})
	defer host.Close()

	repo := newSoulBindingFakeRepo(host.URL)
	repo.bindingsByAgent[testBindingSoulAgentID] = soulBindingTestBinding(testBindingSoulAgentID, "drone-ada")
	repo.bindingsByUser["drone-ada"] = repo.bindingsByAgent[testBindingSoulAgentID]
	svc := newSoulBindingTestService(repo)

	projection, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("receipt-expired-new-key"))
	require.NoError(t, err)
	require.False(t, projection.Replayed, "new idempotency receipt should be reserved, but existing binding is still same-pair idempotent")
	require.True(t, projection.Soul.Bound)
	require.Equal(t, "drone-ada", projection.Soul.BoundAgentUsername)
	require.Equal(t, 1, repo.bindCalls, "same-pair replay after receipt expiry must still resolve through Lesser-owned binding writer")
}

func TestService_BindSoulBody_ConflictCases(t *testing.T) {
	host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
		writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
	})
	defer host.Close()

	t.Run("soul already bound to different body", func(t *testing.T) {
		repo := newSoulBindingFakeRepo(host.URL)
		repo.bindingsByAgent[testBindingSoulAgentID] = soulBindingTestBinding(testBindingSoulAgentID, "other-agent")
		svc := newSoulBindingTestService(repo)

		_, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("conflict-soul"))
		require.ErrorIs(t, err, ErrSoulAlreadyBound)
	})

	t.Run("body already bound to different soul", func(t *testing.T) {
		repo := newSoulBindingFakeRepo(host.URL)
		otherSoul := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		repo.bindingsByUser["drone-ada"] = soulBindingTestBinding(otherSoul, "drone-ada")
		svc := newSoulBindingTestService(repo)

		_, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("conflict-body"))
		require.ErrorIs(t, err, ErrTargetAgentAlreadyHasSoul)
	})
}

func TestService_GetSoulBinding_ActorMismatchAndHostUnavailable(t *testing.T) {
	t.Run("actor mismatch has stable error", func(t *testing.T) {
		host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
			writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
		})
		defer host.Close()

		repo := newSoulBindingFakeRepo(host.URL)
		repo.bindingsByAgent[testBindingSoulAgentID] = soulBindingTestBinding(testBindingSoulAgentID, "drone-ada")
		svc := newSoulBindingTestService(repo)

		_, err := svc.GetSoulBinding(context.Background(), testBindingSoulAgentID, "other-agent")
		require.ErrorIs(t, err, ErrSoulBindingActorMismatch)
	})

	t.Run("local row plus unavailable host fails closed", func(t *testing.T) {
		host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "host unavailable", http.StatusInternalServerError)
		})
		defer host.Close()

		repo := newSoulBindingFakeRepo(host.URL)
		repo.bindingsByAgent[testBindingSoulAgentID] = soulBindingTestBinding(testBindingSoulAgentID, "drone-ada")
		svc := newSoulBindingTestService(repo)

		_, err := svc.GetSoulBinding(context.Background(), testBindingSoulAgentID, "drone-ada")
		require.ErrorIs(t, err, ErrSoulBindingHostRegistryUnavailable)
	})
}

func TestService_BindSoulBody_LocalAndHostRejections(t *testing.T) {
	t.Run("actor must exist", func(t *testing.T) {
		host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
			writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
		})
		defer host.Close()
		repo := newSoulBindingFakeRepo(host.URL)
		repoAccount := &fakeAccountRepo{usersByUsername: map[string]*storage.User{}}
		svc := NewService(repoAccount, repo, &config.Config{Domain: "example.com", LesserHostInstanceKey: testBindingInstanceKey}, zap.NewNop())

		_, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("missing-actor"))
		require.ErrorIs(t, err, ErrTargetAgentNotFound)
	})

	t.Run("actor must be agent", func(t *testing.T) {
		host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
			writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "drone-ada", http.StatusOK)
		})
		defer host.Close()
		repo := newSoulBindingFakeRepo(host.URL)
		repoAccount := &fakeAccountRepo{usersByUsername: map[string]*storage.User{"drone-ada": {Username: "drone-ada", IsAgent: false}}}
		svc := NewService(repoAccount, repo, &config.Config{Domain: "example.com", LesserHostInstanceKey: testBindingInstanceKey}, zap.NewNop())

		_, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("not-agent"))
		require.ErrorIs(t, err, ErrTargetAgentMustBeAgent)
	})

	t.Run("host registry rejection writes no binding", func(t *testing.T) {
		host := newSoulBindingHost(t, func(w http.ResponseWriter, r *http.Request) {
			writeSoulBindingHostIdentity(t, w, testBindingSoulAgentID, "other-agent", http.StatusOK)
		})
		defer host.Close()
		repo := newSoulBindingFakeRepo(host.URL)
		svc := newSoulBindingTestService(repo)

		_, err := svc.BindSoulBody(context.Background(), testSoulBindingInput("host-reject"))
		require.ErrorIs(t, err, ErrSoulBindingHostRegistryRejected)
		require.Nil(t, repo.bindingsByAgent[testBindingSoulAgentID])
		require.Equal(t, 0, repo.bindCalls)
	})
}

func newSoulBindingFakeRepo(hostURL string) *fakeInstanceRepo {
	return &fakeInstanceRepo{
		trust: &storageModels.EffectiveTrustConfig{
			TrustBaseURL: hostURL,
		},
		bindingsByAgent: map[string]*storageModels.InstanceSoulBodyBinding{},
		bindingsByUser:  map[string]*storageModels.InstanceSoulBodyBinding{},
		receipts:        map[string]*storageModels.InstanceSoulBindingIdempotencyReceipt{},
	}
}

func newSoulBindingTestService(repo *fakeInstanceRepo) *Service {
	accountRepo := &fakeAccountRepo{usersByUsername: map[string]*storage.User{
		"drone-ada": {Username: "drone-ada", IsAgent: true},
	}}
	return NewService(accountRepo, repo, &config.Config{Domain: "example.com", LesserHostInstanceKey: testBindingInstanceKey}, zap.NewNop())
}

func testSoulBindingInput(idempotencyKey string) BindSoulBodyInput {
	return BindSoulBodyInput{
		CallerID:             "lesser-body",
		IdempotencyKey:       idempotencyKey,
		ActorUsername:        "drone-ada",
		SoulAgentID:          testBindingSoulAgentID,
		BodyActorID:          "body://ptah/drone-ada",
		HostRegistrationID:   "hreg_01JZPTHOSTREG",
		HostConversationID:   "hconv_01JZPTHOSTCONV",
		AuthorityModel:       SoulAuthorityModelInstanceTrust,
		AnchorState:          SoulAnchorStateHostedOffchain,
		OperationalBinding:   SoulOperationalBindingHostedBound,
		PrincipalAddressHint: "0x2222222222222222222222222222222222222222",
		Evidence: SoulBindingEvidence{
			Source:          "ptah",
			HostRequestID:   "hreq_01JZPTHOSTREQ",
			DeclarationHash: "sha256:4c5835f5c2c84bcaadc17af3c5a5700fdd7f39fb7f61305b02d1a02a0e6c7c56",
			IssuedAt:        "2026-07-14T16:20:00Z",
		},
	}
}

func newSoulBindingHost(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/") {
			http.NotFound(w, r)
			return
		}
		handler(w, r)
	}))
}

func writeSoulBindingHostIdentity(t *testing.T, w http.ResponseWriter, agentID string, localID string, status int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != http.StatusOK {
		return
	}
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"version": "1",
		"agent": map[string]any{
			"agent_id":                 agentID,
			"domain":                   "example.com",
			"local_id":                 localID,
			"wallet":                   testBindingPrincipal,
			"principal_address":        testBindingPrincipal,
			"status":                   "active",
			"lifecycle_status":         "active",
			"authority_model":          SoulAuthorityModelInstanceTrust,
			"anchor_state":             SoulAnchorStateHostedOffchain,
			"operational_binding":      SoulOperationalBindingHostedBound,
			"published_version":        1,
			"self_description_version": 1,
		},
	}))
}

func soulBindingTestBinding(agentID string, username string) *storageModels.InstanceSoulBodyBinding {
	binding := &storageModels.InstanceSoulBodyBinding{
		AgentID:          strings.ToLower(strings.TrimSpace(agentID)),
		Username:         strings.TrimSpace(username),
		PrincipalAddress: testBindingPrincipal,
	}
	if err := binding.UpdateKeys(); err != nil {
		panic(err)
	}
	return binding
}
