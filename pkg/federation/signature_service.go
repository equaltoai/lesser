package federation

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// SignatureService provides enhanced HTTP signature verification with caching and retry logic
type SignatureService struct {
	publicKeyCacheRepo *repositories.PublicKeyCacheRepository
	logger             *zap.Logger
}

// NewSignatureService creates a new signature service
func NewSignatureService(publicKeyCacheRepo *repositories.PublicKeyCacheRepository, logger *zap.Logger) *SignatureService {
	return &SignatureService{
		publicKeyCacheRepo: publicKeyCacheRepo,
		logger:             logger,
	}
}

// VerifySignature verifies an HTTP request signature with caching and retry logic
func (s *SignatureService) VerifySignature(ctx context.Context, req *http.Request, actorURL string) error {
	log := s.logger.With(zap.String("actor_url", actorURL))

	// Try to get cached public key first
	publicKey, keyID, algorithm, err := s.getCachedPublicKey(ctx, actorURL, log)
	if err != nil {
		// Cache miss or error - fetch key with retry logic
		publicKey, keyID, algorithm, err = s.fetchPublicKeyWithRetry(ctx, actorURL, log)
		if err != nil {
			log.Error("failed to fetch public key after retries", zap.Error(err))
			return common.AuthenticationError{Message: "unable to fetch public key for verification"}
		}
	}

	// Verify the signature
	verifyErr := s.verifyWithAlgorithm(req, publicKey, algorithm)

	// Update cache statistics based on verification result
	success := verifyErr == nil
	if err := s.publicKeyCacheRepo.UpdateStats(ctx, actorURL, success); err != nil {
		log.Warn("failed to update cache statistics", zap.Error(err))
	}

	if verifyErr != nil {
		log.Warn("signature verification failed",
			zap.String("key_id", keyID),
			zap.String("algorithm", algorithm),
			zap.Error(verifyErr))
		return common.AuthenticationError{Message: "signature verification failed"}
	}

	log.Debug("signature verification successful",
		zap.String("key_id", keyID),
		zap.String("algorithm", algorithm))

	return nil
}

// getCachedPublicKey tries to get a public key from cache
func (s *SignatureService) getCachedPublicKey(ctx context.Context, actorURL string, log *zap.Logger) (crypto.PublicKey, string, string, error) {
	cache, err := s.publicKeyCacheRepo.GetByActorURL(ctx, actorURL)
	if err != nil {
		return nil, "", "", err
	}

	// Parse the cached PEM key
	publicKey, err := ParsePublicKeyPEM(cache.GetPublicKeyPEM())
	if err != nil {
		log.Warn("failed to parse cached public key, invalidating cache",
			zap.Error(err),
			zap.String("key_id", cache.KeyID))
		// Invalid cached key - remove it
		if invalidateErr := s.publicKeyCacheRepo.InvalidateCache(ctx, actorURL); invalidateErr != nil {
			log.Warn("failed to invalidate cache", zap.Error(invalidateErr))
		}
		return nil, "", "", fmt.Errorf("invalid cached public key: %w", err)
	}

	log.Debug("using cached public key",
		zap.String("key_id", cache.KeyID),
		zap.String("algorithm", cache.Algorithm),
		zap.Time("fetched_at", cache.FetchedAt))

	return publicKey, cache.KeyID, cache.Algorithm, nil
}

// fetchPublicKeyWithRetry fetches a public key with exponential backoff retry logic
func (s *SignatureService) fetchPublicKeyWithRetry(ctx context.Context, actorURL string, log *zap.Logger) (crypto.PublicKey, string, string, error) {
	const maxRetries = 3
	retryDelays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, "", "", ctx.Err()
			case <-time.After(retryDelays[attempt-1]):
				// Continue with retry
			}
		}

		log.Debug("attempting to fetch public key",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries))

		publicKey, keyID, algorithm, pemData, err := s.fetchActorPublicKeyWithPEM(ctx, actorURL, log)
		if err != nil {
			lastErr = err
			log.Warn("public key fetch attempt failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		// Success - cache the key using the original PEM data
		if _, cacheErr := s.publicKeyCacheRepo.Store(ctx, actorURL, keyID, pemData, algorithm); cacheErr != nil {
			log.Warn("failed to cache public key", zap.Error(cacheErr))
			// Continue even if caching fails
		}

		log.Debug("successfully fetched and cached public key",
			zap.String("key_id", keyID),
			zap.String("algorithm", algorithm),
			zap.Int("attempt", attempt+1))

		return publicKey, keyID, algorithm, nil
	}

	return nil, "", "", fmt.Errorf("failed to fetch public key after %d attempts: %w", maxRetries, lastErr)
}

// fetchActorPublicKeyWithPEM fetches an actor's public key from their profile and returns the PEM data
func (s *SignatureService) fetchActorPublicKeyWithPEM(ctx context.Context, actorURL string, log *zap.Logger) (crypto.PublicKey, string, string, string, error) {
	// Create secure HTTP client with timeout
	client := httpclient.NewSecureClient(
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithLogger(log),
	)

	// Create request with ActivityPub Accept header
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to fetch actor: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", "", fmt.Errorf("failed to fetch actor: status %d", resp.StatusCode)
	}

	// Parse actor
	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, "", "", "", fmt.Errorf("failed to parse actor: %w", err)
	}

	// Extract public key
	if actor.PublicKey == nil || actor.PublicKey.PublicKeyPem == "" {
		return nil, "", "", "", fmt.Errorf("actor has no public key")
	}

	// Parse PEM-encoded public key
	publicKey, err := ParsePublicKeyPEM([]byte(actor.PublicKey.PublicKeyPem))
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to parse public key: %w", err)
	}

	// Determine algorithm based on key type (use enhanced detection)
	algorithm := determineAlgorithm(publicKey)

	log.Debug("fetched actor public key",
		zap.String("key_id", actor.PublicKey.ID),
		zap.String("algorithm", algorithm))

	return publicKey, actor.PublicKey.ID, algorithm, actor.PublicKey.PublicKeyPem, nil
}

// verifyWithAlgorithm verifies the signature using the appropriate algorithm
func (s *SignatureService) verifyWithAlgorithm(req *http.Request, publicKey crypto.PublicKey, algorithm string) error {
	// Use the enhanced verification if algorithm is specified, otherwise fall back to basic verification
	if algorithm != "" && algorithm != "rsa-sha256" {
		// Create a synthetic signature object for enhanced verification
		sigHeader := req.Header.Get(SignatureHeader)
		if sigHeader == "" {
			return common.AuthenticationError{Message: "missing signature header"}
		}

		sig, err := ParseSignatureHeader(sigHeader)
		if err != nil {
			return fmt.Errorf("failed to parse signature: %w", err)
		}

		return VerifyHTTPSignatureEnhanced(req, publicKey, sig)
	}

	// Use basic verification for rsa-sha256
	return VerifyHTTPSignature(req, publicKey)
}

// VerifyDigestWithCompatibility verifies digest with both SHA-256 and sha-256 prefixes for compatibility
func (s *SignatureService) VerifyDigestWithCompatibility(req *http.Request, body []byte) error {
	digestHeader := req.Header.Get(DigestHeader)
	if digestHeader == "" {
		// No digest header is okay for some implementations
		return nil
	}

	// Parse digest header (format: "SHA-256=base64hash" or "sha-256=base64hash")
	parts := strings.SplitN(digestHeader, "=", 2)
	if len(parts) != 2 {
		return common.AuthenticationError{Message: "invalid digest header format"}
	}

	algorithm := strings.ToLower(strings.TrimSpace(parts[0]))
	expectedDigest := strings.TrimSpace(parts[1])

	// Support both SHA-256 and sha-256 for compatibility
	if algorithm != "sha-256" {
		return common.AuthenticationError{Message: fmt.Sprintf("unsupported digest algorithm: %s", algorithm)}
	}

	// Calculate actual digest
	actualDigest := calculateDigest(body)
	// Extract the base64 part (after "SHA-256=")
	actualDigestValue := strings.SplitN(actualDigest, "=", 2)[1]

	// Compare digests
	if actualDigestValue != expectedDigest {
		s.logger.Warn("digest mismatch",
			zap.String("expected", expectedDigest),
			zap.String("actual", actualDigestValue))
		return common.AuthenticationError{Message: "digest verification failed"}
	}

	s.logger.Debug("digest verification successful")
	return nil
}

// determineAlgorithm determines the appropriate algorithm based on key type
func determineAlgorithm(publicKey crypto.PublicKey) string {
	switch publicKey.(type) {
	case *rsa.PublicKey:
		// Use hs2019 for modern RSA keys (better interop)
		return AlgorithmHS2019
	case *ecdsa.PublicKey:
		// Use hs2019 for ECDSA keys
		return AlgorithmHS2019
	case ed25519.PublicKey:
		// Use hs2019 for Ed25519 keys
		return AlgorithmHS2019
	default:
		// Fall back to legacy RSA-SHA256 for unknown key types
		return AlgorithmRSASHA256
	}
}

// determineAlgorithmForSigning determines the best signing algorithm based on key type and compatibility
