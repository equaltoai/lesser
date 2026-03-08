// Package souls discovers principal-owned lesser souls and manages local incorporation bindings.
package souls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/zap"
)

var (
	// ErrTrustNotConfigured indicates the instance trust base URL is unavailable.
	ErrTrustNotConfigured = errors.New("trust not configured")
	// ErrSoulNotAvailable indicates a soul is not available to the current authenticated principal.
	ErrSoulNotAvailable = errors.New("soul not available")
	// ErrSoulAlreadyBound indicates the target soul is already incorporated elsewhere.
	ErrSoulAlreadyBound = errors.New("soul already bound")
	// ErrBodyAlreadyHasSoul indicates the target local body already has a soul.
	ErrBodyAlreadyHasSoul = errors.New("body already has soul")
)

const defaultSoulHTTPTimeout = 10 * time.Second

const (
	defaultSoulSearchPageLimit = 100
	maxSoulSearchPages         = 10
)

// Service resolves principal-bound souls for a Lesser instance and manages explicit incorporation bindings.
type Service struct {
	accountRepo  accountRepository
	instanceRepo instanceRepository
	cfg          *config.Config
	logger       *zap.Logger
	httpClient   *http.Client
}

// Soul represents a host-discovered soul plus local binding state.
type Soul struct {
	AgentID                string
	Domain                 string
	LocalID                string
	ENSName                *string
	Wallet                 string
	PrincipalAddress       string
	Status                 string
	LifecycleStatus        string
	SelfDescriptionVersion *int
	Capabilities           []string
	MintTxHash             string
	MintedAt               *time.Time
	UpdatedAt              *time.Time
	Bound                  bool
	BoundUsername          string
	BoundPrincipalAddress  string
	BoundAt                time.Time
	BoundUpdatedAt         time.Time
}

// NewService creates a new soul service.
func NewService(accountRepo accountRepository, instanceRepo instanceRepository, cfg *config.Config, logger *zap.Logger) *Service {
	return &Service{
		accountRepo:  accountRepo,
		instanceRepo: instanceRepo,
		cfg:          cfg,
		logger:       logger,
		httpClient:   &http.Client{Timeout: defaultSoulHTTPTimeout},
	}
}

// WithHTTPClient overrides the HTTP client used for lesser-host requests.
func (s *Service) WithHTTPClient(client *http.Client) *Service {
	if s == nil || client == nil {
		return s
	}
	s.httpClient = client
	return s
}

// ListMine returns the current user's discoverable souls for this instance.
func (s *Service) ListMine(ctx context.Context, username string) ([]Soul, error) {
	trustBaseURL, instanceDomain, ownerWallets, err := s.discoveryInputs(ctx, username)
	if err != nil {
		return nil, err
	}
	if len(ownerWallets) == 0 {
		return []Soul{}, nil
	}

	agentIDs, err := s.discoverOwnedSoulIDs(ctx, trustBaseURL, instanceDomain, ownerWallets)
	if err != nil {
		return nil, err
	}
	if len(agentIDs) == 0 {
		return []Soul{}, nil
	}

	results := make([]Soul, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		identity, err := s.fetchSoulIdentity(ctx, trustBaseURL, agentID)
		if err != nil {
			if errors.Is(err, ErrSoulNotAvailable) {
				continue
			}
			return nil, err
		}
		if !identityMatchesOwnerWallets(identity, ownerWallets) || !domainMatches(identity.Domain, instanceDomain) {
			continue
		}

		binding, err := s.instanceRepo.GetSoulBodyBinding(ctx, agentID)
		if err != nil {
			return nil, err
		}

		results = append(results, soulFromIdentity(identity, binding))
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].LocalID == results[j].LocalID {
			return results[i].AgentID < results[j].AgentID
		}
		return results[i].LocalID < results[j].LocalID
	})

	return results, nil
}

// Incorporate explicitly binds a soul to the authenticated local body.
func (s *Service) Incorporate(ctx context.Context, username string, agentID string) (*Soul, error) {
	trustBaseURL, instanceDomain, ownerWallets, err := s.discoveryInputs(ctx, username)
	if err != nil {
		return nil, err
	}

	normalizedAgentID, err := validateAgentID(agentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSoulNotAvailable, err)
	}

	identity, err := s.fetchSoulIdentity(ctx, trustBaseURL, normalizedAgentID)
	if err != nil {
		return nil, err
	}
	if !domainMatches(identity.Domain, instanceDomain) || !identityMatchesOwnerWallets(identity, ownerWallets) || !identityIsActive(identity) {
		return nil, ErrSoulNotAvailable
	}

	binding, err := s.instanceRepo.BindSoulBody(ctx, identity.AgentID, username, canonicalOwnerAddress(identity))
	if err != nil {
		switch {
		case errors.Is(err, storageRepos.ErrSoulBodyBindingAlreadyExists):
			return nil, ErrSoulAlreadyBound
		case errors.Is(err, storageRepos.ErrSoulBodyAlreadyHasBinding):
			return nil, ErrBodyAlreadyHasSoul
		default:
			return nil, err
		}
	}

	soul := soulFromIdentity(identity, binding)
	return &soul, nil
}

func (s *Service) discoveryInputs(ctx context.Context, username string) (string, string, map[string]struct{}, error) {
	if s == nil || s.accountRepo == nil || s.instanceRepo == nil || s.cfg == nil {
		return "", "", nil, fmt.Errorf("soul service misconfigured")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", nil, fmt.Errorf("username is required")
	}

	effectiveTrust, err := s.instanceRepo.EffectiveTrustConfig(ctx)
	if err != nil {
		return "", "", nil, err
	}
	trustBaseURL := strings.TrimRight(strings.TrimSpace(effectiveTrust.TrustBaseURL), "/")
	if trustBaseURL == "" {
		return "", "", nil, ErrTrustNotConfigured
	}

	instanceDomain := strings.ToLower(strings.TrimSpace(s.cfg.Domain))
	if instanceDomain == "" {
		return "", "", nil, fmt.Errorf("instance domain is required")
	}

	wallets, err := s.accountRepo.GetUserWalletCredentials(ctx, username)
	if err != nil {
		return "", "", nil, err
	}

	return trustBaseURL, instanceDomain, canonicalOwnerWallets(wallets), nil
}

func (s *Service) discoverOwnedSoulIDs(ctx context.Context, trustBaseURL string, instanceDomain string, ownerWallets map[string]struct{}) ([]string, error) {
	agentSet := make(map[string]struct{})
	for principal := range ownerWallets {
		results, err := s.searchSouls(ctx, trustBaseURL, instanceDomain, principal)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			normalizedAgentID, err := validateAgentID(result.AgentID)
			if err != nil {
				continue
			}
			agentSet[normalizedAgentID] = struct{}{}
		}
	}

	agentIDs := make([]string, 0, len(agentSet))
	for agentID := range agentSet {
		agentIDs = append(agentIDs, agentID)
	}
	sort.Strings(agentIDs)
	return agentIDs, nil
}

func (s *Service) searchSouls(ctx context.Context, trustBaseURL string, instanceDomain string, principal string) ([]hostSoulSearchResult, error) {
	results := make([]hostSoulSearchResult, 0)
	cursor := ""
	for page := 0; page < maxSoulSearchPages; page++ {
		payload, err := s.searchSoulsPage(ctx, trustBaseURL, instanceDomain, principal, cursor)
		if err != nil {
			return nil, err
		}
		results = append(results, payload.Results...)
		if !payload.HasMore || strings.TrimSpace(payload.NextCursor) == "" {
			return results, nil
		}
		cursor = strings.TrimSpace(payload.NextCursor)
	}
	return results, nil
}

func (s *Service) searchSoulsPage(ctx context.Context, trustBaseURL string, instanceDomain string, principal string, cursor string) (*hostSoulSearchResponse, error) {
	query := url.Values{}
	query.Set("domain", instanceDomain)
	query.Set("principal", principal)
	query.Set("limit", fmt.Sprintf("%d", defaultSoulSearchPageLimit))
	if strings.TrimSpace(cursor) != "" {
		query.Set("cursor", strings.TrimSpace(cursor))
	}

	endpoint := trustBaseURL + "/api/v1/soul/search?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search souls: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search souls failed: status %d", resp.StatusCode)
	}

	var payload hostSoulSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode soul search: %w", err)
	}

	return &payload, nil
}

func (s *Service) fetchSoulIdentity(ctx context.Context, trustBaseURL string, agentID string) (*hostSoulIdentity, error) {
	endpoint := trustBaseURL + "/api/v1/soul/agents/" + url.PathEscape(agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get soul agent: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrSoulNotAvailable
	default:
		return nil, fmt.Errorf("get soul agent failed: status %d", resp.StatusCode)
	}

	var payload hostSoulAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode soul agent: %w", err)
	}

	normalizedAgentID, err := validateAgentID(payload.Agent.AgentID)
	if err != nil {
		return nil, fmt.Errorf("decode soul agent: %w", err)
	}
	payload.Agent.AgentID = normalizedAgentID

	return &payload.Agent, nil
}

func canonicalOwnerWallets(wallets []*storage.WalletCredential) map[string]struct{} {
	out := make(map[string]struct{})
	for _, wallet := range wallets {
		if wallet == nil {
			continue
		}
		address := strings.ToLower(strings.TrimSpace(wallet.Address))
		if !ethcommon.IsHexAddress(address) {
			continue
		}
		out[address] = struct{}{}
	}
	return out
}

func validateAgentID(agentID string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(agentID))
	if normalized == "" {
		return "", errors.New("agent id is required")
	}
	if !strings.HasPrefix(normalized, "0x") || len(normalized) != 66 {
		return "", fmt.Errorf("invalid agent id %q", agentID)
	}
	if _, err := hexutil.Decode(normalized); err != nil {
		return "", fmt.Errorf("invalid agent id %q", agentID)
	}
	return normalized, nil
}

func identityMatchesOwnerWallets(identity *hostSoulIdentity, ownerWallets map[string]struct{}) bool {
	if identity == nil || len(ownerWallets) == 0 {
		return false
	}
	owner := canonicalOwnerAddress(identity)
	if owner == "" {
		return false
	}
	_, ok := ownerWallets[owner]
	return ok
}

func canonicalOwnerAddress(identity *hostSoulIdentity) string {
	if identity == nil {
		return ""
	}
	owner := strings.ToLower(strings.TrimSpace(identity.PrincipalAddress))
	if owner == "" {
		owner = strings.ToLower(strings.TrimSpace(identity.Wallet))
	}
	if !ethcommon.IsHexAddress(owner) {
		return ""
	}
	return owner
}

func identityIsActive(identity *hostSoulIdentity) bool {
	if identity == nil {
		return false
	}
	status := strings.TrimSpace(identity.LifecycleStatus)
	if status == "" {
		status = strings.TrimSpace(identity.Status)
	}
	return strings.EqualFold(status, "active")
}

func domainMatches(actual string, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected))
}

func soulFromIdentity(identity *hostSoulIdentity, binding *storageModels.InstanceSoulBodyBinding) Soul {
	soul := Soul{
		AgentID:                identity.AgentID,
		Domain:                 strings.TrimSpace(identity.Domain),
		LocalID:                strings.TrimSpace(identity.LocalID),
		ENSName:                normalizedOptionalString(identity.ENSName),
		Wallet:                 strings.ToLower(strings.TrimSpace(identity.Wallet)),
		PrincipalAddress:       strings.ToLower(strings.TrimSpace(identity.PrincipalAddress)),
		Status:                 strings.TrimSpace(identity.Status),
		LifecycleStatus:        strings.TrimSpace(identity.LifecycleStatus),
		SelfDescriptionVersion: identity.SelfDescriptionVersion,
		Capabilities:           append([]string(nil), identity.Capabilities...),
		MintTxHash:             strings.TrimSpace(identity.MintTxHash),
		MintedAt:               identity.MintedAt,
		UpdatedAt:              identity.UpdatedAt,
	}
	if binding != nil {
		soul.Bound = true
		soul.BoundUsername = binding.Username
		soul.BoundPrincipalAddress = binding.PrincipalAddress
		soul.BoundAt = binding.BoundAt
		soul.BoundUpdatedAt = binding.UpdatedAt
	}
	return soul
}

func normalizedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

type hostSoulSearchResponse struct {
	Results    []hostSoulSearchResult `json:"results"`
	HasMore    bool                   `json:"has_more"`
	NextCursor string                 `json:"next_cursor"`
}

type hostSoulSearchResult struct {
	AgentID string `json:"agent_id"`
}

type hostSoulAgentResponse struct {
	Agent hostSoulIdentity `json:"agent"`
}

type hostSoulIdentity struct {
	AgentID                string     `json:"agent_id"`
	Domain                 string     `json:"domain"`
	LocalID                string     `json:"local_id"`
	ENSName                *string    `json:"ens_name"`
	Wallet                 string     `json:"wallet"`
	PrincipalAddress       string     `json:"principal_address"`
	Status                 string     `json:"status"`
	LifecycleStatus        string     `json:"lifecycle_status"`
	SelfDescriptionVersion *int       `json:"self_description_version"`
	Capabilities           []string   `json:"capabilities"`
	MintTxHash             string     `json:"mint_tx_hash"`
	MintedAt               *time.Time `json:"minted_at"`
	UpdatedAt              *time.Time `json:"updated_at"`
}

type accountRepository interface {
	GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error)
}

type instanceRepository interface {
	EffectiveTrustConfig(ctx context.Context) (*storageModels.EffectiveTrustConfig, error)
	GetSoulBodyBinding(ctx context.Context, agentID string) (*storageModels.InstanceSoulBodyBinding, error)
	BindSoulBody(ctx context.Context, agentID string, username string, principalAddress string) (*storageModels.InstanceSoulBodyBinding, error)
}
