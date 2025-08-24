package reputation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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
