package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// RemoteSearchService handles remote actor discovery via WebFinger and ActivityPub
type RemoteSearchService struct {
	actorRepo  remoteSearchActorRepository
	userRepo   remoteSearchUserRepository
	httpClient httpDoer
	logger     *zap.Logger
}

type remoteSearchActorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)
}

type remoteSearchUserRepository interface {
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
}

// NewRemoteSearchService creates a new remote search service
func NewRemoteSearchService(store core.RepositoryStorage) *RemoteSearchService {
	var actorRepo remoteSearchActorRepository
	var userRepo remoteSearchUserRepository
	if store != nil {
		actorRepo = store.Actor()
		userRepo = store.User()
	}

	return &RemoteSearchService{
		actorRepo:  actorRepo,
		userRepo:   userRepo,
		httpClient: httpclient.NewSecureClient(httpclient.WithTimeout(30 * time.Second)),
		logger:     common.Logger(),
	}
}

// SearchResult represents a remote search result
type SearchResult struct {
	Actor        *activitypub.Actor
	IsRemote     bool
	RemoteDomain string
}

// ActorIdentity captures the stable, reversible identity fields callers need
// to preserve local and remote actors across product flows.
type ActorIdentity struct {
	ActorID  string
	Username string
	Acct     string
	Domain   string
	IsRemote bool
}

// ExactActorResolution is the canonical exact actor lookup result shared by
// product-facing callers.
type ExactActorResolution struct {
	Actor *activitypub.Actor
	ActorIdentity
}

// ResolveActorURL resolves an ActivityPub actor URL to an Actor object.
// It checks cache using an inferred handle when possible, then fetches the actor document and caches it.
func (s *RemoteSearchService) ResolveActorURL(ctx context.Context, actorURL string) (*SearchResult, error) {
	actorURL = strings.TrimSpace(actorURL)
	if err := activitypub.ValidateURL(actorURL, "actor_url"); err != nil {
		return nil, err
	}

	parsed, err := url.Parse(actorURL)
	if err != nil {
		return nil, err
	}

	domain := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if domain == "" {
		return nil, ErrInvalidDomainFormat
	}

	// Attempt cache hit using a username inferred from the actor URL path.
	if username := usernameFromActorPath(parsed.Path); username != "" {
		cacheKey := fmt.Sprintf("%s@%s", username, domain)
		cachedActor, err := s.getCachedRemoteActor(ctx, cacheKey)
		if err == nil && cachedActor != nil {
			return &SearchResult{
				Actor:        cachedActor,
				IsRemote:     true,
				RemoteDomain: domain,
			}, nil
		}
	}

	actor, err := s.fetchRemoteActor(ctx, actorURL)
	if err != nil {
		return nil, err
	}

	// Cache using actor-preferred username when possible.
	cacheUser := strings.TrimSpace(actor.PreferredUsername)
	if cacheUser == "" {
		cacheUser = usernameFromActorPath(actor.ID)
	}
	if cacheUser != "" {
		cacheKey := fmt.Sprintf("%s@%s", cacheUser, domain)
		if err := s.cacheRemoteActor(ctx, cacheKey, actor, 24*time.Hour); err != nil {
			s.logger.Warn("failed to cache remote actor resolved by URL",
				zap.String("handle", cacheKey),
				zap.String("actor_url", actorURL),
				zap.Error(err))
		}
	}

	return &SearchResult{
		Actor:        actor,
		IsRemote:     true,
		RemoteDomain: domain,
	}, nil
}

// ResolveActor resolves an actor handle (user@domain) to an Actor object
// It first checks local cache, then performs WebFinger lookup if needed
func (s *RemoteSearchService) ResolveActor(ctx context.Context, handle string) (*SearchResult, error) {
	username, domain, err := parseHandle(handle)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("resolving actor",
		zap.String("username", username),
		zap.String("domain", domain))

	// Check if it's a local actor (no domain or our domain)
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		// Local actor lookup
		actor, err := s.getLocalActor(ctx, username)
		if err != nil {
			return nil, err
		}
		return &SearchResult{
			Actor:    actor,
			IsRemote: false,
		}, nil
	}

	// For remote actors, check cache first
	cacheKey := fmt.Sprintf("%s@%s", username, domain)
	cachedActor, err := s.getCachedRemoteActor(ctx, cacheKey)
	if err == nil && cachedActor != nil {
		s.logger.Debug("found actor in cache",
			zap.String("handle", cacheKey))
		return &SearchResult{
			Actor:        cachedActor,
			IsRemote:     true,
			RemoteDomain: domain,
		}, nil
	}

	// Not in cache, perform WebFinger lookup
	actorURL, err := s.webFingerLookup(ctx, username, domain)
	if err != nil {
		s.logger.Error("webfinger lookup failed",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return nil, errors.Join(ErrWebFingerLookupFailed, err)
	}

	// Fetch the actor from their ActivityPub endpoint
	actor, err := s.fetchRemoteActor(ctx, actorURL)
	if err != nil {
		s.logger.Error("failed to fetch remote actor",
			zap.String("actor_url", actorURL),
			zap.Error(err))
		return nil, errors.Join(ErrFetchRemoteActorFailed, err)
	}
	if err := validateResolvedActorDomainForHandle(actor, domain); err != nil {
		s.logger.Error("actor domain mismatch for resolved handle — possible spoofing",
			zap.String("handle", cacheKey),
			zap.String("actor_url", actorURL),
			zap.String("actor_id", strings.TrimSpace(actor.ID)),
			zap.Error(err))
		return nil, errors.Join(ErrActorDomainMismatch, err)
	}

	// Cache the remote actor with 24 hour TTL
	if err := s.cacheRemoteActor(ctx, cacheKey, actor, 24*time.Hour); err != nil {
		s.logger.Error("failed to cache remote actor",
			zap.String("handle", cacheKey),
			zap.Error(err))
		// Don't fail if caching fails
	}

	return &SearchResult{
		Actor:        actor,
		IsRemote:     true,
		RemoteDomain: domain,
	}, nil
}

// ResolveExactActor resolves local usernames, local handles, local actor URLs,
// remote handles, and remote actor URLs through one canonical seam.
func (s *RemoteSearchService) ResolveExactActor(ctx context.Context, input, localDomain string) (*ExactActorResolution, error) {
	input = strings.TrimSpace(input)
	if err := common.ValidateRequiredParam("actor_lookup", input); err != nil {
		return nil, err
	}

	localDomain = normalizeActorDomain(localDomain)
	result, err := s.resolveExactSearchResult(ctx, input, localDomain)
	if err != nil {
		return nil, err
	}

	if result == nil || result.Actor == nil {
		return nil, common.ActorNotFoundError{Username: input}
	}

	return buildExactActorResolution(result.Actor, localDomain, result.IsRemote, result.RemoteDomain), nil
}

// ResolveDeliverableActor resolves an actor through the exact-resolution seam
// and verifies the result satisfies Lesser's minimum deliverable actor contract.
func (s *RemoteSearchService) ResolveDeliverableActor(ctx context.Context, input, localDomain string) (*ExactActorResolution, error) {
	resolution, err := s.ResolveExactActor(ctx, input, localDomain)
	if err != nil {
		return nil, err
	}
	if resolution == nil || resolution.Actor == nil {
		return nil, common.ActorNotFoundError{Username: input}
	}
	if err := activitypub.ValidateResolvedActor(resolution.Actor); err != nil {
		return nil, err
	}

	return resolution, nil
}

func (s *RemoteSearchService) resolveExactSearchResult(ctx context.Context, input, localDomain string) (*SearchResult, error) {
	switch {
	case isActorURL(input):
		return s.resolveExactActorURL(ctx, input, localDomain)
	case strings.Contains(strings.TrimPrefix(input, "@"), "@"):
		return s.resolveExactActorHandle(ctx, input, localDomain)
	default:
		return s.ResolveActor(ctx, input)
	}
}

func (s *RemoteSearchService) resolveExactActorURL(ctx context.Context, actorURL, localDomain string) (*SearchResult, error) {
	parsed, err := url.Parse(actorURL)
	if err != nil {
		return nil, err
	}

	host := normalizeActorDomain(parsed.Hostname())
	if host == "" {
		return nil, ErrInvalidDomainFormat
	}

	if localDomain != "" && host == localDomain {
		return s.resolveExactLocalActor(ctx, usernameFromActorPath(parsed.Path), actorURL)
	}

	return s.ResolveActorURL(ctx, actorURL)
}

func (s *RemoteSearchService) resolveExactActorHandle(ctx context.Context, handle, localDomain string) (*SearchResult, error) {
	username, domain, err := parseHandle(handle)
	if err != nil {
		return nil, err
	}

	if localDomain != "" && domain == localDomain {
		return s.resolveExactLocalActor(ctx, username, handle)
	}

	return s.ResolveActor(ctx, exactHandle(username, domain))
}

func (s *RemoteSearchService) resolveExactLocalActor(ctx context.Context, username, input string) (*SearchResult, error) {
	if username == "" {
		return nil, common.ActorNotFoundError{Username: input}
	}

	actor, err := s.getLocalActor(ctx, username)
	if err != nil {
		return nil, err
	}

	return &SearchResult{Actor: actor}, nil
}

func exactHandle(username, domain string) string {
	if domain == "" {
		return username
	}
	return fmt.Sprintf("%s@%s", username, domain)
}

// webFingerLookup performs a WebFinger query to find an actor's ActivityPub URL
func (s *RemoteSearchService) webFingerLookup(ctx context.Context, username, domain string) (string, error) {
	return webFingerLookupWithClient(ctx, s.httpClient, s.logger, username, domain)
}

func webFingerLookupWithClient(ctx context.Context, client httpDoer, logger *zap.Logger, username, domain string) (string, error) {
	// Build WebFinger URL
	resource := fmt.Sprintf("acct:%s@%s", username, domain)
	webfingerURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=%s",
		domain, url.QueryEscape(resource))

	logger.Debug("performing webfinger lookup",
		zap.String("url", webfingerURL))

	req, err := http.NewRequestWithContext(ctx, "GET", webfingerURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/jrd+json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("webfinger request failed",
			zap.String("url", webfingerURL),
			zap.Error(err))
		return "", errors.Join(ErrWebFingerRequestFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		logger.Error("webfinger returned non-2xx status",
			zap.String("url", webfingerURL),
			zap.Int("status_code", resp.StatusCode))
		return "", ErrWebFingerNon2xxStatus
	}

	// Parse WebFinger response
	var webfingerResp activitypub.WebFingerResource
	if err := common.ParseHTTPResponse(resp.Body, &webfingerResp); err != nil {
		logger.Error("failed to parse webfinger response",
			zap.String("url", webfingerURL),
			zap.Error(err))
		return "", errors.Join(ErrWebFingerResponseParseFailed, err)
	}

	// Find the ActivityPub link
	for _, link := range webfingerResp.Links {
		if link.Rel == "self" && link.Type == "application/activity+json" {
			return link.Href, nil
		}
	}

	logger.Error("no ActivityPub link found in webfinger response",
		zap.String("url", webfingerURL))
	return "", ErrNoActivityPubLinkFound
}

// fetchRemoteActor fetches an actor from their ActivityPub endpoint
func (s *RemoteSearchService) fetchRemoteActor(ctx context.Context, actorURL string) (*activitypub.Actor, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", actorURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/activity+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("failed to fetch actor",
			zap.String("actor_url", actorURL),
			zap.Error(err))
		return nil, errors.Join(ErrFetchActorHTTPFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("failed to fetch actor: non-2xx status",
			zap.String("actor_url", actorURL),
			zap.Int("status_code", resp.StatusCode))
		return nil, ErrRemoteActorNon2xxStatus
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		s.logger.Error("failed to decode actor",
			zap.String("actor_url", actorURL),
			zap.Error(err))
		return nil, errors.Join(ErrRemoteActorDecodeFailed, err)
	}

	// Validate required fields
	if actor.ID == "" || actor.Inbox == "" {
		s.logger.Error("invalid actor: missing required fields",
			zap.String("actor_url", actorURL),
			zap.String("actor_id", actor.ID),
			zap.String("actor_inbox", actor.Inbox))
		return nil, ErrInvalidActorMissingFields
	}

	// Validate domain consistency: the actor document's declared ID must
	// belong to the same domain as the URL it was fetched from. This prevents
	// a malicious server at evil.example from returning an actor claiming to
	// be from victim.example.
	if err := common.ValidateActorDomainConsistency(actorURL, actor.ID); err != nil {
		s.logger.Error("actor domain mismatch — possible spoofing",
			zap.String("fetch_url", actorURL),
			zap.String("declared_id", actor.ID),
			zap.Error(err))
		return nil, errors.Join(ErrActorDomainMismatch, err)
	}

	return &actor, nil
}

// SearchRemoteActors searches for actors on remote instances
// Supports both exact @user@domain matches and fuzzy search on known instances
func (s *RemoteSearchService) SearchRemoteActors(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	var results []*SearchResult

	// Try exact actor URL or exact handle resolution first.
	switch {
	case isActorURL(query):
		result, err := s.ResolveActorURL(ctx, query)
		if err != nil {
			s.logger.Debug("failed to resolve exact actor URL",
				zap.String("query", query),
				zap.Error(err))
		} else if result != nil {
			results = append(results, result)
		}
	case isValidHandle(query):
		result, err := s.ResolveActor(ctx, query)
		if err != nil {
			s.logger.Debug("failed to resolve exact actor",
				zap.String("query", query),
				zap.Error(err))
		} else if result != nil {
			results = append(results, result)
		}
	}

	// If we have results from exact match or limit is reached, return
	if len(results) > 0 || limit <= 0 {
		return results, nil
	}

	// Try fuzzy search on known remote instances
	remoteResults, err := s.searchKnownInstances(ctx, query, limit)
	if err != nil {
		s.logger.Debug("failed to search known instances",
			zap.String("query", query),
			zap.Error(err))
		// Continue with what we have
	} else {
		results = append(results, remoteResults...)
	}

	// Apply limit if specified
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func usernameFromActorPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, err := url.Parse(value)
	if err == nil && parsed.Path != "" {
		value = parsed.Path
	}

	path := strings.Trim(value, "/")
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	candidate := strings.TrimPrefix(parts[len(parts)-1], "@")
	if err := activitypub.ValidateUsername(candidate); err != nil {
		return ""
	}
	return candidate
}

// parseHandle parses a handle in the format @user@domain or user@domain
func parseHandle(handle string) (username, domain string, err error) {
	// Remove leading @ if present
	handle = strings.TrimPrefix(handle, "@")

	// Split by @
	parts := strings.Split(handle, "@")

	if len(parts) == 1 {
		// Local user - validate username
		username = parts[0]
		if err := activitypub.ValidateUsername(username); err != nil {
			return "", "", errors.Join(ErrInvalidUsernameFormat, err)
		}
		return username, "", nil
	} else if len(parts) == 2 {
		// Remote user - validate both username and domain
		username = parts[0]
		domain = normalizeActorDomain(parts[1])

		if err := activitypub.ValidateUsername(username); err != nil {
			return "", "", errors.Join(ErrInvalidUsernameFormat, err)
		}

		if err := activitypub.ValidateDomain(domain); err != nil {
			return "", "", errors.Join(ErrInvalidDomainFormat, err)
		}

		return username, domain, nil
	}

	return "", "", ErrInvalidHandleFormat
}

// isValidHandle checks if a query looks like a federated handle
func isValidHandle(query string) bool {
	// Remove leading @ if present
	query = strings.TrimPrefix(query, "@")

	// Should have exactly one @ for user@domain
	return strings.Count(query, "@") == 1
}

func isActorURL(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func (s *RemoteSearchService) getLocalActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	if s.actorRepo == nil {
		return nil, common.ActorNotFoundError{Username: username}
	}

	actor, err := s.actorRepo.GetActorByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, common.ActorNotFoundError{Username: username}
	}

	return actor, nil
}

func (s *RemoteSearchService) getCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	if s.actorRepo == nil {
		return nil, common.ActorNotFoundError{Username: handle}
	}

	actor, err := s.actorRepo.GetCachedRemoteActor(ctx, handle)
	if err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, common.ActorNotFoundError{Username: cachedRemoteActorUsername(handle)}
	}
	if err := activitypub.ValidateResolvedActor(actor); err != nil {
		s.logger.Warn("ignoring invalid cached remote actor",
			zap.String("handle", handle),
			zap.String("actor_id", strings.TrimSpace(actor.ID)),
			zap.Error(err))
		return nil, common.ActorNotFoundError{Username: cachedRemoteActorUsername(handle)}
	}
	if !cachedRemoteActorMatchesHandle(actor, handle) {
		identity := DescribeActorIdentity(actor, "")
		s.logger.Warn("ignoring cached remote actor identity mismatch",
			zap.String("handle", handle),
			zap.String("actor_id", strings.TrimSpace(actor.ID)),
			zap.String("cached_username", identity.Username),
			zap.String("cached_domain", identity.Domain))
		return nil, common.ActorNotFoundError{Username: cachedRemoteActorUsername(handle)}
	}

	return actor, nil
}

func (s *RemoteSearchService) cacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	if s.userRepo == nil {
		return nil
	}
	return s.userRepo.CacheRemoteActor(ctx, handle, actor, ttl)
}

func buildExactActorResolution(actor *activitypub.Actor, localDomain string, isRemote bool, remoteDomain string) *ExactActorResolution {
	identity := DescribeActorIdentity(actor, localDomain)
	if normalizedRemoteDomain := normalizeActorDomain(remoteDomain); normalizedRemoteDomain != "" {
		identity.Domain = normalizedRemoteDomain
		identity.IsRemote = normalizeActorDomain(localDomain) == "" || identity.Domain != normalizeActorDomain(localDomain)
	}
	if isRemote {
		identity.IsRemote = true
	}
	if identity.IsRemote && identity.Domain != "" && identity.Username != "" {
		identity.Acct = fmt.Sprintf("%s@%s", identity.Username, identity.Domain)
	}

	return &ExactActorResolution{
		Actor:         actor,
		ActorIdentity: identity,
	}
}

// DescribeActorIdentity derives the canonical local or remote identity contract
// callers can use without guessing the actor domain back later.
func DescribeActorIdentity(actor *activitypub.Actor, localDomain string) ActorIdentity {
	if actor == nil {
		return ActorIdentity{}
	}

	localDomain = normalizeActorDomain(localDomain)
	actorID := strings.TrimSpace(actor.ID)
	domain := deriveActorDomain(actor)
	username := deriveActorUsername(actor)
	isRemote := false

	if domain != "" {
		isRemote = localDomain == "" || domain != localDomain
	}

	acct := username
	if isRemote && domain != "" && username != "" {
		acct = fmt.Sprintf("%s@%s", username, domain)
	}

	return ActorIdentity{
		ActorID:  actorID,
		Username: username,
		Acct:     acct,
		Domain:   domain,
		IsRemote: isRemote,
	}
}

func deriveActorDomain(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}

	candidates := []string{
		strings.TrimSpace(actor.ID),
		strings.TrimSpace(actor.URL),
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		parsed, err := url.Parse(candidate)
		if err == nil {
			if domain := normalizeActorDomain(parsed.Hostname()); domain != "" {
				return domain
			}
		}
	}

	_, domain, err := parseLooseHandle(actor.PreferredUsername)
	if err == nil {
		return domain
	}

	return ""
}

func deriveActorUsername(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}

	candidates := []string{
		strings.TrimSpace(actor.PreferredUsername),
		usernameFromActorPath(actor.ID),
		usernameFromActorPath(actor.URL),
	}

	for _, candidate := range candidates {
		username, _, err := parseLooseHandle(candidate)
		if err == nil && username != "" {
			return username
		}
	}

	return ""
}

func parseLooseHandle(value string) (username string, domain string, err error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	if value == "" {
		return "", "", ErrInvalidHandleFormat
	}

	parts := strings.Split(value, "@")
	switch len(parts) {
	case 1:
		return strings.TrimSpace(parts[0]), "", nil
	case 2:
		domain := normalizeActorDomain(parts[1])
		if domain == "" {
			return "", "", ErrInvalidDomainFormat
		}
		return strings.TrimSpace(parts[0]), domain, nil
	default:
		return "", "", ErrInvalidHandleFormat
	}
}

func normalizeActorDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://")
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}

	host := actorDomainHost(value)
	if idx := strings.Index(host, "%"); idx >= 0 {
		zoneHost := host[:idx]
		if ip, err := netip.ParseAddr(zoneHost); err != nil || !ip.Is6() {
			return ""
		}
		host = zoneHost
	}
	if ip, ok := parseActorDomainIP(host); ok {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return ""
		}
		return ip.String()
	}
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return strings.TrimSpace(host)
}

func actorDomainHost(value string) string {
	if strings.HasPrefix(value, "[") {
		closing := strings.Index(value, "]")
		if closing < 0 {
			return ""
		}
		suffix := value[closing+1:]
		if suffix != "" && !strings.HasPrefix(suffix, ":") {
			return ""
		}
		return value[1:closing]
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

func parseActorDomainIP(value string) (netip.Addr, bool) {
	if ip, err := netip.ParseAddr(value); err == nil {
		return ip.Unmap(), true
	}
	return parseLegacyIPv4(value)
}

// parseLegacyIPv4 recognizes the historical inet_aton numeric forms as one
// family so alternate spellings cannot bypass IP classification or dedup.
func parseLegacyIPv4(value string) (netip.Addr, bool) {
	parts := strings.Split(value, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}

	values := make([]uint64, len(parts))
	for i, part := range parts {
		if part == "" {
			return netip.Addr{}, false
		}
		parsed, err := strconv.ParseUint(part, 0, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		values[i] = parsed
	}

	var raw uint64
	switch len(values) {
	case 1:
		raw = values[0]
	case 2:
		if values[0] > 0xff || values[1] > 0xffffff {
			return netip.Addr{}, false
		}
		raw = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xff || values[1] > 0xff || values[2] > 0xffff {
			return netip.Addr{}, false
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]
	case 4:
		for _, value := range values {
			if value > 0xff {
				return netip.Addr{}, false
			}
		}
		raw = values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
	}

	return netip.AddrFrom4([4]byte{byte(raw >> 24), byte(raw >> 16), byte(raw >> 8), byte(raw)}), true
}

func cachedRemoteActorMatchesHandle(actor *activitypub.Actor, handle string) bool {
	username, domain, err := parseHandle(handle)
	if err != nil || domain == "" {
		return false
	}

	identity := DescribeActorIdentity(actor, "")
	return strings.EqualFold(exactHandle(identity.Username, identity.Domain), exactHandle(username, domain))
}

func validateResolvedActorDomainForHandle(actor *activitypub.Actor, expectedDomain string) error {
	expectedDomain = normalizeActorDomain(expectedDomain)
	if actor == nil || expectedDomain == "" {
		return ErrActorDomainMismatch
	}

	actorDomain := normalizeActorDomain(common.ExtractDomainFromActorID(actor.ID))
	if actorDomain == "" {
		return ErrActorDomainMismatch
	}
	if !strings.EqualFold(actorDomain, expectedDomain) {
		return fmt.Errorf("resolved actor ID domain %q does not match requested handle domain %q", actorDomain, expectedDomain)
	}
	return nil
}

func cachedRemoteActorUsername(handle string) string {
	username, _, err := parseLooseHandle(handle)
	if err == nil && username != "" {
		return username
	}
	return strings.TrimSpace(strings.TrimPrefix(handle, "@"))
}

// searchKnownInstances performs fuzzy search on known remote instances
func (s *RemoteSearchService) searchKnownInstances(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	// Get list of known instances from storage
	if s.httpClient == nil {
		return nil, nil // No storage available
	}

	// Get active instances (this would come from federation stats/health)
	knownInstances, err := s.getKnownActiveInstances(ctx)
	if err != nil {
		s.logger.Error("failed to get known instances", zap.Error(err))
		return nil, errors.Join(ErrGetKnownInstancesFailed, err)
	}

	var results []*SearchResult
	searchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Search each instance concurrently (up to a reasonable limit)
	maxConcurrent := 5
	if len(knownInstances) < maxConcurrent {
		maxConcurrent = len(knownInstances)
	}

	type instanceResult struct {
		results []*SearchResult
		err     error
	}

	resultsChan := make(chan instanceResult, maxConcurrent)
	semaphore := make(chan struct{}, maxConcurrent)

	// Launch concurrent searches
	searchCount := 0
	for _, instance := range knownInstances {
		if searchCount >= maxConcurrent {
			break
		}

		searchCount++
		go func(instanceDomain string) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			instanceResults, err := s.searchRemoteInstance(searchCtx, instanceDomain, query)
			resultsChan <- instanceResult{results: instanceResults, err: err}
		}(instance)
	}

	// Collect results
	for i := 0; i < searchCount; i++ {
		select {
		case result := <-resultsChan:
			if result.err != nil {
				s.logger.Debug("instance search failed", zap.Error(result.err))
				continue
			}
			results = append(results, result.results...)

		case <-searchCtx.Done():
			s.logger.Debug("remote search timeout")
			goto searchDone
		}

		// Stop if we have enough results
		if limit > 0 && len(results) >= limit {
			break
		}
	}
searchDone:

	return results, nil
}

// getKnownActiveInstances returns a list of known active federated instances
func (s *RemoteSearchService) getKnownActiveInstances(_ context.Context) ([]string, error) {
	// This would typically query federation metrics or instance health data
	// For now, return a basic set of well-known instances
	return []string{
		"mastodon.social",
		"mas.to",
		"pixelfed.social",
		"lemmy.ml",
	}, nil
}

// searchRemoteInstance searches a specific remote instance for actors
func (s *RemoteSearchService) searchRemoteInstance(ctx context.Context, domain, query string) ([]*SearchResult, error) {
	// Construct search URL for the remote instance
	// Different instances may have different search endpoints
	searchURL := fmt.Sprintf("https://%s/api/v2/search?q=%s&type=accounts&limit=5",
		domain, url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, errors.Join(ErrCreateSearchRequestFailed, err)
	}

	req.Header.Set("User-Agent", "Lesser/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.Join(ErrSearchRequestFailed, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrSearchNon2xxStatus
	}

	// Parse response
	var searchResp struct {
		Accounts []struct {
			Username    string `json:"username"`
			Acct        string `json:"acct"`
			DisplayName string `json:"display_name"`
			Avatar      string `json:"avatar"`
			Note        string `json:"note"`
			URL         string `json:"url"`
		} `json:"accounts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, errors.Join(ErrSearchResponseDecodeFailed, err)
	}

	// Convert to our result format
	var results []*SearchResult
	for _, account := range searchResp.Accounts {
		// Create a minimal Actor object from the search result
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   account.URL,
				Type: "Person",
			},
			Name:              account.DisplayName,
			PreferredUsername: account.Username,
			Icon: &activitypub.Image{
				BaseObject: activitypub.BaseObject{
					Type: "Image",
				},
				URL: account.Avatar,
			},
			Summary: account.Note,
			URL:     account.URL,
			Inbox:   fmt.Sprintf("https://%s/users/%s/inbox", domain, account.Username),
			Outbox:  fmt.Sprintf("https://%s/users/%s/outbox", domain, account.Username),
		}

		results = append(results, &SearchResult{
			Actor:        actor,
			IsRemote:     true,
			RemoteDomain: domain,
		})
	}

	return results, nil
}
