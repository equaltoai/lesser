package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	store      core.RepositoryStorage
	httpClient *httpclient.SecureClient
	logger     *zap.Logger
}

// NewRemoteSearchService creates a new remote search service
func NewRemoteSearchService(store core.RepositoryStorage) *RemoteSearchService {
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
		actor, err := s.store.Actor().GetActorByUsername(ctx, username)
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
	cachedActor, err := s.store.Actor().GetCachedRemoteActor(ctx, cacheKey)
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
	if err := s.store.User().CacheRemoteActor(ctx, cacheKey, actor, 24*time.Hour); err != nil {
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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
// Supports both exact @user@domain matches and fuzzy search on known instances
func (s *RemoteSearchService) SearchRemoteActors(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	var results []*SearchResult

	// Try exact handle resolution first
	if isValidHandle(query) {
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

// searchKnownInstances performs fuzzy search on known remote instances
func (s *RemoteSearchService) searchKnownInstances(ctx context.Context, query string, limit int) ([]*SearchResult, error) {
	// Get list of known instances from storage
	if s.store == nil {
		return nil, nil // No storage available
	}

	// Get active instances (this would come from federation stats/health)
	knownInstances, err := s.getKnownActiveInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get known instances: %w", err)
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
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Lesser/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
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
		return nil, fmt.Errorf("failed to decode search response: %w", err)
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
