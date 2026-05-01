package souls

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeAccountRepo struct {
	wallets         []*storage.WalletCredential
	walletsByUser   map[string][]*storage.WalletCredential
	usersByUsername map[string]*storage.User
	err             error
	getUserErr      error
}

func (f *fakeAccountRepo) GetUserWalletCredentials(_ context.Context, username string) ([]*storage.WalletCredential, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.walletsByUser != nil {
		return f.walletsByUser[strings.TrimSpace(username)], nil
	}
	return f.wallets, nil
}

func (f *fakeAccountRepo) GetUser(_ context.Context, username string) (*storage.User, error) {
	if f.getUserErr != nil {
		return nil, f.getUserErr
	}
	if f.usersByUsername == nil {
		return nil, nil
	}
	return f.usersByUsername[strings.TrimSpace(username)], nil
}

type fakeInstanceRepo struct {
	trust           *storageModels.EffectiveTrustConfig
	trustErr        error
	bindingsByAgent map[string]*storageModels.InstanceSoulBodyBinding
	bindingsByUser  map[string]*storageModels.InstanceSoulBodyBinding
	bindErr         error
}

func (f *fakeInstanceRepo) EffectiveTrustConfig(_ context.Context) (*storageModels.EffectiveTrustConfig, error) {
	if f.trustErr != nil {
		return nil, f.trustErr
	}
	return f.trust, nil
}

func (f *fakeInstanceRepo) GetSoulBodyBinding(_ context.Context, agentID string) (*storageModels.InstanceSoulBodyBinding, error) {
	if f == nil {
		return nil, nil
	}
	return f.bindingsByAgent[strings.ToLower(strings.TrimSpace(agentID))], nil
}

func (f *fakeInstanceRepo) GetSoulBodyBindingByUsername(_ context.Context, username string) (*storageModels.InstanceSoulBodyBinding, error) {
	if f == nil {
		return nil, nil
	}
	return f.bindingsByUser[strings.TrimSpace(username)], nil
}

func (f *fakeInstanceRepo) BindSoulBody(_ context.Context, agentID string, username string, principalAddress string) (*storageModels.InstanceSoulBodyBinding, error) {
	if f.bindErr != nil {
		return nil, f.bindErr
	}

	normalizedAgentID := strings.ToLower(strings.TrimSpace(agentID))
	normalizedUsername := strings.TrimSpace(username)

	if existing := f.bindingsByAgent[normalizedAgentID]; existing != nil {
		if strings.EqualFold(existing.Username, normalizedUsername) {
			return existing, nil
		}
		return nil, storageRepos.ErrSoulBodyBindingAlreadyExists
	}

	if existing := f.bindingsByUser[normalizedUsername]; existing != nil {
		if strings.EqualFold(existing.AgentID, normalizedAgentID) {
			return existing, nil
		}
		return nil, storageRepos.ErrSoulBodyAlreadyHasBinding
	}

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	binding := &storageModels.InstanceSoulBodyBinding{
		AgentID:          normalizedAgentID,
		Username:         normalizedUsername,
		PrincipalAddress: strings.ToLower(strings.TrimSpace(principalAddress)),
		BoundAt:          now,
		UpdatedAt:        now,
	}
	if err := binding.UpdateKeys(); err != nil {
		return nil, err
	}

	if f.bindingsByAgent == nil {
		f.bindingsByAgent = map[string]*storageModels.InstanceSoulBodyBinding{}
	}
	if f.bindingsByUser == nil {
		f.bindingsByUser = map[string]*storageModels.InstanceSoulBodyBinding{}
	}
	f.bindingsByAgent[normalizedAgentID] = binding
	f.bindingsByUser[normalizedUsername] = binding
	return binding, nil
}

type errorRoundTripper struct {
	err error
}

func (e errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

func TestService_ListMine_FollowsPaginationAndAnnotatesBindings(t *testing.T) {
	t.Parallel()

	const (
		walletAlice = "0x1111111111111111111111111111111111111111"
		walletAlt   = "0x2222222222222222222222222222222222222222"
		agentAlpha  = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		agentBeta   = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		agentGamma  = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		agentDelta  = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	)
	alphaENS := "alpha.eth"
	betaENS := "beta.eth"

	var mu sync.Mutex
	searchCalls := map[string][]string{}
	agentHits := map[string]int{}

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/soul/search":
			principal := r.URL.Query().Get("principal")
			cursor := r.URL.Query().Get("cursor")

			mu.Lock()
			searchCalls[principal] = append(searchCalls[principal], cursor)
			mu.Unlock()

			switch {
			case principal == walletAlice && cursor == "":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{
						{"agent_id": agentAlpha},
						{"agent_id": agentDelta},
					},
					"count":       2,
					"has_more":    true,
					"next_cursor": "page-2",
				}))
			case principal == walletAlice && cursor == "page-2":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{
						{"agent_id": agentBeta},
					},
					"count":    1,
					"has_more": false,
				}))
			case principal == walletAlt && cursor == "":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{
						{"agent_id": agentBeta},
						{"agent_id": agentGamma},
					},
					"count":    2,
					"has_more": false,
				}))
			default:
				http.NotFound(w, r)
			}
			return

		case strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/"):
			agentID := strings.TrimPrefix(r.URL.Path, "/api/v1/soul/agents/")

			mu.Lock()
			agentHits[agentID]++
			mu.Unlock()

			var agent map[string]any
			switch agentID {
			case agentAlpha:
				agent = map[string]any{
					"agent_id":          agentAlpha,
					"domain":            "example.com",
					"local_id":          "alpha",
					"ens_name":          alphaENS,
					"wallet":            walletAlice,
					"principal_address": walletAlice,
					"status":            "active",
					"lifecycle_status":  "active",
					"updated_at":        now.Format(time.RFC3339),
				}
			case agentBeta:
				agent = map[string]any{
					"agent_id":          agentBeta,
					"domain":            "example.com",
					"local_id":          "beta",
					"ens_name":          betaENS,
					"wallet":            walletAlt,
					"principal_address": walletAlt,
					"status":            "active",
					"lifecycle_status":  "active",
					"updated_at":        now.Add(time.Minute).Format(time.RFC3339),
				}
			case agentGamma:
				agent = map[string]any{
					"agent_id":          agentGamma,
					"domain":            "example.com",
					"local_id":          "gamma",
					"ens_name":          "   ",
					"wallet":            walletAlt,
					"principal_address": "",
					"status":            "active",
					"lifecycle_status":  "active",
				}
			case agentDelta:
				agent = map[string]any{
					"agent_id":          agentDelta,
					"domain":            "other.example",
					"local_id":          "delta",
					"wallet":            walletAlice,
					"principal_address": walletAlice,
					"status":            "active",
					"lifecycle_status":  "active",
				}
			default:
				http.NotFound(w, r)
				return
			}

			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version": "1",
				"agent":   agent,
			}))
			return
		}

		http.NotFound(w, r)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{
			wallets: []*storage.WalletCredential{
				{Address: walletAlice},
				{Address: walletAlt},
			},
		},
		&fakeInstanceRepo{
			trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
			bindingsByAgent: map[string]*storageModels.InstanceSoulBodyBinding{
				agentBeta: {
					AgentID:          agentBeta,
					Username:         "alice",
					PrincipalAddress: walletAlt,
					BoundAt:          now,
					UpdatedAt:        now,
				},
				agentGamma: {
					AgentID:          agentGamma,
					Username:         "bob",
					PrincipalAddress: walletAlt,
					BoundAt:          now,
					UpdatedAt:        now,
				},
			},
		},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	souls, err := service.ListMine(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, souls, 3)

	require.Equal(t, agentAlpha, souls[0].AgentID)
	require.False(t, souls[0].Bound)
	require.NotNil(t, souls[0].ENSName)
	require.Equal(t, alphaENS, *souls[0].ENSName)

	require.Equal(t, agentBeta, souls[1].AgentID)
	require.True(t, souls[1].Bound)
	require.Equal(t, "alice", souls[1].BoundAgentUsername)
	require.NotNil(t, souls[1].ENSName)
	require.Equal(t, betaENS, *souls[1].ENSName)

	require.Equal(t, agentGamma, souls[2].AgentID)
	require.True(t, souls[2].Bound)
	require.Equal(t, "bob", souls[2].BoundAgentUsername)
	require.Equal(t, walletAlt, souls[2].BoundPrincipalAddress)
	require.Nil(t, souls[2].ENSName)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"", "page-2"}, searchCalls[walletAlice])
	require.Equal(t, []string{""}, searchCalls[walletAlt])
	require.Equal(t, 1, agentHits[agentAlpha])
	require.Equal(t, 1, agentHits[agentBeta])
	require.Equal(t, 1, agentHits[agentGamma])
	require.Equal(t, 1, agentHits[agentDelta])
}

func TestService_Incorporate_Success(t *testing.T) {
	t.Parallel()

	const (
		walletAlice = "0x3333333333333333333333333333333333333333"
		agentAlpha  = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)
	ensName := "alpha.eth"
	targetAgentUsername := "agent-alpha"

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "1",
			"agent": map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"ens_name":          ensName,
				"wallet":            walletAlice,
				"principal_address": walletAlice,
				"status":            "active",
				"lifecycle_status":  "active",
			},
		}))
	}))
	defer host.Close()

	instanceRepo := &fakeInstanceRepo{
		trust:           &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
		bindingsByAgent: map[string]*storageModels.InstanceSoulBodyBinding{},
		bindingsByUser:  map[string]*storageModels.InstanceSoulBodyBinding{},
	}

	service := NewService(
		&fakeAccountRepo{
			wallets: []*storage.WalletCredential{{Address: walletAlice}},
			usersByUsername: map[string]*storage.User{
				targetAgentUsername: {
					Username:   targetAgentUsername,
					IsAgent:    true,
					AgentOwner: "@alice",
				},
			},
		},
		instanceRepo,
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	soul, err := service.Incorporate(context.Background(), "alice", targetAgentUsername, agentAlpha)
	require.NoError(t, err)
	require.NotNil(t, soul)
	require.True(t, soul.Bound)
	require.Equal(t, targetAgentUsername, soul.BoundAgentUsername)
	require.Equal(t, walletAlice, soul.BoundPrincipalAddress)
	require.NotNil(t, soul.ENSName)
	require.Equal(t, ensName, *soul.ENSName)
}

func TestService_Incorporate_MapsAvailabilityAndConflictErrors(t *testing.T) {
	t.Parallel()

	const (
		walletAlice = "0x4444444444444444444444444444444444444444"
		walletOther = "0x5555555555555555555555555555555555555555"
		agentAlpha  = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	)
	targetAgentUsername := "agent-alpha"

	testCases := []struct {
		name         string
		trustBaseURL string
		identity     map[string]any
		bindErr      error
		agentID      string
		wantErr      error
	}{
		{
			name:     "trust not configured",
			identity: nil,
			agentID:  agentAlpha,
			wantErr:  ErrTrustNotConfigured,
		},
		{
			name:         "invalid agent id",
			trustBaseURL: "http://placeholder.invalid",
			identity:     nil,
			agentID:      "bad-agent",
			wantErr:      ErrSoulNotAvailable,
		},
		{
			name:         "owner mismatch",
			trustBaseURL: "",
			identity: map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"wallet":            walletOther,
				"principal_address": walletOther,
				"status":            "active",
				"lifecycle_status":  "active",
			},
			agentID: agentAlpha,
			wantErr: ErrSoulNotAvailable,
		},
		{
			name:         "already bound elsewhere",
			trustBaseURL: "",
			identity: map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"wallet":            walletAlice,
				"principal_address": walletAlice,
				"status":            "active",
				"lifecycle_status":  "active",
			},
			bindErr: storageRepos.ErrSoulBodyBindingAlreadyExists,
			agentID: agentAlpha,
			wantErr: ErrSoulAlreadyBound,
		},
		{
			name:         "target agent already has soul",
			trustBaseURL: "",
			identity: map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"wallet":            walletAlice,
				"principal_address": walletAlice,
				"status":            "active",
				"lifecycle_status":  "active",
			},
			bindErr: storageRepos.ErrSoulBodyAlreadyHasBinding,
			agentID: agentAlpha,
			wantErr: ErrTargetAgentAlreadyHasSoul,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			trustBaseURL := tc.trustBaseURL
			var host *httptest.Server
			if tc.identity != nil {
				host = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"version": "1",
						"agent":   tc.identity,
					}))
				}))
				defer host.Close()
				trustBaseURL = host.URL
			}

			service := NewService(
				&fakeAccountRepo{
					wallets: []*storage.WalletCredential{{Address: walletAlice}},
					usersByUsername: map[string]*storage.User{
						targetAgentUsername: {
							Username:   targetAgentUsername,
							IsAgent:    true,
							AgentOwner: "@alice",
						},
					},
				},
				&fakeInstanceRepo{
					trust:           &storageModels.EffectiveTrustConfig{TrustBaseURL: trustBaseURL},
					bindErr:         tc.bindErr,
					bindingsByAgent: map[string]*storageModels.InstanceSoulBodyBinding{},
					bindingsByUser:  map[string]*storageModels.InstanceSoulBodyBinding{},
				},
				&config.Config{Domain: "example.com"},
				zap.NewNop(),
			)
			if host != nil {
				service = service.WithHTTPClient(host.Client())
			}

			_, err := service.Incorporate(context.Background(), "alice", targetAgentUsername, tc.agentID)
			require.Error(t, err)
			require.True(t, errors.Is(err, tc.wantErr), "expected %v, got %v", tc.wantErr, err)
		})
	}
}

func TestService_Incorporate_RejectsInvalidTargetAgents(t *testing.T) {
	t.Parallel()

	const validSoulAgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	testCases := []struct {
		name                string
		targetAgentUsername string
		usersByUsername     map[string]*storage.User
		wantErr             error
	}{
		{
			name:                "target agent required",
			targetAgentUsername: " ",
			wantErr:             ErrTargetAgentRequired,
		},
		{
			name:                "target agent not found",
			targetAgentUsername: "missing-agent",
			wantErr:             ErrTargetAgentNotFound,
		},
		{
			name:                "target account must be an agent",
			targetAgentUsername: "owner",
			usersByUsername: map[string]*storage.User{
				"owner": {Username: "owner", IsAgent: false},
			},
			wantErr: ErrTargetAgentMustBeAgent,
		},
		{
			name:                "target agent not owned by principal",
			targetAgentUsername: "other-agent",
			usersByUsername: map[string]*storage.User{
				"other-agent": {Username: "other-agent", IsAgent: true, AgentOwner: "@bob"},
			},
			wantErr: ErrTargetAgentNotOwned,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := NewService(
				&fakeAccountRepo{
					wallets:         []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}},
					usersByUsername: tc.usersByUsername,
				},
				&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
				&config.Config{Domain: "example.com"},
				zap.NewNop(),
			)

			_, err := service.Incorporate(context.Background(), "alice", tc.targetAgentUsername, validSoulAgentID)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestService_DiscoveryInputsAndHTTPErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("misconfigured service", func(t *testing.T) {
		t.Parallel()

		service := &Service{}
		_, _, _, err := service.discoveryInputs(context.Background(), "alice")
		require.Error(t, err)
	})

	t.Run("empty username", func(t *testing.T) {
		t.Parallel()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)
		_, _, _, err := service.discoveryInputs(context.Background(), " ")
		require.Error(t, err)
	})

	t.Run("account repository error", func(t *testing.T) {
		t.Parallel()

		service := NewService(
			&fakeAccountRepo{err: errors.New("boom")},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)
		_, _, _, err := service.discoveryInputs(context.Background(), "alice")
		require.Error(t, err)
	})

	t.Run("trust resolution error", func(t *testing.T) {
		t.Parallel()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trustErr: errors.New("trust lookup failed")},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)
		_, _, _, err := service.discoveryInputs(context.Background(), "alice")
		require.Error(t, err)
	})

	t.Run("empty instance domain", func(t *testing.T) {
		t.Parallel()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
			&config.Config{},
			zap.NewNop(),
		)
		_, _, _, err := service.discoveryInputs(context.Background(), "alice")
		require.Error(t, err)
	})

	t.Run("search non-200", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.searchSoulsPage(context.Background(), host.URL, "example.com", "0x1111111111111111111111111111111111111111", "")
		require.Error(t, err)
	})

	t.Run("search invalid base url", func(t *testing.T) {
		t.Parallel()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop())
		_, err := service.searchSoulsPage(context.Background(), "://bad", "example.com", "0x1111111111111111111111111111111111111111", "")
		require.Error(t, err)
	})

	t.Run("search transport error", func(t *testing.T) {
		t.Parallel()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop()).
			WithHTTPClient(&http.Client{Transport: errorRoundTripper{err: errors.New("dial failed")}})
		_, err := service.searchSoulsPage(context.Background(), "https://trust.example", "example.com", "0x1111111111111111111111111111111111111111", "")
		require.Error(t, err)
	})

	t.Run("search bad json", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.searchSoulsPage(context.Background(), host.URL, "example.com", "0x1111111111111111111111111111111111111111", "")
		require.Error(t, err)
	})

	t.Run("fetch not found", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.fetchSoulIdentity(context.Background(), host.URL, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.ErrorIs(t, err, ErrSoulNotAvailable)
	})

	t.Run("fetch non-200", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.fetchSoulIdentity(context.Background(), host.URL, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.Error(t, err)
	})

	t.Run("fetch invalid base url", func(t *testing.T) {
		t.Parallel()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop())
		_, err := service.fetchSoulIdentity(context.Background(), "://bad", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.Error(t, err)
	})

	t.Run("fetch transport error", func(t *testing.T) {
		t.Parallel()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop()).
			WithHTTPClient(&http.Client{Transport: errorRoundTripper{err: errors.New("dial failed")}})
		_, err := service.fetchSoulIdentity(context.Background(), "https://trust.example", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.Error(t, err)
	})

	t.Run("fetch bad json", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.fetchSoulIdentity(context.Background(), host.URL, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.Error(t, err)
	})
}

func TestSoulHelpers(t *testing.T) {
	t.Parallel()

	require.Nil(t, (*Service)(nil).WithHTTPClient(&http.Client{}))

	service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop())
	require.Same(t, service, service.WithHTTPClient(nil))

	require.Empty(t, canonicalOwnerWallets([]*storage.WalletCredential{
		nil,
		{Address: "bad"},
	}))
	require.Len(t, canonicalOwnerWallets([]*storage.WalletCredential{
		{Address: "0x1111111111111111111111111111111111111111"},
	}), 1)

	_, err := validateAgentID("")
	require.Error(t, err)
	_, err = validateAgentID("0x1234")
	require.Error(t, err)

	validAgent := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	normalized, err := validateAgentID(strings.ToUpper(validAgent))
	require.NoError(t, err)
	require.Equal(t, validAgent, normalized)

	require.False(t, identityMatchesOwnerWallets(nil, map[string]struct{}{"0x1111111111111111111111111111111111111111": {}}))
	require.False(t, identityMatchesOwnerWallets(&hostSoulIdentity{Wallet: "bad"}, map[string]struct{}{"0x1111111111111111111111111111111111111111": {}}))
	require.True(t, identityMatchesOwnerWallets(&hostSoulIdentity{Wallet: "0x1111111111111111111111111111111111111111"}, map[string]struct{}{"0x1111111111111111111111111111111111111111": {}}))

	require.Empty(t, canonicalOwnerAddress(nil))
	require.Equal(t,
		"0x1111111111111111111111111111111111111111",
		canonicalOwnerAddress(&hostSoulIdentity{Wallet: "0x1111111111111111111111111111111111111111"}),
	)
	require.Empty(t, canonicalOwnerAddress(&hostSoulIdentity{Wallet: "bad"}))

	require.False(t, identityIsActive(nil))
	require.True(t, identityIsActive(&hostSoulIdentity{LifecycleStatus: "active"}))
	require.True(t, identityIsActive(&hostSoulIdentity{Status: "active"}))
	require.False(t, identityIsActive(&hostSoulIdentity{Status: "inactive"}))

	require.True(t, domainMatches("Example.com", "example.com"))
	require.False(t, domainMatches("example.org", "example.com"))

	require.True(t, agentOwnedByPrincipal(&storage.User{Username: "agent-alpha", IsAgent: true, AgentOwner: "@alice"}, "alice"))
	require.True(t, agentOwnedByPrincipal(&storage.User{Username: "agent-alpha", IsAgent: true}, "agent-alpha"))
	require.False(t, agentOwnedByPrincipal(&storage.User{Username: "agent-alpha", IsAgent: true, AgentOwner: "@bob"}, "alice"))

	require.Nil(t, normalizedOptionalString(nil))
	blank := "   "
	require.Nil(t, normalizedOptionalString(&blank))
	value := " alpha.eth "
	normalizedValue := normalizedOptionalString(&value)
	require.NotNil(t, normalizedValue)
	require.Equal(t, "alpha.eth", *normalizedValue)

	require.Nil(t, cloneSoulAvatar(nil))

	currentStyleID := 2
	ensName := " alpha.eth "
	mintedAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 4, 2, 13, 30, 0, 0, time.UTC)
	mapped := soulFromIdentity(&hostSoulIdentity{
		AgentID:                "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Domain:                 " example.com ",
		LocalID:                " agent-alpha ",
		ENSName:                &ensName,
		Wallet:                 "0x1111111111111111111111111111111111111111",
		TokenID:                " 42 ",
		MetaURI:                " https://example.com/meta.json ",
		Avatar:                 &hostSoulAvatar{TokenURI: " https://example.com/token.json ", Image: " data:image/svg+xml;base64,abc ", CurrentStyleID: &currentStyleID, CurrentStyleName: " Sigil ", CurrentRendererAddress: " 0xABCDEF ", Styles: []hostSoulAvatarStyle{{StyleID: 2, StyleName: " Sigil ", RendererAddress: " 0x1234 ", Image: " data:image/svg+xml;base64,style ", Selected: true}}},
		PrincipalAddress:       "0x2222222222222222222222222222222222222222",
		PrincipalSignature:     " 0xdeadbeef ",
		PrincipalDeclaration:   " I accept responsibility. ",
		PrincipalDeclaredAt:    " 2026-04-01T12:00:00Z ",
		Status:                 " active ",
		LifecycleStatus:        " active ",
		LifecycleReason:        " steady ",
		SuccessorAgentID:       " 0xbb ",
		PredecessorAgentID:     " 0xcc ",
		SelfDescriptionVersion: &currentStyleID,
		Capabilities:           []string{"chat"},
		MintTxHash:             " 0xfeed ",
		MintedAt:               &mintedAt,
		UpdatedAt:              &updatedAt,
	}, &storageModels.InstanceSoulBodyBinding{
		Username:         "agent-alpha",
		PrincipalAddress: "0x3333333333333333333333333333333333333333",
		BoundAt:          mintedAt,
		UpdatedAt:        updatedAt,
	})
	require.Equal(t, "example.com", mapped.Domain)
	require.Equal(t, "agent-alpha", mapped.LocalID)
	require.NotNil(t, mapped.ENSName)
	require.Equal(t, "alpha.eth", *mapped.ENSName)
	require.Equal(t, "42", mapped.TokenID)
	require.Equal(t, "https://example.com/meta.json", mapped.MetaURI)
	require.NotNil(t, mapped.Avatar)
	require.Equal(t, "https://example.com/token.json", mapped.Avatar.TokenURI)
	require.Equal(t, "Sigil", mapped.Avatar.CurrentStyleName)
	require.Equal(t, "0xabcdef", mapped.Avatar.CurrentRendererAddress)
	require.Len(t, mapped.Avatar.Styles, 1)
	require.Equal(t, "0x1234", mapped.Avatar.Styles[0].RendererAddress)
	require.True(t, mapped.Bound)
	require.Equal(t, "agent-alpha", mapped.BoundAgentUsername)
	require.Equal(t, "0x3333333333333333333333333333333333333333", mapped.BoundPrincipalAddress)
	require.Equal(t, mintedAt, mapped.BoundAt)
	require.Equal(t, updatedAt, mapped.BoundUpdatedAt)

	_, err = validateAgentID("0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
	require.Error(t, err)
}

func TestService_ListMineAndSearchEdgePaths(t *testing.T) {
	t.Parallel()

	t.Run("no owner wallets returns empty", func(t *testing.T) {
		t.Parallel()

		service := NewService(
			&fakeAccountRepo{},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)

		souls, err := service.ListMine(context.Background(), "alice")
		require.NoError(t, err)
		require.Empty(t, souls)
	})

	t.Run("discovery input error is propagated", func(t *testing.T) {
		t.Parallel()

		accountErr := errors.New("wallet lookup failed")
		service := NewService(
			&fakeAccountRepo{err: accountErr},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		)

		_, err := service.ListMine(context.Background(), "alice")
		require.ErrorIs(t, err, accountErr)
	})

	t.Run("discovered agent removed when identity disappears", func(t *testing.T) {
		t.Parallel()

		const agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/soul/search":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{{"agent_id": agentAlpha}},
					"count":   1,
				}))
			case strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/"):
				http.NotFound(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		souls, err := service.ListMine(context.Background(), "alice")
		require.NoError(t, err)
		require.Empty(t, souls)
	})

	t.Run("discover error is propagated", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.ListMine(context.Background(), "alice")
		require.Error(t, err)
	})

	t.Run("invalid discovered ids result in empty list", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/soul/search" {
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{{"agent_id": "bad-agent"}},
					"count":   1,
				}))
				return
			}
			http.NotFound(w, r)
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		souls, err := service.ListMine(context.Background(), "alice")
		require.NoError(t, err)
		require.Empty(t, souls)
	})

	t.Run("binding lookup error is propagated", func(t *testing.T) {
		t.Parallel()

		const agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		bindingErr := errors.New("binding lookup failed")

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/soul/search":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{{"agent_id": agentAlpha}},
					"count":   1,
				}))
			case strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/"):
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"agent": map[string]any{
						"agent_id":          agentAlpha,
						"domain":            "example.com",
						"local_id":          "alpha",
						"wallet":            "0x1111111111111111111111111111111111111111",
						"principal_address": "0x1111111111111111111111111111111111111111",
						"status":            "active",
						"lifecycle_status":  "active",
					},
				}))
			default:
				http.NotFound(w, r)
			}
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&erroringInstanceRepo{
				instanceRepository: &fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
				getBindingErr:      bindingErr,
			},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.ListMine(context.Background(), "alice")
		require.ErrorIs(t, err, bindingErr)
	})

	t.Run("identity fetch error is propagated", func(t *testing.T) {
		t.Parallel()

		const agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/soul/search":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{{"agent_id": agentAlpha}},
					"count":   1,
				}))
			case strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/"):
				http.Error(w, "boom", http.StatusBadGateway)
			default:
				http.NotFound(w, r)
			}
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		_, err := service.ListMine(context.Background(), "alice")
		require.Error(t, err)
		require.Contains(t, err.Error(), "get soul agent failed")
	})

	t.Run("search stops after max pages", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version":     "1",
				"results":     []map[string]any{{"agent_id": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
				"count":       1,
				"has_more":    true,
				"next_cursor": "next",
			}))
		}))
		defer host.Close()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop()).
			WithHTTPClient(host.Client())

		results, err := service.searchSouls(context.Background(), host.URL, "example.com", "0x1111111111111111111111111111111111111111")
		require.NoError(t, err)
		require.Len(t, results, maxSoulSearchPages)
	})

	t.Run("same local id sorts by agent id", func(t *testing.T) {
		t.Parallel()

		const (
			agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			agentBeta  = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		)

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v1/soul/search":
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"results": []map[string]any{{"agent_id": agentBeta}, {"agent_id": agentAlpha}},
					"count":   2,
				}))
			case strings.HasPrefix(r.URL.Path, "/api/v1/soul/agents/"):
				agentID := strings.TrimPrefix(r.URL.Path, "/api/v1/soul/agents/")
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"agent": map[string]any{
						"agent_id":          agentID,
						"domain":            "example.com",
						"local_id":          "shared",
						"wallet":            "0x1111111111111111111111111111111111111111",
						"principal_address": "0x1111111111111111111111111111111111111111",
						"status":            "active",
						"lifecycle_status":  "active",
					},
				}))
			default:
				http.NotFound(w, r)
			}
		}))
		defer host.Close()

		service := NewService(
			&fakeAccountRepo{wallets: []*storage.WalletCredential{{Address: "0x1111111111111111111111111111111111111111"}}},
			&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
			&config.Config{Domain: "example.com"},
			zap.NewNop(),
		).WithHTTPClient(host.Client())

		souls, err := service.ListMine(context.Background(), "alice")
		require.NoError(t, err)
		require.Len(t, souls, 2)
		require.Equal(t, agentAlpha, souls[0].AgentID)
		require.Equal(t, agentBeta, souls[1].AgentID)
	})

	t.Run("fetch invalid agent id payload fails", func(t *testing.T) {
		t.Parallel()

		host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version": "1",
				"agent": map[string]any{
					"agent_id": "bad-agent",
				},
			}))
		}))
		defer host.Close()

		service := NewService(&fakeAccountRepo{}, &fakeInstanceRepo{}, &config.Config{Domain: "example.com"}, zap.NewNop()).
			WithHTTPClient(host.Client())

		_, err := service.fetchSoulIdentity(context.Background(), host.URL, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		require.Error(t, err)
	})
}

func TestService_Incorporate_UnexpectedBindingError(t *testing.T) {
	t.Parallel()

	const (
		walletAlice = "0x6666666666666666666666666666666666666666"
		agentAlpha  = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	targetAgentUsername := "agent-alpha"

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "1",
			"agent": map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"wallet":            walletAlice,
				"principal_address": walletAlice,
				"status":            "active",
				"lifecycle_status":  "active",
			},
		}))
	}))
	defer host.Close()

	bindErr := errors.New("bind failed")
	service := NewService(
		&fakeAccountRepo{
			wallets: []*storage.WalletCredential{{Address: walletAlice}},
			usersByUsername: map[string]*storage.User{
				targetAgentUsername: {
					Username:   targetAgentUsername,
					IsAgent:    true,
					AgentOwner: "@alice",
				},
			},
		},
		&fakeInstanceRepo{
			trust:           &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
			bindErr:         bindErr,
			bindingsByAgent: map[string]*storageModels.InstanceSoulBodyBinding{},
			bindingsByUser:  map[string]*storageModels.InstanceSoulBodyBinding{},
		},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.Incorporate(context.Background(), "alice", targetAgentUsername, agentAlpha)
	require.ErrorIs(t, err, bindErr)
}

func TestService_Incorporate_FetchIdentityError(t *testing.T) {
	t.Parallel()

	const (
		walletAlice = "0x7777777777777777777777777777777777777777"
		agentAlpha  = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	targetAgentUsername := "agent-alpha"

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer host.Close()

	service := NewService(
		&fakeAccountRepo{
			wallets: []*storage.WalletCredential{{Address: walletAlice}},
			usersByUsername: map[string]*storage.User{
				targetAgentUsername: {
					Username:   targetAgentUsername,
					IsAgent:    true,
					AgentOwner: "@alice",
				},
			},
		},
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL}},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	_, err := service.Incorporate(context.Background(), "alice", targetAgentUsername, agentAlpha)
	require.ErrorIs(t, err, ErrSoulNotAvailable)
}

func TestService_ResolveBoundAgent_ReturnsCanonicalBoundSoul(t *testing.T) {
	t.Parallel()

	const (
		agentAlpha      = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		agentWallet     = "0x1111111111111111111111111111111111111111"
		principalWallet = "0x2222222222222222222222222222222222222222"
	)

	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/soul/agents/"+agentAlpha, r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"version": "1",
			"agent": map[string]any{
				"agent_id":          agentAlpha,
				"domain":            "example.com",
				"local_id":          "alpha",
				"wallet":            agentWallet,
				"principal_address": principalWallet,
				"status":            "active",
				"lifecycle_status":  "active",
			},
		}))
	}))
	defer host.Close()

	service := NewService(
		nil,
		&fakeInstanceRepo{
			trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
			bindingsByUser: map[string]*storageModels.InstanceSoulBodyBinding{
				"agent-alpha": {
					AgentID:          agentAlpha,
					Username:         "agent-alpha",
					PrincipalAddress: principalWallet,
				},
			},
		},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	soul, err := service.ResolveBoundAgent(context.Background(), "agent-alpha")
	require.NoError(t, err)
	require.NotNil(t, soul)
	require.True(t, soul.Bound)
	require.Equal(t, "agent-alpha", soul.BoundAgentUsername)
	require.Equal(t, principalWallet, soul.BoundPrincipalAddress)
	require.Equal(t, agentWallet, soul.Wallet)
}

func TestService_ResolveBoundAgent_ReturnsNilWhenUnbound(t *testing.T) {
	t.Parallel()

	service := NewService(
		nil,
		&fakeInstanceRepo{trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: "https://trust.example"}},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	)

	soul, err := service.ResolveBoundAgent(context.Background(), "agent-alpha")
	require.NoError(t, err)
	require.Nil(t, soul)
}

func TestService_ResolveBoundAgent_FailsClosedForUnavailableHostIdentity(t *testing.T) {
	t.Parallel()

	const agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	testCases := []struct {
		name     string
		response map[string]any
	}{
		{
			name: "wrong domain",
			response: map[string]any{
				"agent_id":         agentAlpha,
				"domain":           "other.example",
				"local_id":         "alpha",
				"wallet":           "0x1111111111111111111111111111111111111111",
				"status":           "active",
				"lifecycle_status": "active",
			},
		},
		{
			name: "inactive lifecycle",
			response: map[string]any{
				"agent_id":         agentAlpha,
				"domain":           "example.com",
				"local_id":         "alpha",
				"wallet":           "0x1111111111111111111111111111111111111111",
				"status":           "active",
				"lifecycle_status": "retired",
			},
		},
		{
			name: "inactive status",
			response: map[string]any{
				"agent_id": agentAlpha,
				"domain":   "example.com",
				"local_id": "alpha",
				"wallet":   "0x1111111111111111111111111111111111111111",
				"status":   "inactive",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/soul/agents/"+agentAlpha, r.URL.Path)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					"version": "1",
					"agent":   tc.response,
				}))
			}))
			defer host.Close()

			service := NewService(
				nil,
				&fakeInstanceRepo{
					trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
					bindingsByUser: map[string]*storageModels.InstanceSoulBodyBinding{
						"agent-alpha": {
							AgentID:  agentAlpha,
							Username: "agent-alpha",
						},
					},
				},
				&config.Config{Domain: "example.com"},
				zap.NewNop(),
			).WithHTTPClient(host.Client())

			soul, err := service.ResolveBoundAgent(context.Background(), "agent-alpha")
			require.ErrorIs(t, err, ErrSoulNotAvailable)
			require.Nil(t, soul)
		})
	}
}

func TestService_ResolveBoundAgent_FailsClosedWhenHostIdentityMissing(t *testing.T) {
	t.Parallel()

	const agentAlpha = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	host := httptest.NewServer(http.NotFoundHandler())
	defer host.Close()

	service := NewService(
		nil,
		&fakeInstanceRepo{
			trust: &storageModels.EffectiveTrustConfig{TrustBaseURL: host.URL},
			bindingsByUser: map[string]*storageModels.InstanceSoulBodyBinding{
				"agent-alpha": {
					AgentID:  agentAlpha,
					Username: "agent-alpha",
				},
			},
		},
		&config.Config{Domain: "example.com"},
		zap.NewNop(),
	).WithHTTPClient(host.Client())

	soul, err := service.ResolveBoundAgent(context.Background(), "agent-alpha")
	require.ErrorIs(t, err, ErrSoulNotAvailable)
	require.Nil(t, soul)
}

type erroringInstanceRepo struct {
	instanceRepository
	getBindingErr error
}

func (e *erroringInstanceRepo) GetSoulBodyBinding(ctx context.Context, agentID string) (*storageModels.InstanceSoulBodyBinding, error) {
	if e.getBindingErr != nil {
		return nil, e.getBindingErr
	}
	return e.instanceRepository.GetSoulBodyBinding(ctx, agentID)
}

func (e *erroringInstanceRepo) GetSoulBodyBindingByUsername(ctx context.Context, username string) (*storageModels.InstanceSoulBodyBinding, error) {
	if e.getBindingErr != nil {
		return nil, e.getBindingErr
	}
	return e.instanceRepository.GetSoulBodyBindingByUsername(ctx, username)
}
