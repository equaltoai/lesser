package federation

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// RemoteSearchService handles remote actor discovery via WebFinger and ActivityPub
type RemoteSearchService struct {
	store      storage.Storage
	httpClient *httpclient.SecureClient
	logger     *zap.Logger
}

// NewRemoteSearchService creates a new remote search service
func NewRemoteSearchService(store storage.Storage) *RemoteSearchService {
	return &RemoteSearchService{
		store:      store,
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
	if domain == "" {
		// Local actor lookup
		actor, err := s.store.GetActor(ctx, username)
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
	cachedActor, err := s.store.GetCachedRemoteActor(ctx, cacheKey)
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
		return nil, fmt.Errorf("webfinger lookup failed: %w", err)
	}

	// Fetch the actor from their ActivityPub endpoint
	actor, err := s.fetchRemoteActor(ctx, actorURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote actor: %w", err)
	}

	// Cache the remote actor with 24 hour TTL
	if err := s.store.CacheRemoteActor(ctx, cacheKey, actor, 24*time.Hour); err != nil {
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

// webFingerLookup performs a WebFinger query to find an actor's ActivityPub URL
func (s *RemoteSearchService) webFingerLookup(ctx context.Context, username, domain string) (string, error) {
	// Build WebFinger URL
	resource := fmt.Sprintf("acct:%s@%s", username, domain)
	webfingerURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=%s",
		domain, url.QueryEscape(resource))

	s.logger.Debug("performing webfinger lookup",
		zap.String("url", webfingerURL))

	req, err := http.NewRequestWithContext(ctx, "GET", webfingerURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/jrd+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("webfinger request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webfinger returned status %d", resp.StatusCode)
	}

	// Parse WebFinger response
	var webfingerResp activitypub.WebFingerResource
	if err := common.ParseHTTPResponse(resp.Body, &webfingerResp); err != nil {
		return "", fmt.Errorf("failed to parse webfinger response: %w", err)
	}

	// Find the ActivityPub link
	for _, link := range webfingerResp.Links {
		if link.Rel == "self" && link.Type == "application/activity+json" {
			return link.Href, nil
		}
	}

	return "", fmt.Errorf("no ActivityPub link found in webfinger response")
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
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch actor: status %d", resp.StatusCode)
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, fmt.Errorf("failed to decode actor: %w", err)
	}

	// Validate required fields
	if actor.ID == "" || actor.Inbox == "" {
		return nil, fmt.Errorf("invalid actor: missing required fields")
	}

	return &actor, nil
}

// SearchRemoteActors searches for actors on remote instances
// This can be extended to query remote instance search endpoints
func (s *RemoteSearchService) SearchRemoteActors(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	// For now, only handle exact @user@domain matches
	if !isValidHandle(query) {
		return nil, nil
	}

	result, err := s.ResolveActor(ctx, query)
	if err != nil {
		s.logger.Debug("failed to resolve actor",
			zap.String("query", query),
			zap.Error(err))
		return nil, nil
	}

	return []*SearchResult{result}, nil
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
			return "", "", fmt.Errorf("invalid username: %w", err)
		}
		return username, "", nil
	} else if len(parts) == 2 {
		// Remote user - validate both username and domain
		username = parts[0]
		domain = parts[1]

		if err := activitypub.ValidateUsername(username); err != nil {
			return "", "", fmt.Errorf("invalid username: %w", err)
		}

		if err := activitypub.ValidateDomain(domain); err != nil {
			return "", "", fmt.Errorf("invalid domain: %w", err)
		}

		return username, domain, nil
	}

	return "", "", fmt.Errorf("invalid handle format: %s", handle)
}

// isValidHandle checks if a query looks like a federated handle
func isValidHandle(query string) bool {
	// Remove leading @ if present
	query = strings.TrimPrefix(query, "@")

	// Should have exactly one @ for user@domain
	return strings.Count(query, "@") == 1
}
