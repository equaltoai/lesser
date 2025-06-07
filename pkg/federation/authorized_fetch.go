package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"go.uber.org/zap"
)

// AuthorizedFetchService handles authorized fetch for ActivityPub objects
type AuthorizedFetchService struct {
	store      storage.Storage
	logger     *zap.Logger
	httpClient *http.Client
	domain     string
}

// NewAuthorizedFetchService creates a new authorized fetch service
func NewAuthorizedFetchService(store storage.Storage, domain string, logger *zap.Logger) *AuthorizedFetchService {
	return &AuthorizedFetchService{
		store:  store,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		domain: domain,
	}
}

// FetchObject fetches an ActivityPub object with authorization
func (f *AuthorizedFetchService) FetchObject(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (interface{}, error) {
	f.logger.Debug("fetching object with authorization",
		zap.String("object_url", objectURL),
		zap.String("signing_actor", signingActor.ID))

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Get the actor's private key
	privateKeyPEM, err := f.store.GetActorPrivateKey(ctx, signingActor.PreferredUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	// Parse the private key
	privateKey, err := ParsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Sign the request
	if err := SignHTTPRequest(req, privateKey, signingActor.PublicKey.ID); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Send the request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch object: status %d", resp.StatusCode)
	}

	// Decode the response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate the object
	if err := f.validateObject(result, objectURL); err != nil {
		return nil, fmt.Errorf("object validation failed: %w", err)
	}

	return result, nil
}

// FetchActor fetches an ActivityPub actor with authorization
func (f *AuthorizedFetchService) FetchActor(ctx context.Context, actorURL string, signingActor *activitypub.Actor) (*activitypub.Actor, error) {
	f.logger.Debug("fetching actor with authorization",
		zap.String("actor_url", actorURL),
		zap.String("signing_actor", signingActor.ID))

	// Fetch the object
	obj, err := f.FetchObject(ctx, actorURL, signingActor)
	if err != nil {
		return nil, err
	}

	// Convert to Actor
	objMap, ok := obj.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid actor object type")
	}

	// Marshal to JSON then unmarshal to Actor
	data, err := json.Marshal(objMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal actor data: %w", err)
	}

	var actor activitypub.Actor
	if err := json.Unmarshal(data, &actor); err != nil {
		return nil, fmt.Errorf("failed to unmarshal actor: %w", err)
	}

	// Validate actor type
	switch actor.Type {
	case activitypub.PersonType, activitypub.ServiceType, activitypub.GroupType,
		activitypub.OrganizationType, activitypub.ApplicationType:
		// Valid actor types
	default:
		return nil, fmt.Errorf("not an actor object: type %s", actor.Type)
	}

	return &actor, nil
}

// VerifyAuthorizedFetch verifies an incoming authorized fetch request
func (f *AuthorizedFetchService) VerifyAuthorizedFetch(ctx context.Context, req *http.Request) (*activitypub.Actor, error) {
	f.logger.Debug("verifying authorized fetch request",
		zap.String("path", req.URL.Path),
		zap.String("method", req.Method))

	// Extract signature from headers
	signature := req.Header.Get("Signature")
	if signature == "" {
		return nil, fmt.Errorf("missing signature header")
	}

	// Parse the signature
	sig, err := ParseHTTPSignature(signature)
	if err != nil {
		return nil, fmt.Errorf("failed to parse signature: %w", err)
	}

	// Extract actor ID from keyId
	actorID := extractActorIDFromKeyID(sig.KeyID)
	if actorID == "" {
		return nil, fmt.Errorf("failed to extract actor ID from keyId: %s", sig.KeyID)
	}

	// Fetch the actor to get their public key
	// Note: This creates a chicken-and-egg problem in strict authorized fetch mode
	// In practice, you might need to allow unsigned fetches for actor documents
	// or maintain a cache of known actors
	actor, err := f.fetchActorWithoutAuth(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch actor: %w", err)
	}

	// Verify the signature
	if err := VerifyHTTPSignature(req, actor.PublicKey.PublicKeyPem, sig); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	return actor, nil
}

// IsAuthorizedFetchEnabled checks if authorized fetch is enabled
func (f *AuthorizedFetchService) IsAuthorizedFetchEnabled(ctx context.Context) bool {
	// Check instance configuration
	// This could be stored in DynamoDB or environment variable
	// For now, return false by default
	return false
}

// Private methods

func (f *AuthorizedFetchService) validateObject(obj map[string]interface{}, expectedID string) error {
	// Verify the object ID matches what we requested
	id, ok := obj["id"].(string)
	if !ok || id != expectedID {
		return fmt.Errorf("object ID mismatch: expected %s, got %s", expectedID, id)
	}

	// Verify the object has a type
	if _, ok := obj["type"].(string); !ok {
		return fmt.Errorf("object missing type field")
	}

	return nil
}

func (f *AuthorizedFetchService) fetchActorWithoutAuth(ctx context.Context, actorURL string) (*activitypub.Actor, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Send request without signature
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch actor: status %d", resp.StatusCode)
	}

	var actor activitypub.Actor
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
		return nil, err
	}

	return &actor, nil
}

// extractActorIDFromKeyID extracts the actor ID from a key ID
// e.g., "https://example.com/users/alice#main-key" -> "https://example.com/users/alice"
func extractActorIDFromKeyID(keyID string) string {
	// Remove fragment
	if idx := len(keyID) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if keyID[i] == '#' {
				return keyID[:i]
			}
		}
	}
	return keyID
}
