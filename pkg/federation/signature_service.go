package federation

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// SignatureService provides enhanced HTTP signature verification with caching and retry logic
type SignatureService struct {
	publicKeyCacheRepo signaturePublicKeyCacheRepository
	httpClient         httpDoer
	logger             *zap.Logger
	sleep              func(ctx context.Context, d time.Duration) error
}

type signaturePublicKeyCacheRepository interface {
	GetByActorURL(ctx context.Context, actorURL string) (*models.PublicKeyCache, error)
	InvalidateCache(ctx context.Context, actorURL string) error
	Store(ctx context.Context, actorURL, keyID, publicKeyPEM, algorithm string) (*models.PublicKeyCache, error)
	UpdateStats(ctx context.Context, actorURL string, success bool) error
}

// NewSignatureService creates a new signature service
func NewSignatureService(publicKeyCacheRepo *repositories.PublicKeyCacheRepository, logger *zap.Logger) *SignatureService {
	var repo signaturePublicKeyCacheRepository
	if publicKeyCacheRepo != nil {
		repo = publicKeyCacheRepo
	}

	return &SignatureService{
		publicKeyCacheRepo: repo,
		httpClient: httpclient.NewSecureClient(
			httpclient.WithTimeout(10*time.Second),
			httpclient.WithLogger(logger),
		),
		logger: logger,
		sleep:  defaultSignatureSleep,
	}
}

func defaultSignatureSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// VerifySignature verifies an HTTP request signature with caching and retry logic
func (s *SignatureService) VerifySignature(ctx context.Context, req *http.Request, actorURL string) error {
	log := s.logger.With(zap.String("actor_url", actorURL))
	trace, ok := FollowTraceFromContext(ctx)
	if !ok && req != nil {
		trace, ok = FollowTraceFromContext(req.Context())
	}

	if ok {
		fields := append(FollowTraceFields(trace, "signature.verify.entry"),
			zap.String("actor_url", actorURL),
		)
		s.logger.Info("federation follow trace", fields...)
	}

	// Try to get cached public key first
	publicKey, keyID, algorithm, err := s.getCachedPublicKey(ctx, actorURL, log)
	if err != nil {
		if ok {
			fields := append(FollowTraceFields(trace, "signature.verify.cache_fetch"),
				zap.String("actor_url", actorURL),
				zap.String("cache_result", "fetch_required"),
				zap.String("cache_error", err.Error()),
			)
			s.logger.Info("federation follow trace", fields...)
		}
		// Cache miss or error - fetch key with retry logic
		publicKey, keyID, algorithm, err = s.fetchPublicKeyWithRetry(ctx, actorURL, log)
		if err != nil {
			log.Error("failed to fetch public key after retries", zap.Error(err))
			return common.AuthenticationError{Message: "unable to fetch public key for verification"}
		}
	} else if ok {
		fields := append(FollowTraceFields(trace, "signature.verify.cache_fetch"),
			zap.String("actor_url", actorURL),
			zap.String("cache_result", "hit"),
			zap.String("cached_key_id", keyID),
			zap.String("cached_algorithm", algorithm),
		)
		s.logger.Info("federation follow trace", fields...)
	}

	// Verify the signature
	verifyErr := s.verifyWithAlgorithm(ctx, req, publicKey, algorithm)

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
		log.Error("cached public key is invalid, invalidating cache",
			zap.String("key_id", cache.KeyID),
			zap.String("actor_url", actorURL),
			zap.Error(err))
		// Invalid cached key - remove it
		if invalidateErr := s.publicKeyCacheRepo.InvalidateCache(ctx, actorURL); invalidateErr != nil {
			log.Warn("failed to invalidate cache", zap.Error(invalidateErr))
		}
		return nil, "", "", errors.Join(ErrInvalidCachedPublicKey, err)
	}

	log.Debug("using cached public key",
		zap.String("key_id", cache.KeyID),
		zap.String("algorithm", cache.Algorithm),
		zap.Time("fetched_at", cache.FetchedAt))

	if trace, ok := FollowTraceFromContext(ctx); ok {
		fields := append(FollowTraceFields(trace, "signature.verify.cache_hit"),
			zap.String("actor_url", actorURL),
			zap.String("cached_key_id", cache.KeyID),
			zap.String("cached_algorithm", cache.Algorithm),
			zap.Time("cached_fetched_at", cache.FetchedAt),
		)
		s.logger.Info("federation follow trace", fields...)
	}

	return publicKey, cache.KeyID, cache.Algorithm, nil
}

// fetchPublicKeyWithRetry fetches a public key with exponential backoff retry logic
func (s *SignatureService) fetchPublicKeyWithRetry(ctx context.Context, actorURL string, log *zap.Logger) (crypto.PublicKey, string, string, error) {
	const maxRetries = 3
	retryDelays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			if err := s.sleep(ctx, retryDelays[attempt-1]); err != nil {
				return nil, "", "", err
			}
		}

		log.Debug("attempting to fetch public key",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries))

		if trace, ok := FollowTraceFromContext(ctx); ok {
			fields := append(FollowTraceFields(trace, "signature.verify.fetch_attempt"),
				zap.String("actor_url", actorURL),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
			)
			s.logger.Info("federation follow trace", fields...)
		}

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

	log.Error("public key fetch failed after all retry attempts",
		zap.String("actor_url", actorURL),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr))
	return nil, "", "", errors.Join(ErrPublicKeyFetchFailed, lastErr)
}

// fetchActorPublicKeyWithPEM fetches an actor's public key from their profile and returns the PEM data
func (s *SignatureService) fetchActorPublicKeyWithPEM(ctx context.Context, actorURL string, log *zap.Logger) (crypto.PublicKey, string, string, string, error) {
	// Create request with ActivityPub Accept header
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		log.Error("failed to create HTTP request for actor fetch",
			zap.String("actor_url", actorURL),
			zap.String("method", http.MethodGet),
			zap.Error(err))
		return nil, "", "", "", errors.Join(ErrRequestCreationFailed, err)
	}

	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", "Lesser/1.0")

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed during actor fetch",
			zap.String("actor_url", actorURL),
			zap.String("method", req.Method),
			zap.Error(err))
		return nil, "", "", "", errors.Join(ErrFetchActorHTTPFailed, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Error("actor fetch returned non-OK status",
			zap.String("actor_url", actorURL),
			zap.Int("status_code", resp.StatusCode),
			zap.String("status", resp.Status))
		return nil, "", "", "", ErrFetchActorHTTPStatusFailed
	}

	// Parse actor
	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		log.Error("failed to parse ActivityPub actor response",
			zap.String("actor_url", actorURL),
			zap.Int("status_code", resp.StatusCode),
			zap.String("content_type", resp.Header.Get("Content-Type")),
			zap.Error(err))
		return nil, "", "", "", errors.Join(ErrActorUnmarshalFailed, err)
	}

	// Extract public key
	if actor.PublicKey == nil || actor.PublicKey.PublicKeyPem == "" {
		log.Error("actor does not have a public key for signature verification",
			zap.String("actor_url", actorURL),
			zap.String("actor_id", actor.ID),
			zap.Bool("has_public_key_object", actor.PublicKey != nil),
			zap.Bool("has_pem_data", actor.PublicKey != nil && actor.PublicKey.PublicKeyPem != ""))
		return nil, "", "", "", ErrActorHasNoPublicKey
	}

	// Parse PEM-encoded public key
	publicKey, err := ParsePublicKeyPEM([]byte(actor.PublicKey.PublicKeyPem))
	if err != nil {
		log.Error("failed to parse actor's PEM-encoded public key",
			zap.String("actor_url", actorURL),
			zap.String("key_id", actor.PublicKey.ID),
			zap.Int("pem_length", len(actor.PublicKey.PublicKeyPem)),
			zap.Error(err))
		return nil, "", "", "", errors.Join(ErrPublicKeyParseFailed, err)
	}

	// Determine algorithm based on key type (use enhanced detection)
	algorithm := determineAlgorithm(publicKey)

	log.Debug("fetched actor public key",
		zap.String("key_id", actor.PublicKey.ID),
		zap.String("algorithm", algorithm))

	if trace, ok := FollowTraceFromContext(ctx); ok {
		fields := append(FollowTraceFields(trace, "signature.verify.fetched_actor"),
			zap.String("actor_url", actorURL),
			zap.String("fetched_actor_id", actor.ID),
			zap.String("fetched_public_key_id", actor.PublicKey.ID),
			zap.String("fetched_public_key_owner", actor.PublicKey.Owner),
			zap.String("fetched_algorithm", algorithm),
		)
		s.logger.Info("federation follow trace", fields...)
	}

	return publicKey, actor.PublicKey.ID, algorithm, actor.PublicKey.PublicKeyPem, nil
}

// verifyWithAlgorithm verifies the signature using the appropriate algorithm
func (s *SignatureService) verifyWithAlgorithm(ctx context.Context, req *http.Request, publicKey crypto.PublicKey, algorithm string) error {
	if trace, ok := FollowTraceFromContext(ctx); ok {
		fields := append(FollowTraceFields(trace, "signature.verify.algorithm"),
			zap.String("selected_algorithm", algorithm),
		)
		fields = append(fields, BuildSignatureTraceFields(req)...)
		s.logger.Info("federation follow trace", fields...)
	}

	// Use the enhanced verification if algorithm is specified, otherwise fall back to basic verification
	if algorithm != "" && algorithm != "rsa-sha256" {
		// Create a synthetic signature object for enhanced verification
		sigHeader := req.Header.Get(SignatureHeader)
		sig, err := ParseSignatureHeader(sigHeader)
		if err != nil {
			s.logger.Error("failed to parse signature header for enhanced verification",
				zap.String("algorithm", algorithm),
				zap.String("signature_header", sigHeader),
				zap.Error(err))
			return errors.Join(ErrSignatureParseFailed, err)
		}

		err = VerifyHTTPSignatureEnhanced(req, publicKey, sig)
		if trace, ok := FollowTraceFromContext(ctx); ok && err != nil {
			fields := append(FollowTraceFields(trace, "signature.verify.result"),
				zap.String("selected_algorithm", algorithm),
				zap.String("verification_error", err.Error()),
			)
			s.logger.Info("federation follow trace", fields...)
		}
		return err
	}

	// Use basic verification for rsa-sha256
	err := VerifyHTTPSignature(req, publicKey)
	if trace, ok := FollowTraceFromContext(ctx); ok && err != nil {
		fields := append(FollowTraceFields(trace, "signature.verify.result"),
			zap.String("selected_algorithm", AlgorithmRSASHA256),
			zap.String("verification_error", err.Error()),
		)
		s.logger.Info("federation follow trace", fields...)
	}
	return err
}

// VerifyDigestWithCompatibility verifies digest with both SHA-256 and sha-256 prefixes for compatibility
func (s *SignatureService) VerifyDigestWithCompatibility(req *http.Request, body []byte) error {
	digestHeader := req.Header.Get(DigestHeader)
	if err := common.ValidateRequiredParam("digestHeader", digestHeader); err != nil {
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
		s.logger.Error("unsupported digest algorithm", zap.String("algorithm", algorithm))
		return common.AuthenticationError{Message: "unsupported digest algorithm"}
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
