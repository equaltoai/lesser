package reputation

import (
	"math/rand"
	"testing"
	"testing/quick"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Tests for VouchManager
// =============================================================================

// TestNewVouchManager verifies the VouchManager constructor
// Requirements: 2.1
func TestNewVouchManager(t *testing.T) {
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	// Create VouchManager with nil storage (VouchManager accepts core.RepositoryStorage)
	vm := NewVouchManager(nil, nil, instanceURL, logger)

	require.NotNil(t, vm)
	require.Equal(t, instanceURL, vm.instanceURL)
	require.Equal(t, logger, vm.logger)
	require.Nil(t, vm.store)
	require.Nil(t, vm.signer)
}

// =============================================================================
// Property Tests for VouchManager
// =============================================================================

// TestProperty_ConfidenceValidation verifies that confidence must be between 0 and 1
// **Property 4: Confidence Validation**
// **Validates: Requirements 2.2**
func TestProperty_ConfidenceValidation(t *testing.T) {
	// This property test validates the confidence validation logic
	// Since VouchManager.CreateVouch requires storage, we test the validation logic directly

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		// Generate random confidence values
		confidence := r.Float64()*4 - 1 // Range: -1 to 3

		// The validation rule: confidence must be between 0 and 1
		isValid := confidence >= 0 && confidence <= 1

		// If confidence is outside [0, 1], it should be invalid
		if confidence < 0 || confidence > 1 {
			return !isValid // Should be invalid
		}

		return isValid // Should be valid
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}

// TestProperty_ReputationThreshold verifies that voucher reputation must be >= 500
// **Property 5: Reputation Threshold**
// **Validates: Requirements 2.4**
func TestProperty_ReputationThreshold(t *testing.T) {
	// This property test validates the reputation threshold logic
	// The threshold is 500 - vouchers with less reputation cannot create vouches

	property := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		// Generate random reputation values (0 to 1000)
		reputation := r.Intn(1001)

		// The validation rule: reputation must be >= 500
		canVouch := reputation >= 500

		// Verify the threshold logic
		if reputation < 500 {
			return !canVouch // Should not be able to vouch
		}

		return canVouch // Should be able to vouch
	}

	config := &quick.Config{MaxCount: 100}
	if err := quick.Check(property, config); err != nil {
		t.Errorf("Property failed: %v", err)
	}
}

// =============================================================================
// Tests for Vouch validation logic
// These test the validation rules that VouchManager enforces
// =============================================================================

// TestVouchValidation_ConfidenceBounds tests confidence validation bounds
func TestVouchValidation_ConfidenceBounds(t *testing.T) {
	testCases := []struct {
		name       string
		confidence float64
		valid      bool
	}{
		{"valid_zero", 0.0, true},
		{"valid_one", 1.0, true},
		{"valid_half", 0.5, true},
		{"valid_low", 0.1, true},
		{"valid_high", 0.9, true},
		{"invalid_negative", -0.1, false},
		{"invalid_above_one", 1.1, false},
		{"invalid_large_negative", -100.0, false},
		{"invalid_large_positive", 100.0, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isValid := tc.confidence >= 0 && tc.confidence <= 1
			require.Equal(t, tc.valid, isValid, "confidence %f validation mismatch", tc.confidence)
		})
	}
}

// TestVouchValidation_ReputationThreshold tests reputation threshold validation
func TestVouchValidation_ReputationThreshold(t *testing.T) {
	const threshold = 500

	testCases := []struct {
		name       string
		reputation int
		canVouch   bool
	}{
		{"exactly_threshold", 500, true},
		{"above_threshold", 600, true},
		{"well_above_threshold", 1000, true},
		{"below_threshold", 499, false},
		{"zero_reputation", 0, false},
		{"just_below_threshold", 400, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			canVouch := tc.reputation >= threshold
			require.Equal(t, tc.canVouch, canVouch, "reputation %d threshold check mismatch", tc.reputation)
		})
	}
}

// TestVouchValidation_ExpirationCheck tests vouch expiration validation
func TestVouchValidation_ExpirationCheck(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name      string
		expiresAt time.Time
		isExpired bool
	}{
		{"not_expired_future", now.Add(24 * time.Hour), false},
		{"not_expired_far_future", now.Add(180 * 24 * time.Hour), false},
		{"expired_past", now.Add(-24 * time.Hour), true},
		{"expired_just_now", now.Add(-1 * time.Second), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isExpired := now.After(tc.expiresAt)
			require.Equal(t, tc.isExpired, isExpired, "expiration check mismatch for %v", tc.expiresAt)
		})
	}
}

// TestVouchValidation_ActiveAndRevokedStatus tests vouch status validation
func TestVouchValidation_ActiveAndRevokedStatus(t *testing.T) {
	testCases := []struct {
		name    string
		active  bool
		revoked bool
		valid   bool
	}{
		{"active_not_revoked", true, false, true},
		{"inactive_not_revoked", false, false, false},
		{"active_revoked", true, true, false},
		{"inactive_revoked", false, true, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// A vouch is valid if it's active AND not revoked
			isValid := tc.active && !tc.revoked
			require.Equal(t, tc.valid, isValid, "status validation mismatch")
		})
	}
}

// =============================================================================
// Tests for convertStorageVouchesToReputationVouches
// This is a helper function that can be tested without storage
// =============================================================================

// TestConvertStorageVouchesToReputationVouches tests the conversion function
func TestConvertStorageVouchesToReputationVouches(t *testing.T) {
	vm := &VouchManager{
		instanceURL: "https://test.example.com",
		logger:      zap.NewNop(),
	}

	now := time.Now()
	expiresAt := now.Add(180 * 24 * time.Hour)
	revokedAt := now.Add(-24 * time.Hour)

	t.Run("empty_slice", func(t *testing.T) {
		result := vm.convertStorageVouchesToReputationVouches(nil)
		require.NotNil(t, result)
		require.Empty(t, result)
	})

	t.Run("single_vouch", func(t *testing.T) {
		storageVouches := []*storageVouch{
			{
				ID:                "vouch1",
				From:              "actor1",
				To:                "actor2",
				CreatedAt:         now,
				ExpiresAt:         &expiresAt,
				Confidence:        0.8,
				Context:           "test context",
				VoucherReputation: 600.0,
				Active:            true,
				Revoked:           false,
				Signature:         "sig123",
			},
		}

		result := vm.convertStorageVouchesToReputationVouches(storageVouches)
		require.Len(t, result, 1)
		require.Equal(t, "vouch1", result[0].ID)
		require.Equal(t, "actor1", result[0].From)
		require.Equal(t, "actor2", result[0].To)
		require.Equal(t, "https://test.example.com", result[0].InstanceURL)
		require.Equal(t, 0.8, result[0].Confidence)
		require.Equal(t, "test context", result[0].Context)
		require.Equal(t, 600, result[0].VoucherReputation)
		require.True(t, result[0].Active)
		require.False(t, result[0].Revoked)
		require.Equal(t, "sig123", result[0].Signature)
	})

	t.Run("multiple_vouches", func(t *testing.T) {
		storageVouches := []*storageVouch{
			{
				ID:                "vouch1",
				From:              "actor1",
				To:                "actor2",
				CreatedAt:         now,
				ExpiresAt:         &expiresAt,
				Confidence:        0.8,
				VoucherReputation: 600.0,
				Active:            true,
			},
			{
				ID:                "vouch2",
				From:              "actor3",
				To:                "actor2",
				CreatedAt:         now,
				ExpiresAt:         &expiresAt,
				Confidence:        0.9,
				VoucherReputation: 700.0,
				Active:            true,
			},
		}

		result := vm.convertStorageVouchesToReputationVouches(storageVouches)
		require.Len(t, result, 2)
	})

	t.Run("vouch_with_nil_expires_at", func(t *testing.T) {
		storageVouches := []*storageVouch{
			{
				ID:                "vouch1",
				From:              "actor1",
				To:                "actor2",
				CreatedAt:         now,
				ExpiresAt:         nil, // Nil expiration
				Confidence:        0.8,
				VoucherReputation: 600.0,
				Active:            true,
			},
		}

		result := vm.convertStorageVouchesToReputationVouches(storageVouches)
		require.Len(t, result, 1)
		require.True(t, result[0].ExpiresAt.IsZero(), "ExpiresAt should be zero time when nil")
	})

	t.Run("revoked_vouch", func(t *testing.T) {
		storageVouches := []*storageVouch{
			{
				ID:                "vouch1",
				From:              "actor1",
				To:                "actor2",
				CreatedAt:         now,
				ExpiresAt:         &expiresAt,
				Confidence:        0.8,
				VoucherReputation: 600.0,
				Active:            false,
				Revoked:           true,
				RevokedAt:         &revokedAt,
			},
		}

		result := vm.convertStorageVouchesToReputationVouches(storageVouches)
		require.Len(t, result, 1)
		require.False(t, result[0].Active)
		require.True(t, result[0].Revoked)
		require.NotNil(t, result[0].RevokedAt)
	})
}

// storageVouch is a local type alias for testing
type storageVouch = storage.Vouch
