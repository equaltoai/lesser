// Package federation provides ActivityPub federation services including authorized fetch and object retrieval.
package federation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// AuthorizedFetchService handles authorized fetch for ActivityPub objects
type AuthorizedFetchService struct {
	store      core.RepositoryStorage
	logger     *zap.Logger
	httpClient *httpclient.SecureClient
	domain     string
}

// NewAuthorizedFetchService creates a new authorized fetch service
func NewAuthorizedFetchService(store core.RepositoryStorage, domain string, logger *zap.Logger) *AuthorizedFetchService {
	return &AuthorizedFetchService{
		store:  store,
		logger: logger,
		httpClient: httpclient.NewSecureClient(
			httpclient.WithTimeout(10*time.Second),
			httpclient.WithLogger(logger),
		),
		domain: domain,
	}
}

// FetchObject fetches an ActivityPub object with authorization
func (f *AuthorizedFetchService) FetchObject(ctx context.Context, objectURL string, signingActor *activitypub.Actor) (any, error) {
	f.logger.Debug("fetching object with authorization",
		zap.String("object_url", objectURL),
		zap.String("signing_actor", signingActor.ID))

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		f.logger.Error("failed to create HTTP request", zap.String("url", objectURL), zap.Error(err))
		return nil, errors.Join(ErrRequestCreationFailed, err)
	}

	// Set headers
	req.Header.Set("Accept", ActivityPubAcceptType)
	req.Header.Set("User-Agent", UserAgent)

	// Validate repository access for private key retrieval
	if err := common.ValidateRepositoryAccess(signingActor.PreferredUsername, signingActor.ID, "GetActorPrivateKey"); err != nil {
		f.logger.Error("repository access validation failed", zap.String("username", signingActor.PreferredUsername), zap.Error(err))
		return nil, errors.Join(ErrRepositoryAccessValidationFailed, err)
	}

	// Get the actor's private key
	privateKeyPEM, err := f.store.Actor().GetActorPrivateKey(ctx, signingActor.PreferredUsername)
	if err != nil {
		f.logger.Error("failed to get private key", zap.String("username", signingActor.PreferredUsername), zap.Error(err))
		return nil, errors.Join(ErrPrivateKeyRetrievalFailed, err)
	}

	// Parse the private key
	privateKey, err := ParsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		f.logger.Error("failed to parse private key", zap.Error(err))
		return nil, errors.Join(ErrPrivateKeyParseFailed, err)
	}

	// Sign the request
	if err := SignHTTPRequest(req, privateKey, signingActor.PublicKey.ID); err != nil {
		f.logger.Error("failed to sign request", zap.String("key_id", signingActor.PublicKey.ID), zap.Error(err))
		return nil, errors.Join(ErrRequestSigningFailed, err)
	}

	// Send the request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		f.logger.Error("failed to send request", zap.String("url", objectURL), zap.Error(err))
		return nil, errors.Join(ErrHTTPRequestFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		f.logger.Error("object fetch failed with non-2xx status",
			zap.String("url", objectURL),
			zap.Int("status_code", resp.StatusCode))
		return nil, ErrFetchObjectHTTPFailed
	}

	// Decode the response
	var result map[string]any
	if err := common.ParseHTTPResponse(resp.Body, &result); err != nil {
		f.logger.Error("failed to decode response", zap.String("url", objectURL), zap.Error(err))
		return nil, errors.Join(ErrResponseDecodeFailed, err)
	}

	// Validate the object
	if err := f.validateObject(result, objectURL); err != nil {
		f.logger.Error("object validation failed", zap.String("url", objectURL), zap.Error(err))
		return nil, errors.Join(ErrObjectValidationFailed, err)
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
	objMap, ok := obj.(map[string]any)
	if !ok {
		return nil, ErrInvalidActorObjectType
	}

	// Marshal to JSON then unmarshal to Actor
	data, err := json.Marshal(objMap)
	if err != nil {
		f.logger.Error("failed to marshal actor data", zap.Error(err))
		return nil, errors.Join(ErrActorDataMarshalFailed, err)
	}

	var actor activitypub.Actor
	if err := common.ParseActivityPubObject(data, &actor); err != nil {
		f.logger.Error("failed to unmarshal actor", zap.Error(err))
		return nil, errors.Join(ErrActorUnmarshalFailed, err)
	}

	// Validate actor type
	switch actor.Type {
	case activitypub.PersonType, activitypub.ServiceType, activitypub.GroupType,
		activitypub.OrganizationType, activitypub.ApplicationType:
		// Valid actor types
	default:
		f.logger.Error("invalid actor type", zap.String("type", actor.Type))
		return nil, ErrNotActorObject
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
	if err := common.ValidateRequiredParam("signature", signature); err != nil {
		return nil, ErrMissingSignatureHeader
	}

	// Parse the signature
	sig, err := ParseSignatureHeader(signature)
	if err != nil {
		f.logger.Error("failed to parse signature", zap.String("signature", signature), zap.Error(err))
		return nil, errors.Join(ErrSignatureParseFailed, err)
	}

	// Extract actor ID from keyId
	actorID := extractActorIDFromKeyID(sig.KeyID)
	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		f.logger.Error("failed to extract actor ID from keyId", zap.String("key_id", sig.KeyID))
		return nil, ErrExtractActorIDFailed
	}

	// Fetch the actor to get their public key
	// Note: This creates a chicken-and-egg problem in strict authorized fetch mode
	// In practice, you might need to allow unsigned fetches for actor documents
	// or maintain a cache of known actors
	actor, err := f.fetchActorWithoutAuth(ctx, actorID)
	if err != nil {
		f.logger.Error("failed to fetch actor", zap.String("actor_id", actorID), zap.Error(err))
		return nil, errors.Join(ErrFetchRemoteActorFailed, err)
	}

	// Parse the public key
	publicKey, err := ParsePublicKeyPEM([]byte(actor.PublicKey.PublicKeyPem))
	if err != nil {
		f.logger.Error("failed to parse public key", zap.String("actor_id", actor.ID), zap.Error(err))
		return nil, errors.Join(ErrPublicKeyParseFailed, err)
	}

	// Verify the signature
	if err := VerifyHTTPSignature(req, publicKey); err != nil {
		f.logger.Error("signature verification failed", zap.String("actor_id", actor.ID), zap.Error(err))
		return nil, errors.Join(ErrSignatureVerificationFailed, err)
	}

	return actor, nil
}

// IsAuthorizedFetchEnabled checks if authorized fetch is enabled
func (f *AuthorizedFetchService) IsAuthorizedFetchEnabled(ctx context.Context) bool {
	// Check centralized configuration first
	cfg := config.Get()
	if cfg.AuthorizedFetchEnabled {
		return true
	}

	// Fall back to checking instance configuration from storage
	rules, err := f.store.Instance().GetInstanceRules(ctx)
	if err != nil {
		f.logger.Debug("failed to get instance rules, defaulting authorized fetch to disabled",
			zap.Error(err))
		return false
	}

	// Look for authorized fetch rule
	for _, rule := range rules {
		if rule.ID == "authorized_fetch_enabled" {
			return rule.Text == "true" || rule.Text == "1"
		}
	}

	// Default to false for security (require explicit enablement)
	return false
}

// Private methods

func (f *AuthorizedFetchService) validateObject(obj map[string]any, expectedID string) error {
	// Verify the object ID matches what we requested
	id, ok := obj["id"].(string)
	if !ok || id != expectedID {
		f.logger.Error("object ID mismatch", zap.String("expected", expectedID), zap.String("got", id))
		return ErrObjectIDMismatch
	}

	// Verify the object has a type
	if _, ok := obj["type"].(string); !ok {
		return ErrObjectMissingType
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		f.logger.Error("actor fetch failed with non-2xx status",
			zap.String("url", actorURL),
			zap.Int("status_code", resp.StatusCode))
		return nil, ErrFetchActorHTTPFailed
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
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
