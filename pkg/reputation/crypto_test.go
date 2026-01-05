package reputation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(t *testing.T, status int, payload any) *http.Response {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}
}

func textResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
		},
	}
}

func TestCanonicalizationCompatibility(t *testing.T) {
	// Create a logger for testing
	logger := zap.NewNop()

	// Create a signer with generated keys for testing
	signer, err := NewSigner("", "https://test.example.com", logger)
	require.NoError(t, err)

	t.Run("reputation signing and verification", func(t *testing.T) {
		rep := &Reputation{
			ActorID:         "https://test.example.com/users/alice",
			InstanceURL:     "https://test.example.com",
			TrustScore:      850,
			ActivityScore:   720,
			ModerationScore: 900,
			CommunityScore:  780,
			TotalScore:      812,
			CalculatedAt:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Version:         "1.0",
			TotalPosts:      100,
			TotalFollowers:  50,
			AccountAge:      365,
			VouchCount:      5,
		}

		// Sign the reputation
		err := signer.SignReputation(rep)
		require.NoError(t, err)

		// Verify signature is present
		assert.NotEmpty(t, rep.Signature)
		assert.NotEmpty(t, rep.PublicKey)

		// Create verifier
		verifier := NewVerifier("https://test.example.com", logger, nil)

		// Verify the signature
		valid, err := verifier.VerifyReputation(rep)
		require.NoError(t, err)
		assert.True(t, valid, "signature should be valid")
	})

	t.Run("vouch signing and verification", func(t *testing.T) {
		vouch := &Vouch{
			ID:          "https://test.example.com/vouches/1",
			From:        "https://test.example.com/users/alice",
			To:          "https://test.example.com/users/bob",
			InstanceURL: "https://test.example.com",
			CreatedAt:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt:   time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC),
			Confidence:  0.95,
			Context:     "colleague",
			Active:      true,
		}

		// Sign the vouch
		err := signer.SignVouch(vouch)
		require.NoError(t, err)

		// Verify signature is present
		assert.NotEmpty(t, vouch.Signature)
	})

	t.Run("portable reputation signing", func(t *testing.T) {
		rep := &Reputation{
			ActorID:         "https://test.example.com/users/alice",
			InstanceURL:     "https://test.example.com",
			TrustScore:      850,
			ActivityScore:   720,
			ModerationScore: 900,
			CommunityScore:  780,
			TotalScore:      812,
			CalculatedAt:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			Version:         "1.0",
		}

		vouch := Vouch{
			ID:          "https://test.example.com/vouches/1",
			From:        "https://test.example.com/users/alice",
			To:          "https://test.example.com/users/bob",
			InstanceURL: "https://test.example.com",
			CreatedAt:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt:   time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC),
			Confidence:  0.95,
			Context:     "colleague",
			Active:      true,
		}

		pr := &PortableReputation{
			Context:    []string{"https://w3id.org/security/v1", "https://example.com/reputation/v1"},
			Type:       "ReputationAssertion",
			Actor:      "https://test.example.com/users/alice",
			Reputation: rep,
			Vouches:    []Vouch{vouch},
		}

		// Sign the portable reputation
		err := signer.SignPortableReputation(pr)
		require.NoError(t, err)

		// Verify all signatures are present
		assert.NotEmpty(t, pr.IssuerProof)
		assert.NotEmpty(t, pr.Reputation.Signature)
		assert.NotEmpty(t, pr.Vouches[0].Signature)
		assert.Equal(t, "https://test.example.com", pr.Issuer)
	})
}

func TestCanonicalizationDeterminism(t *testing.T) {
	// Test that the canonicalization produces deterministic results
	rep := &Reputation{
		// Test with fields in non-alphabetical order
		Version:         "1.0",
		ActorID:         "https://test.example.com/users/alice",
		TotalScore:      812,
		InstanceURL:     "https://test.example.com",
		ActivityScore:   720,
		TrustScore:      850,
		ModerationScore: 900,
		CommunityScore:  780,
		CalculatedAt:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Canonicalize multiple times
	var results [][]byte
	for i := 0; i < 5; i++ {
		canonical, err := canonicalizeJSON(rep)
		require.NoError(t, err)
		results = append(results, canonical)
	}

	// All results should be identical
	firstResult := string(results[0])
	for i, result := range results[1:] {
		assert.Equal(t, firstResult, string(result), "canonicalization %d should match first result", i+1)
	}

	// Result should not contain signature fields
	assert.NotContains(t, firstResult, "signature")
	assert.NotContains(t, firstResult, "Signature")
	assert.NotContains(t, firstResult, "issuerProof")
	assert.NotContains(t, firstResult, "IssuerProof")

	t.Logf("Canonical JSON: %s", firstResult)
}

func TestCanonicalizationPerformance(t *testing.T) {
	rep := &Reputation{
		ActorID:         "https://test.example.com/users/alice",
		InstanceURL:     "https://test.example.com",
		TrustScore:      850,
		ActivityScore:   720,
		ModerationScore: 900,
		CommunityScore:  780,
		TotalScore:      812,
		CalculatedAt:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:         "1.0",
	}

	// Measure performance over multiple iterations
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := canonicalizeJSON(rep)
		require.NoError(t, err)
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	t.Logf("Average canonicalization time over %d iterations: %v", iterations, avgDuration)

	// Ensure it's reasonably fast (should be well under 1ms per operation)
	assert.Less(t, avgDuration, time.Millisecond, "canonicalization should be fast")
}

// =============================================================================
// Tests for VerifyPortableReputation
// Requirements: 3.1, 3.2, 3.3, 3.4
// =============================================================================

func TestVerifyPortableReputation_ExpiredDocument(t *testing.T) {
	// Requirements: 3.1 - WHEN VerifyPortableReputation is called with expired document
	// THEN the Verifier SHALL return invalid with NotExpired=false
	logger := zap.NewNop()

	// Create a signer to sign the document
	signer, err := NewSigner("", "https://issuer.example.com", logger)
	require.NoError(t, err)

	// Create a portable reputation - we'll manually set expiration after signing
	pr := &PortableReputation{
		Context: []string{"https://w3id.org/security/v1"},
		Type:    "ReputationAssertion",
		Actor:   "https://test.example.com/users/alice",
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now().Add(-60 * 24 * time.Hour),
			Version:      "1.0",
		},
	}

	// Sign the document (this sets IssuedAt and ExpiresAt to valid values)
	err = signer.SignPortableReputation(pr)
	require.NoError(t, err)

	// Now manually set the expiration to the past to simulate an expired document
	pr.ExpiresAt = time.Now().Add(-30 * 24 * time.Hour) // Expired 30 days ago

	// Create verifier (nil storage defaults to trusting all domains)
	verifier := NewVerifier("https://test.example.com", logger, nil)

	// Verify - should fail due to expiration (checked before fetching public key)
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.NotExpired, "NotExpired should be false for expired document")
	assert.False(t, result.Valid, "Valid should be false for expired document")
	assert.Contains(t, result.Error, "expired")
}

func TestVerifyPortableReputation_InvalidIssuerProof(t *testing.T) {
	// Requirements: 3.2 - WHEN VerifyPortableReputation is called with invalid issuer proof
	// THEN the Verifier SHALL return invalid with SignatureValid=false
	logger := zap.NewNop()

	// Create a signer
	signer, err := NewSigner("", "https://issuer.example.com", logger)
	require.NoError(t, err)

	// Create a test server that returns a DIFFERENT public key than what was used to sign
	differentSigner, err := NewSigner("", "https://different.example.com", logger)
	require.NoError(t, err)

	// Create a portable reputation
	pr := &PortableReputation{
		Context:   []string{"https://w3id.org/security/v1"},
		Type:      "ReputationAssertion",
		Actor:     "https://test.example.com/users/alice",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		},
	}

	// Sign the document with original signer
	err = signer.SignPortableReputation(pr)
	require.NoError(t, err)

	// Create verifier (nil storage defaults to trusting all domains)
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": differentSigner.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify - should fail due to invalid signature (different key returned by server)
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.NotExpired, "NotExpired should be true")
	assert.False(t, result.SignatureValid, "SignatureValid should be false for invalid proof")
	assert.False(t, result.Valid, "Valid should be false")
}

func TestVerifyPortableReputation_ValidDocument(t *testing.T) {
	// Requirements: 3.4 - WHEN VerifyPortableReputation is called with valid document
	// THEN the Verifier SHALL return valid=true
	logger := zap.NewNop()

	issuerURL := "https://issuer.example.com"
	signer, err := NewSigner("", issuerURL, logger)
	require.NoError(t, err)

	// Create a portable reputation
	pr := &PortableReputation{
		Context: []string{"https://w3id.org/security/v1"},
		Type:    "ReputationAssertion",
		Actor:   "https://test.example.com/users/alice",
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		},
	}

	// Sign the document - this will set Issuer to issuerURL
	err = signer.SignPortableReputation(pr)
	require.NoError(t, err)

	// Verify the issuer was set correctly
	assert.Equal(t, issuerURL, pr.Issuer)

	// Create verifier (nil storage defaults to trusting all domains)
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify - should succeed
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.NotExpired, "NotExpired should be true")
	assert.True(t, result.SignatureValid, "SignatureValid should be true")
	assert.True(t, result.IssuerTrusted, "IssuerTrusted should be true (nil storage defaults to trust)")
	assert.True(t, result.Valid, "Valid should be true for valid document")
}

// =============================================================================
// Tests for getInstancePublicKey
// Requirements: 3.5, 3.6
// =============================================================================

func TestGetInstancePublicKey_CachedKey(t *testing.T) {
	// Requirements: 3.5 - WHEN getInstancePublicKey is called with cached key
	// THEN the Verifier SHALL return the cached key
	logger := zap.NewNop()

	// Create a signer to get a valid public key
	signer, err := NewSigner("", "https://cached.example.com", logger)
	require.NoError(t, err)

	instanceURL := "https://cached.example.com"

	// Create an HTTP client that should NOT be called (key is cached)
	serverCalled := false

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			serverCalled = true
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Pre-populate the cache with a key
	// We need to decode the base64 public key to get the raw bytes
	pubKeyBytes, err := base64.StdEncoding.DecodeString(signer.GetPublicKeyBase64())
	require.NoError(t, err)
	verifier.keyCache[instanceURL] = pubKeyBytes

	// Call getInstancePublicKey - should return cached key without calling server
	key, err := verifier.getInstancePublicKey(context.Background(), instanceURL)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Verify server was NOT called
	assert.False(t, serverCalled, "Server should not be called when key is cached")

	// Verify the returned key matches what we cached
	assert.Equal(t, pubKeyBytes, []byte(key))
}

func TestGetInstancePublicKey_FetchFromWellKnown(t *testing.T) {
	// Requirements: 3.6 - WHEN getInstancePublicKey is called with uncached key
	// THEN the Verifier SHALL fetch from .well-known endpoint
	logger := zap.NewNop()

	// Create a signer to get a valid public key
	signer, err := NewSigner("", "https://fetch.example.com", logger)
	require.NoError(t, err)

	instanceURL := "https://fetch.example.com"

	// Create an HTTP client that returns the public key
	serverCalled := false

	// Create verifier with empty cache
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			serverCalled = true
			if req.URL.Host == "fetch.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Call getInstancePublicKey - should fetch from server
	key, err := verifier.getInstancePublicKey(context.Background(), instanceURL)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Verify server WAS called
	assert.True(t, serverCalled, "Server should be called when key is not cached")

	// Verify the key was cached for future use
	cachedKey, exists := verifier.keyCache[instanceURL]
	assert.True(t, exists, "Key should be cached after fetch")
	assert.Equal(t, key, cachedKey)
}

func TestGetInstancePublicKey_HTTPError(t *testing.T) {
	// Requirements: 3.6 - Test HTTP error handling
	logger := zap.NewNop()

	instanceURL := "https://error.example.com"

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusInternalServerError, ""), nil
		}),
	}

	// Call getInstancePublicKey - should fail due to HTTP error
	key, err := verifier.getInstancePublicKey(context.Background(), instanceURL)
	require.Error(t, err)
	require.Nil(t, key)
	assert.Contains(t, err.Error(), "unexpected status code")
}

func TestGetInstancePublicKey_InvalidResponse(t *testing.T) {
	// Requirements: 3.6 - Test invalid response handling
	logger := zap.NewNop()
	instanceURL := "https://invalid.example.com"

	t.Run("invalid JSON response", func(t *testing.T) {
		verifier := NewVerifier("https://test.example.com", logger, nil)
		verifier.httpClient = &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "invalid.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
					return textResponse(http.StatusOK, "not valid json"), nil
				}
				return textResponse(http.StatusNotFound, ""), nil
			}),
		}

		key, err := verifier.getInstancePublicKey(context.Background(), instanceURL)
		require.Error(t, err)
		require.Nil(t, key)
		assert.Contains(t, err.Error(), "failed to decode")
	})

	t.Run("invalid base64 public key", func(t *testing.T) {
		verifier := NewVerifier("https://test.example.com", logger, nil)
		verifier.httpClient = &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "invalid.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
					return jsonResponse(t, http.StatusOK, map[string]string{
						"public_key": "not-valid-base64!!!",
					}), nil
				}
				return textResponse(http.StatusNotFound, ""), nil
			}),
		}

		key, err := verifier.getInstancePublicKey(context.Background(), instanceURL)
		require.Error(t, err)
		require.Nil(t, key)
		assert.Contains(t, err.Error(), "invalid public key encoding")
	})
}

// =============================================================================
// Tests for isInstanceTrusted (tested indirectly through VerifyPortableReputation)
// Requirements: 3.7, 3.8, 3.9
// =============================================================================

func TestIsInstanceTrusted_NilStorageDefaultsToTrue(t *testing.T) {
	// Requirements: 3.9 - WHEN isInstanceTrusted is called with nil storage
	// THEN the Verifier SHALL return true (default to trusting)
	logger := zap.NewNop()

	issuerURL := "https://issuer.example.com"
	signer, err := NewSigner("", issuerURL, logger)
	require.NoError(t, err)

	// Create a portable reputation
	pr := &PortableReputation{
		Context: []string{"https://w3id.org/security/v1"},
		Type:    "ReputationAssertion",
		Actor:   "https://test.example.com/users/alice",
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		},
	}

	// Sign the document
	err = signer.SignPortableReputation(pr)
	require.NoError(t, err)

	// Create verifier with nil storage (should default to trusting all domains)
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify - IssuerTrusted should be true because nil storage defaults to trust
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.IssuerTrusted, "IssuerTrusted should be true when storage is nil")
}

func TestIsInstanceTrusted_URLParsingError(t *testing.T) {
	// Requirements: 3.9 - Test URL parsing error handling
	// When URL parsing fails, isInstanceTrusted should return false
	logger := zap.NewNop()

	// Create a signer
	signer, err := NewSigner("", "https://test.example.com", logger)
	require.NoError(t, err)

	// Create a portable reputation with an invalid issuer URL
	pr := &PortableReputation{
		Context:   []string{"https://w3id.org/security/v1"},
		Type:      "ReputationAssertion",
		Actor:     "https://test.example.com/users/alice",
		Issuer:    "://invalid-url", // Invalid URL that will fail parsing
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		},
	}

	// Sign the reputation component only (we'll manually set the issuer proof)
	err = signer.SignReputation(pr.Reputation)
	require.NoError(t, err)

	// Set a dummy issuer proof (it won't be verified because key fetch will fail first)
	pr.IssuerProof = "dummy-proof"

	// Create verifier with nil storage
	verifier := NewVerifier("https://test.example.com", logger, nil)

	// Verify - should fail because the issuer URL is invalid/untrusted
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The verification should fail at the trust check stage
	assert.False(t, result.Valid, "Valid should be false for invalid issuer URL")
	assert.Contains(t, result.Error, "issuer is not trusted")
}

func TestIsInstanceTrusted_DomainWithPort(t *testing.T) {
	// Test that domain extraction correctly handles ports
	logger := zap.NewNop()

	issuerURL := "https://issuer.example.com:8443"
	signer, err := NewSigner("", issuerURL, logger)
	require.NoError(t, err)

	// Create a portable reputation
	pr := &PortableReputation{
		Context: []string{"https://w3id.org/security/v1"},
		Type:    "ReputationAssertion",
		Actor:   "https://test.example.com/users/alice",
		Reputation: &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		},
	}

	// Sign the document
	err = signer.SignPortableReputation(pr)
	require.NoError(t, err)

	// Create verifier with nil storage
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com:8443" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify - should succeed even with port in URL
	result, err := verifier.VerifyPortableReputation(pr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Valid, "Valid should be true even with port in issuer URL")
}

// =============================================================================
// Tests for VerifyVouchSignature
// Requirements: 3.10, 3.11
// =============================================================================

func TestVerifyVouchSignature_ValidSignature(t *testing.T) {
	// Requirements: 3.10 - WHEN VerifyVouchSignature is called with valid signature
	// THEN the Verifier SHALL return true
	logger := zap.NewNop()

	issuerURL := "https://issuer.example.com"
	signer, err := NewSigner("", issuerURL, logger)
	require.NoError(t, err)

	// Create and sign a vouch
	vouch := &Vouch{
		ID:          "https://test.example.com/vouches/1",
		From:        "https://test.example.com/users/alice",
		To:          "https://test.example.com/users/bob",
		InstanceURL: issuerURL,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(180 * 24 * time.Hour),
		Confidence:  0.9,
		Context:     "colleague",
		Active:      true,
	}

	err = signer.SignVouch(vouch)
	require.NoError(t, err)
	require.NotEmpty(t, vouch.Signature)

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify the signature
	valid, err := verifier.VerifyVouchSignature(vouch)
	require.NoError(t, err)
	assert.True(t, valid, "Valid signature should return true")
}

func TestVerifyVouchSignature_InvalidSignature(t *testing.T) {
	// Requirements: 3.11 - WHEN VerifyVouchSignature is called with invalid signature
	// THEN the Verifier SHALL return false
	logger := zap.NewNop()

	// Create a signer for signing
	signer, err := NewSigner("", "https://signer.example.com", logger)
	require.NoError(t, err)

	// Create a different signer whose key will be returned by the server
	differentSigner, err := NewSigner("", "https://different.example.com", logger)
	require.NoError(t, err)

	// Create and sign a vouch with the original signer
	issuerURL := "https://issuer.example.com"
	vouch := &Vouch{
		ID:          "https://test.example.com/vouches/1",
		From:        "https://test.example.com/users/alice",
		To:          "https://test.example.com/users/bob",
		InstanceURL: issuerURL, // Points to server returning different key
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(180 * 24 * time.Hour),
		Confidence:  0.9,
		Context:     "colleague",
		Active:      true,
	}

	err = signer.SignVouch(vouch)
	require.NoError(t, err)

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": differentSigner.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify the signature - should fail because server returns different key
	valid, err := verifier.VerifyVouchSignature(vouch)
	require.NoError(t, err)
	assert.False(t, valid, "Invalid signature should return false")
}

func TestVerifyVouchSignature_KeyFetchError(t *testing.T) {
	// Requirements: 3.11 - Test key fetch error handling
	logger := zap.NewNop()

	// Create a signer
	signer, err := NewSigner("", "https://test.example.com", logger)
	require.NoError(t, err)

	// Create and sign a vouch
	issuerURL := "https://issuer.example.com"
	vouch := &Vouch{
		ID:          "https://test.example.com/vouches/1",
		From:        "https://test.example.com/users/alice",
		To:          "https://test.example.com/users/bob",
		InstanceURL: issuerURL, // Points to server that returns error
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(180 * 24 * time.Hour),
		Confidence:  0.9,
		Context:     "colleague",
		Active:      true,
	}

	err = signer.SignVouch(vouch)
	require.NoError(t, err)

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return textResponse(http.StatusInternalServerError, ""), nil
		}),
	}

	// Verify the signature - should fail due to key fetch error
	valid, err := verifier.VerifyVouchSignature(vouch)
	require.Error(t, err)
	assert.False(t, valid, "Should return false when key fetch fails")
	assert.Contains(t, err.Error(), "failed to get issuer public key")
}

func TestVerifyVouchSignature_InvalidSignatureEncoding(t *testing.T) {
	// Test invalid signature encoding
	logger := zap.NewNop()

	// Create a signer
	signer, err := NewSigner("", "https://test.example.com", logger)
	require.NoError(t, err)

	// Create a vouch with invalid signature encoding
	issuerURL := "https://issuer.example.com"
	vouch := &Vouch{
		ID:          "https://test.example.com/vouches/1",
		From:        "https://test.example.com/users/alice",
		To:          "https://test.example.com/users/bob",
		InstanceURL: issuerURL,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(180 * 24 * time.Hour),
		Confidence:  0.9,
		Context:     "colleague",
		Active:      true,
		Signature:   "not-valid-base64!!!",
	}

	// Create verifier
	verifier := NewVerifier("https://test.example.com", logger, nil)
	verifier.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "issuer.example.com" && req.URL.Path == "/.well-known/reputation-keys" {
				return jsonResponse(t, http.StatusOK, map[string]string{
					"public_key": signer.GetPublicKeyBase64(),
				}), nil
			}
			return textResponse(http.StatusNotFound, ""), nil
		}),
	}

	// Verify the signature - should fail due to invalid encoding
	valid, err := verifier.VerifyVouchSignature(vouch)
	require.Error(t, err)
	assert.False(t, valid, "Should return false for invalid signature encoding")
	assert.Contains(t, err.Error(), "invalid signature encoding")
}

// =============================================================================
// Tests for GetPublicKeyBase64 on Signer
// Requirements: 3.12
// =============================================================================

func TestGetPublicKeyBase64(t *testing.T) {
	// Requirements: 3.12 - WHEN GetPublicKeyBase64 is called on Signer
	// THEN the Signer SHALL return the base64-encoded public key
	logger := zap.NewNop()

	t.Run("returns valid base64 encoded key", func(t *testing.T) {
		signer, err := NewSigner("", "https://test.example.com", logger)
		require.NoError(t, err)

		pubKeyBase64 := signer.GetPublicKeyBase64()

		// Verify it's not empty
		assert.NotEmpty(t, pubKeyBase64, "Public key should not be empty")

		// Verify it's valid base64
		decoded, err := base64.StdEncoding.DecodeString(pubKeyBase64)
		require.NoError(t, err, "Public key should be valid base64")

		// Verify the decoded key has the correct length for Ed25519 (32 bytes)
		assert.Equal(t, 32, len(decoded), "Ed25519 public key should be 32 bytes")
	})

	t.Run("returns consistent key", func(t *testing.T) {
		signer, err := NewSigner("", "https://test.example.com", logger)
		require.NoError(t, err)

		// Call multiple times - should return the same key
		key1 := signer.GetPublicKeyBase64()
		key2 := signer.GetPublicKeyBase64()
		key3 := signer.GetPublicKeyBase64()

		assert.Equal(t, key1, key2, "Public key should be consistent")
		assert.Equal(t, key2, key3, "Public key should be consistent")
	})

	t.Run("different signers have different keys", func(t *testing.T) {
		signer1, err := NewSigner("", "https://test1.example.com", logger)
		require.NoError(t, err)

		signer2, err := NewSigner("", "https://test2.example.com", logger)
		require.NoError(t, err)

		key1 := signer1.GetPublicKeyBase64()
		key2 := signer2.GetPublicKeyBase64()

		assert.NotEqual(t, key1, key2, "Different signers should have different keys")
	})

	t.Run("key from PEM matches GetPublicKeyBase64", func(t *testing.T) {
		// Create a signer with a generated key
		signer, err := NewSigner("", "https://test.example.com", logger)
		require.NoError(t, err)

		pubKeyBase64 := signer.GetPublicKeyBase64()

		// The public key should be usable for verification
		// Create a reputation and sign it
		rep := &Reputation{
			ActorID:      "https://test.example.com/users/alice",
			InstanceURL:  "https://test.example.com",
			TotalScore:   500,
			CalculatedAt: time.Now(),
			Version:      "1.0",
		}

		err = signer.SignReputation(rep)
		require.NoError(t, err)

		// The reputation's public key should match GetPublicKeyBase64
		assert.Equal(t, pubKeyBase64, rep.PublicKey, "Reputation public key should match GetPublicKeyBase64")
	})
}
